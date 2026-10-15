// xo_http.c: the HTTP/1.1 server of the native std/http (decision 0074),
// included by xo_std.c.
//
// What the Go backend's server observably does comes from net/http (Go
// 1.26: server.go, request.go, transfer.go, textproto, net/url) and from
// std/rt/http.go, which reads the whole body before the handler runs and
// writes the response in one piece. This file reproduces it:
//
//   - reading a request and writing the response on HTTP/1.1 are Xo's
//     (std/native/llrt/http1.xo Reader and respond, over http1w.xo; the
//     zero-C audit's B12 and B28): the C reads and writes the socket or
//     TLS for it (xo_hc_recv, xo_hc_send_at);
//   - the server (xo_srv, xo_hconn): connection tasks in a scope of their
//     own under main's root, so canceling serve lets requests in flight
//     finish (std/rt's reqRoot); a stop or the end of serve closes idle
//     connections and waits for the others;
//   - the client's connections, TLS, HTTP/2, and request writing
//     (xo_cl_start; http1.xo client_do drives it and reads HTTP/1.1
//     responses).
//
// internal/llvm's TestHTTPParseDiff sends the same bytes to this code and
// to a Go net/http server and compares what both answer; it is fuzzed
// there with AddressSanitizer (with testdata/http_ref.c, the C request
// parser, in http1.xo's place); TestHTTP1ParseDiff and
// TestHTTP1ClientDiff test the Xo parsers in programs.

// A request body longer than this is cut short like a body whose client
// went away (the handler gets what was read, the connection closes).
// Go has no limit; XO_HTTP_MAX_BODY overrides (bytes).
#define HTTP_MAX_BODY ((int64_t)64 << 20)

// The results of socket operations of the client (and of TLS ones)
enum { CC_OK, CC_EOF, CC_TIMEOUT, CC_CANCELED, CC_ERR };

// TLS (xo_tls.c, included at the end of this file; decision 0105)
typedef struct tls_conn tls_conn;
typedef struct tls_srv tls_srv;
typedef struct xo_troots xo_troots;
static int64_t tls_read(tls_conn *t, char *p, int64_t n);
static int tls_write(tls_conn *t, const char *p, int64_t n);
static void tls_conn_free(tls_conn *t);
static void tls_close_notify(tls_conn *t);
static void tls_srv_free(tls_srv *s);
static tls_conn *tls_server_conn(tls_srv *s, xo_pd *pd);
static int64_t tls_hc_read(tls_conn *t, char *p, int64_t n);
static int tls_client_start(xo_pd *pd, const char *host, xo_troots *roots, int64_t when, tls_conn **out, char *msg, size_t mn, int *kind);
static const char *tls_alert(tls_conn *t);
static int tls_server_handshake(tls_conn *t);
static int64_t tls_nb_read(tls_conn *t, char *p, int64_t n);
static int tls_alpn_h2(tls_conn *t);
static int tls_h2_bad_cipher(tls_conn *t, char *why, size_t wn);
static void tls_set_when(tls_conn *t, int64_t when);

// ---------------------------------------------------------------------------
// Strings and tables

typedef struct {
	char *p;
	int64_t n;
} hstr;

typedef struct {
	hstr k, v;
} hpair;

typedef struct {
	hpair *a;
	int64_t n, cap;
} hlist;

// The strings and pair arrays of requests and responses come from the
// runtime's allocator (xo_alloc: per thread free lists), not malloc: a
// request made about twenty of them, and macOS malloc's free path showed
// in server profiles (with mach_absolute_time under it).
static hstr hs_dup(const char *p, int64_t n) {
	hstr s = {xo_alloc(n + 1), n};
	if (n) {
		memcpy(s.p, p, (size_t)n);
	}
	s.p[n] = 0;
	return s;
}

static void hs_free(hstr *s) {
	if (s->p) {
		xo_free(s->p);
	}
	s->p = NULL;
	s->n = 0;
}

static int hs_eq(hstr a, const char *b) { return a.n == (int64_t)strlen(b) && memcmp(a.p, b, (size_t)a.n) == 0; }

static void hl_add(hlist *l, hstr k, hstr v) {
	if (l->n == l->cap) {
		int64_t old = l->cap;
		l->cap = l->cap ? l->cap * 2 : 8;
		int64_t sz = l->cap * (int64_t)sizeof(hpair);
		l->a = l->a ? xo_resize(l->a, old * (int64_t)sizeof(hpair), sz) : xo_alloc(sz);
	}
	l->a[l->n].k = k;
	l->a[l->n].v = v;
	l->n++;
}

// hl_clear empties l and keeps its array for reuse.
static void hl_clear(hlist *l) {
	for (int64_t i = 0; i < l->n; i++) {
		hs_free(&l->a[i].k);
		hs_free(&l->a[i].v);
	}
	l->n = 0;
}

static void hl_free(hlist *l) {
	hl_clear(l);
	if (l->a) {
		xo_free(l->a);
	}
	l->a = NULL;
	l->n = l->cap = 0;
}

static int64_t hl_find(hlist *l, const char *k) {
	for (int64_t i = 0; i < l->n; i++) {
		if (hs_eq(l->a[i].k, k)) {
			return i;
		}
	}
	return -1;
}

static void hl_del(hlist *l, const char *k) {
	int64_t i = hl_find(l, k);
	if (i < 0) {
		return;
	}
	hs_free(&l->a[i].k);
	hs_free(&l->a[i].v);
	memmove(l->a + i, l->a + i + 1, (size_t)(l->n - i - 1) * sizeof(hpair));
	l->n--;
}

// token bytes of RFC 7230 (textproto validHeaderFieldByte, httpguts
// isTokenTable)
static int h_tchar(unsigned c) {
	if ((c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
		return 1;
	}
	switch (c) {
	case '!':
	case '#':
	case '$':
	case '%':
	case '&':
	case '\'':
	case '*':
	case '+':
	case '-':
	case '.':
	case '^':
	case '_':
	case '`':
	case '|':
	case '~':
		return 1;
	}
	return 0;
}

// textproto validHeaderValueByte: VCHAR, SP, HTAB, obs-text
static int h_lower(int c) { return c >= 'A' && c <= 'Z' ? c + 32 : c; }

// h_canon is textproto.CanonicalMIMEHeaderKey (in place): unchanged when
// a byte is not a token byte.
static void h_canon(char *p, int64_t n) {
	for (int64_t i = 0; i < n; i++) {
		if (!h_tchar((unsigned char)p[i])) {
			return;
		}
	}
	int upper = 1;
	for (int64_t i = 0; i < n; i++) {
		char c = p[i];
		if (upper && c >= 'a' && c <= 'z') {
			c = (char)(c - 32);
		} else if (!upper && c >= 'A' && c <= 'Z') {
			c = (char)(c + 32);
		}
		p[i] = c;
		upper = c == '-';
	}
}

// h_has_token is httpguts.headerValueContainsToken (shouldClose).
static int h_has_token(const char *v, int64_t n, const char *tok) {
	int64_t tl = (int64_t)strlen(tok);
	int64_t i = 0;
	while (i <= n) {
		int64_t j = i;
		while (j < n && v[j] != ',') {
			j++;
		}
		int64_t a = i, b = j;
		while (a < b && (v[a] == ' ' || v[a] == '\t')) {
			a++;
		}
		while (b > a && (v[b - 1] == ' ' || v[b - 1] == '\t')) {
			b--;
		}
		if (b - a == tl) {
			int eq = 1;
			for (int64_t k = 0; k < tl; k++) {
				unsigned c = (unsigned char)v[a + k];
				if (c >= 0x80 || h_lower((int)c) != h_lower(tok[k])) {
					eq = 0;
					break;
				}
			}
			if (eq) {
				return 1;
			}
		}
		i = j + 1;
	}
	return 0;
}

// ---------------------------------------------------------------------------
// The parts of a request's URL. Request targets (net/url ParseRequestURI)
// and queries are parsed in Xo (std/native/llrt/http1.xo): HTTP/1.1's
// with the head, HTTP/2's in h2_fields (xo_h2.c h2_request).


typedef struct {
	hstr scheme, host, path, rawquery;
	int has_query;
} hurl;

static void hu_free(hurl *u) {
	hs_free(&u->scheme);
	hs_free(&u->host);
	hs_free(&u->path);
	hs_free(&u->rawquery);
}

// ---------------------------------------------------------------------------
// Reading a request

enum {
	HP_NEED = 0,      // more bytes needed
	HP_OK = 1,        // the head is complete
	HP_EOF = 2,       // the end of the stream before a request: close quietly
	HP_BAD = 3,       // "400 Bad Request"
	HP_STATUS = 4,    // statusError: code and text (400 and 505)
	HP_TOO_LARGE = 5, // 431
	HP_BAD_TE = 6,    // 501 unsupported transfer encoding
};

typedef struct {
	hstr method, target;
	int major, minor;
	hlist hdr;           // canonical names, values joined with ", ", in first appearance order
	hstr host;           // req.Host
	hurl url;
	hlist query;         // the URL's query: sorted keys, first values
	int64_t clen;        // ContentLength: -1 chunked
	int chunked;
	int close;           // req.Close
	int wants10ka, wantsclose;
	int expect100, expect_other;
	int h2up;            // "PRI * HTTP/2.0" (closed after the answer)
	int status;          // HP_STATUS: the code
	const char *reason;  // HP_STATUS: the text
} hreq;

static void hr_free(hreq *r) {
	hs_free(&r->method);
	hs_free(&r->target);
	hl_free(&r->hdr);
	hs_free(&r->host);
	hu_free(&r->url);
	hl_free(&r->query);
	memset(r, 0, sizeof *r);
}

// The results of a chunked body's decoding (http1.xo chunked).
enum { HB_NEED, HB_OK, HB_ERR };

// ---------------------------------------------------------------------------
// Writing a response: HTTP/1.1's is std/native/llrt/http1.xo's (write,
// respond, emit over http1w.xo; the zero-C audit's B12 and B28).

static int h_body_allowed(int64_t st) { return !((st >= 100 && st <= 199) || st == 204 || st == 304); }

// ---------------------------------------------------------------------------
// The server (std.md 4.4): handles for std/native/http.xo
//
// A server (xo_srv) keeps its live connections and a scope of its own
// for their tasks, linked under main's root: canceling the serving task
// does not cancel requests in flight (std/rt's reqRoot); ending main
// (the grace period, a second signal) does. A connection (xo_hconn) owns
// its input buffer, the request being served, and the header of the
// response being built.

typedef struct xo_srv {
	xo_hdr h;
	pthread_mutex_t mu;
	struct xo_hconn *conns; // live connections
	int64_t nconns;
	int stopping;
	xo_wl idle; // waiting for nconns == 0
	xo_scope *scope;
	int closed;
	tls_srv *tls; // Server.with_tls (xo_tls.c), NULL: plain HTTP
} xo_srv;

typedef struct xo_hconn {
	xo_hdr h;
	xo_srv *srv;
	xo_conn *conn;
	struct xo_hconn *prev, *next; // in srv->conns while registered
	int registered;
	int idle;                     // waiting for the first byte of a request
	char *buf;
	int64_t n, cap, off; // unconsumed input is buf[off:n]
	hreq req;     // an HTTP/2 stream's request
	int have_req;
	nbuf body;    // an HTTP/2 stream's request body
	hlist resp;   // an HTTP/2 stream's response header
	char ip[INET6_ADDRSTRLEN];
	// HTTP/1.1: http1.xo's X1 record (its input, request, and response
	// state), freed with the connection
	int64_t x1;
	tls_conn *tls; // a connection of a TLS server (xo_tls.c)
	// HTTP/2 (xo_h2.c): a connection whose ALPN chose h2 has h2; a
	// stream's request (h2_take) has h2s and holds its connection in
	// h2owner
	struct h2c *h2;
	struct h2s *h2s;
	struct xo_hconn *h2owner;
} xo_hconn;

// cabi_http.xo's exports (std/http's programs carry them; the weak
// definitions keep other programs linking, which never serve HTTP/1.1)
__attribute__((weak)) void xo_x_h1_free(int64_t x) { (void)x; }
__attribute__((weak)) int64_t xo_x_h1_emit(int64_t hc, int64_t status, int64_t state, int64_t p, int64_t n) {
	return (void)hc, (void)status, (void)p, (void)n, state;
}
__attribute__((weak)) int64_t xo_x_h1_respond(int64_t hc, int64_t status, int64_t p, int64_t n) {
	return (void)hc, (void)status, (void)p, (void)n, 0;
}
__attribute__((weak)) int64_t xo_x_h1_stream_end(int64_t hc, int64_t status, int64_t state, int64_t p, int64_t n) {
	return (void)hc, (void)status, (void)state, (void)p, (void)n, 0;
}

// HTTP/2 (xo_h2.c, included at the end of this file)
static void h2c_free(struct h2c *c);
static void h2_stop(struct h2c *c);
static struct xo_hconn *h2_untake(struct h2c *c);
static void h2_stream_release(xo_hconn *hc);
static int64_t h2_respond(xo_hconn *hc, int64_t status, const char *body, int64_t bn);
static int h2_sink_emit(xo_hconn *hc, int64_t status, int *started, const char *p, int64_t n);
static int h2_sink_finish(xo_hconn *hc, const char *p, int64_t n);

static void srv_unregister(xo_hconn *hc) {
	xo_srv *s = hc->srv;
	pthread_mutex_lock(&s->mu);
	if (hc->registered) {
		hc->registered = 0;
		if (hc->prev) {
			hc->prev->next = hc->next;
		} else {
			s->conns = hc->next;
		}
		if (hc->next) {
			hc->next->prev = hc->prev;
		}
		if (--s->nconns == 0) {
			wl_wake(&s->idle);
		}
	}
	pthread_mutex_unlock(&s->mu);
}

static void hconn_free(void *p) {
	xo_hconn *hc = p;
	if (hc->h2owner) {
		// an HTTP/2 stream's request
		if (hc->h2s) {
			h2_stream_release(hc);
		}
		if (hc->have_req) {
			hr_free(&hc->req);
		}
		free(hc->body.p);
		hl_free(&hc->resp);
		xo_hconn *owner = hc->h2owner;
		xo_free(hc);
		h_release(owner);
		return;
	}
	srv_unregister(hc);
	h2c_free(hc->h2);
	pd_close(hc->conn->pd);
	h_release(hc->conn);
	h_release(hc->srv);
	free(hc->buf);
	if (hc->have_req) {
		hr_free(&hc->req);
	}
	if (hc->x1) {
		xo_x_h1_free(hc->x1);
	}
	free(hc->body.p);
	hl_free(&hc->resp);
	tls_conn_free(hc->tls);
	xo_free(hc);
}

static void srv_free(void *p) {
	xo_srv *s = p;
	if (s->scope && !s->closed) {
		// never closed (serve did not get to its end): its tasks hold the
		// server, so none is left
		h_release(s->scope);
	}
	tls_srv_free(s->tls);
	pthread_mutex_destroy(&s->mu);
	xo_free(s);
}

// xo_srv_new makes a server and opens the scope of its connections.
xo_srv *xo_srv_new(void) {
	xo_ctx *c = ctx();
	xo_srv *s = xo_alloc(sizeof(xo_srv));
	memset(s, 0, sizeof *s);
	h_init(s, srv_free);
	mu_init(&s->mu);
	xo_scope *sc = xo_alloc(sizeof(xo_scope));
	memset(sc, 0, sizeof *sc);
	h_init(sc, XO_FREE_FN(xo_x_scope_free));
	sc->prune_at = 64; // finished connections leave the list (scope_prune)
	sc->parent_node = c->node;
	xo_node *root = (xo_node *)(intptr_t)xo_x_stop_root();
	node_link(&sc->body, root ? root : c->node);
	sc->tasks = calloc(1, sizeof(xo_node));
	if (!sc->tasks) {
		xo_oom();
	}
	node_link(sc->tasks, &sc->body);
	s->scope = sc;
	return s;
}

// xo_srv_group is the scope of the connection tasks as a task.Group.
xo_scope *xo_srv_group(xo_srv *s) { return xo_scope_group(s->scope); }

// xo_srv_stop stops the server: connections close after their current
// request, and idle ones now.
void xo_srv_stop(xo_srv *s) {
	pthread_mutex_lock(&s->mu);
	s->stopping = 1;
	for (xo_hconn *hc = s->conns; hc; hc = hc->next) {
		if (hc->h2) {
			// GOAWAY, then the streams in flight finish
			h2_stop(hc->h2);
		} else if (hc->idle) {
			pd_close(hc->conn->pd);
		}
	}
	pthread_mutex_unlock(&s->mu);
}

static void srv_wake_timer(xo_timer *t) { park_signal(t->arg); }

// xo_srv_wait waits for the connections to end: after a stop request
// (core 7.5) until they do or the serving task is canceled (the grace
// period ended); else (serve's task was canceled) at most 30 s, as
// net/http's Shutdown in std/rt.
void xo_srv_wait(xo_srv *s) {
	xo_ctx *c = ctx();
	int stopping = xo_stop_requested() != 0;
	int64_t until = stopping ? 0 : deadline((int64_t)30 * 1000000000);
	xo_timer t = {.when = until, .idx = -1, .fn = srv_wake_timer, .arg = &c->park};
	int timed = 0;
	pthread_mutex_lock(&s->mu);
	while (s->nconns > 0) {
		if (stopping && ctx_canceled(c)) {
			break;
		}
		if (until && __atomic_load_n(&t.fired, __ATOMIC_ACQUIRE)) {
			break;
		}
		xo_went e = {.park = &c->park};
		park_reset(&c->park);
		wl_add(&s->idle, &e);
		pthread_mutex_unlock(&s->mu);
		if (until && !timed) {
			timed = 1;
			timer_add(&t);
		}
		park_wait_n(c, stopping, 0);
		pthread_mutex_lock(&s->mu);
		wl_del(&s->idle, &e);
	}
	pthread_mutex_unlock(&s->mu);
	if (timed) {
		timer_del(&t);
	}
}

// xo_srv_close cancels the connections left and waits for their tasks.
void xo_srv_close(xo_srv *s) {
	if (s->closed) {
		return;
	}
	s->closed = 1;
	xo_srv_stop(s);
	pthread_mutex_lock(&s->mu);
	for (xo_hconn *hc = s->conns; hc; hc = hc->next) {
		pd_close(hc->conn->pd);
	}
	pthread_mutex_unlock(&s->mu);
	xo_ctx *c = ctx();
	xo_node *saved = c->node;
	int64_t u = c->unwinding;
	c->unwinding = 0;
	xo_scope_close(s->scope, 1);
	h_release(s->scope);
	c->node = saved;
	c->unwinding = u;
}

// xo_hc_new registers connection cn (accepted by the listener) with
// server s.
xo_hconn *xo_hc_new(xo_srv *s, xo_conn *cn) {
	xo_hconn *hc = xo_alloc(sizeof(xo_hconn));
	memset(hc, 0, sizeof *hc);
	h_init(hc, hconn_free);
	h_retain(s);
	h_retain(cn);
	hc->srv = s;
	hc->conn = cn;
	struct sockaddr_storage ss;
	socklen_t sl = sizeof ss;
	strcpy(hc->ip, "");
	if (nx_getpeername(cn->pd->fd, (struct sockaddr *)&ss, &sl) == 0) {
		if (ss.ss_family == AF_INET) {
			nx_ntop(AF_INET, &((struct sockaddr_in *)&ss)->sin_addr, hc->ip, sizeof hc->ip);
		} else if (ss.ss_family == AF_INET6) {
			nx_ntop(AF_INET6, &((struct sockaddr_in6 *)&ss)->sin6_addr, hc->ip, sizeof hc->ip);
		}
	}
	pthread_mutex_lock(&s->mu);
	hc->next = s->conns;
	if (s->conns) {
		s->conns->prev = hc;
	}
	s->conns = hc;
	s->nconns++;
	hc->registered = 1;
	if (s->tls) {
		hc->tls = tls_server_conn(s->tls, cn->pd);
	}
	if (s->stopping) {
		pd_close(cn->pd);
	}
	pthread_mutex_unlock(&s->mu);
	return hc;
}

// xo_hc_recv reads at most cap bytes into p (http1.xo fill): bytes read,
// 0 at the end of the stream, -1 when the connection was closed or the
// task canceled, or (idle: nothing of a request came yet, so a stop
// closes it) the server is stopping.
int64_t xo_hc_recv(xo_hconn *hc, char *p, int64_t cap, int64_t idle) {
	xo_srv *s = hc->srv;
	if (s) {
		pthread_mutex_lock(&s->mu);
		if (s->stopping && idle) {
			pthread_mutex_unlock(&s->mu);
			return -1;
		}
		hc->idle = (int)idle;
		pthread_mutex_unlock(&s->mu);
	}
	int64_t r = -1;
	xo_ctx *c = ctx();
	if (hc->tls) {
		// the handshake first, then decrypted bytes (xo_tls.c)
		if (!ctx_canceled(c)) {
			r = tls_hc_read(hc->tls, p, cap);
		}
	} else if (!ctx_canceled(c) && pd_begin(hc->conn->pd)) {
		xo_pd *pd = hc->conn->pd;
		for (;;) {
			r = x_read(pd->fd, p, (size_t)cap);
			if (r >= 0) {
				break;
			}
			if (r == -EINTR) {
				continue;
			}
			if ((r == -EAGAIN || r == -EWOULDBLOCK) && pd_wait(pd, 0, c) == PD_READY) {
				continue;
			}
			r = -1;
			break;
		}
		pd_end(pd);
	}
	if (s) {
		pthread_mutex_lock(&s->mu);
		hc->idle = 0;
		pthread_mutex_unlock(&s->mu);
	}
	return r < 0 ? -1 : r;
}

// hc_send writes the bytes of o: 0 when the connection broke.
static int hc_send(xo_hconn *hc, const char *p, int64_t n) {
	if (hc->tls) {
		return tls_write(hc->tls, p, n) == CC_OK;
	}
	xo_ctx *c = ctx();
	xo_pd *pd = hc->conn->pd;
	if (!pd_begin(pd)) {
		return 0;
	}
	int64_t off = 0;
	while (off < n) {
		int64_t w = x_write(pd->fd, p + off, (size_t)(n - off));
		if (w >= 0) {
			off += w;
			continue;
		}
		if (w == -EINTR) {
			continue;
		}
		if ((w == -EAGAIN || w == -EWOULDBLOCK) && pd_wait(pd, 1, c) == PD_READY) {
			continue;
		}
		pd_end(pd);
		return 0;
	}
	pd_end(pd);
	return 1;
}

// http_max_body is the largest request body HTTP/2's server reads
// (HTTP_MAX_BODY, XO_HTTP_MAX_BODY overrides; http1.xo max_body for
// HTTP/1.1).
static int64_t http_max_body(void) {
	static int64_t max = -1;
	int64_t m = __atomic_load_n(&max, __ATOMIC_RELAXED);
	if (m < 0) {
		const char *e = getenv("XO_HTTP_MAX_BODY");
		m = e ? atoll(e) : HTTP_MAX_BODY;
		if (m <= 0) {
			m = HTTP_MAX_BODY;
		}
		__atomic_store_n(&max, m, __ATOMIC_RELAXED);
	}
	return m;
}

// xo_hc_send_at is hc_send for http1.xo.
int64_t xo_hc_send_at(xo_hconn *hc, int64_t p, int64_t n) { return hc_send(hc, (const char *)(intptr_t)p, n); }

// xo_hc_stopping reports whether hc's server is stopping (no other
// request on the connection).
int64_t xo_hc_stopping(xo_hconn *hc) {
	if (!hc->srv) {
		return 0;
	}
	pthread_mutex_lock(&hc->srv->mu);
	int stopping = hc->srv->stopping;
	pthread_mutex_unlock(&hc->srv->mu);
	return stopping;
}

// xo_hc_shut_wr shuts down hc's sending side (after a 431 answer).
void xo_hc_shut_wr(xo_hconn *hc) {
	if (hc->conn) {
		nx_shutdown(hc->conn->pd->fd, SHUT_WR);
	}
}

int64_t xo_hc_x1(xo_hconn *hc) { return hc->x1; }

void xo_hc_set_x1(xo_hconn *hc, int64_t x) { hc->x1 = x; }

static xo_rstr *hstr_out(hstr s, int64_t *len) { return nstr(s.p ? s.p : "", s.n, len); }

xo_rstr *xo_hc_method(xo_hconn *hc, int64_t *len) { return hstr_out(hc->req.method, len); }

// xo_hc_path is the request's URL path (unescaped; Request.path).
xo_rstr *xo_hc_path(xo_hconn *hc, int64_t *len) { return hstr_out(hc->req.url.path, len); }

xo_rstr *xo_hc_scheme(xo_hconn *hc, int64_t *len) { return hstr_out(hc->req.url.scheme, len); }

// xo_hc_host is the request's Host as received; std/native/llrt/
// http1.xo host_name splits off the port (the zero-C audit's B28).
xo_rstr *xo_hc_host(xo_hconn *hc, int64_t *len) { return hstr_out(hc->req.host, len); }

// xo_hc_urlhost is the host of an absolute request target's URL;
// http1.xo host_port reads its port.
xo_rstr *xo_hc_urlhost(xo_hconn *hc, int64_t *len) { return hstr_out(hc->req.url.host, len); }

int64_t xo_hc_nheaders(xo_hconn *hc) { return hc->req.hdr.n; }

xo_rstr *xo_hc_hname(xo_hconn *hc, int64_t i, int64_t *len) { return hstr_out(hc->req.hdr.a[i].k, len); }

xo_rstr *xo_hc_hval(xo_hconn *hc, int64_t i, int64_t *len) { return hstr_out(hc->req.hdr.a[i].v, len); }

// the query parameters: sorted keys, first values (std/rt's urlOf)
int64_t xo_hc_nquery(xo_hconn *hc) { return hc->req.query.n; }

xo_rstr *xo_hc_qkey(xo_hconn *hc, int64_t i, int64_t *len) { return hstr_out(hc->req.query.a[i].k, len); }

xo_rstr *xo_hc_qval(xo_hconn *hc, int64_t i, int64_t *len) { return hstr_out(hc->req.query.a[i].v, len); }

xo_rstr *xo_hc_body(xo_hconn *hc, int64_t *len) { return nstr(hc->body.p ? hc->body.p : "", hc->body.n, len); }

xo_rstr *xo_hc_remote_ip(xo_hconn *hc, int64_t *len) { return nstr(hc->ip, (int64_t)strlen(hc->ip), len); }

// xo_hc_header sets a field of the response header (Header.Set: the name
// canonical, a later value replaces an earlier one).
void xo_hc_header(xo_hconn *hc, const char *k, int64_t kn, const char *v, int64_t vn) {
	hstr key = hs_dup(k, kn);
	h_canon(key.p, kn);
	for (int64_t i = 0; i < hc->resp.n; i++) {
		if (hc->resp.a[i].k.n == kn && memcmp(hc->resp.a[i].k.p, key.p, (size_t)kn) == 0) {
			hs_free(&key);
			hs_free(&hc->resp.a[i].v);
			hc->resp.a[i].v = hs_dup(v, vn);
			return;
		}
	}
	hl_add(&hc->resp, key, hs_dup(v, vn));
}

// xo_hc_respond writes an HTTP/2 stream's response (HTTP/1.1's is
// http1.xo respond): 1 when the connection stays open for another
// request.
int64_t xo_hc_respond(xo_hconn *hc, int64_t status, const char *body, int64_t bn) {
	if (hc->h2s) {
		return h2_respond(hc, status, body, bn);
	}
	return xo_x_h1_respond(XO_ADDR(hc), status, XO_ADDR(body), bn);
}

// ---------------------------------------------------------------------------
// Streamed bodies (http.stream, decision 0083)
//
// A sink takes the writes of a stream's body: into a buffer (a response
// dispatched in process, collected), or to a connection as net/http's
// response writer sends them: through a 2048 byte buffer (std/rt writes
// strings, and bufio.Writer.WriteString fills and flushes the buffer in
// turn), the head going out with the first data that leaves the buffer
// (or the first flush) while the handler is not done, so chunked
// on HTTP/1.1; a stream that never leaves the buffer is an ordinary
// response when it ends.

typedef struct {
	xo_hdr h;
	int conn;
	nbuf buf;
	xo_hconn *hc;
	int64_t status;
	int started, failed;
	int64_t state; // HTTP/1.1: http1.xo's SK_* bits
} xo_sink;

static void sink_free(void *p) {
	xo_sink *s = p;
	free(s->buf.p);
	if (s->hc) {
		h_release(s->hc);
	}
	xo_free(s);
}

static xo_sink *sink_new(void) {
	xo_sink *s = xo_alloc(sizeof(xo_sink));
	memset(s, 0, sizeof *s);
	h_init(s, sink_free);
	return s;
}

// xo_sink_buf is a sink into a buffer.
xo_sink *xo_sink_buf(void) { return sink_new(); }

// xo_sink_conn is a sink that sends the response of status on hc (its
// header set with xo_hc_header).
xo_sink *xo_sink_conn(xo_hconn *hc, int64_t status) {
	xo_sink *s = sink_new();
	h_retain(hc);
	s->conn = 1;
	s->hc = hc;
	s->status = status;
	return s;
}

// sink_emit writes p as net/http's chunkWriter does: the head with the
// first data, then a chunk per write (HTTP/1.1: http1.xo emit).
static void sink_emit(xo_sink *s, const char *p, int64_t n) {
	if (s->failed) {
		return;
	}
	xo_hconn *hc = s->hc;
	if (hc->h2s) {
		if (!h2_sink_emit(hc, s->status, &s->started, p, n)) {
			s->failed = 1;
		}
		return;
	}
	s->state = xo_x_h1_emit(XO_ADDR(hc), s->status, s->state, XO_ADDR(p), n);
	s->started = (s->state & 1) != 0;
	s->failed = (s->state & 16) != 0;
}

// xo_sink_write is BodyWriter.write: 0 when the connection failed.
int64_t xo_sink_write(xo_sink *s, const char *p, int64_t n) {
	if (!s->conn) {
		nbuf_put(&s->buf, p, n);
		return 1;
	}
	// bufio.Writer.WriteString (std/rt writes strings): no write goes
	// around the buffer, since chunkWriter has no WriteString; HTTP/2's
	// buffer is 4 KiB (handlerChunkWriteSize)
	int64_t size = s->hc->h2s ? 4096 : 2048;
	while (n > size - s->buf.n) {
		int64_t k = size - s->buf.n;
		nbuf_put(&s->buf, p, k);
		p += k;
		n -= k;
		sink_emit(s, s->buf.p, s->buf.n);
		s->buf.n = 0;
	}
	if (n > 0) {
		nbuf_put(&s->buf, p, n);
	}
	return !s->failed;
}

// xo_sink_flush is BodyWriter.flush: what is buffered goes out (the head
// at the latest).
int64_t xo_sink_flush(xo_sink *s) {
	if (!s->conn) {
		return 1;
	}
	if (s->buf.n > 0) {
		sink_emit(s, s->buf.p, s->buf.n);
		s->buf.n = 0;
	} else if (!s->started) {
		sink_emit(s, "", 0);
	}
	return !s->failed;
}

// xo_sink_end ends the stream on its connection: 1 when the connection
// stays open for another request.
int64_t xo_sink_end(xo_sink *s) {
	xo_hconn *hc = s->hc;
	if (!s->started) {
		return xo_hc_respond(hc, s->status, s->buf.p ? s->buf.p : "", s->buf.n);
	}
	if (hc->h2s) {
		// the rest with the end of the stream
		(void)h2_sink_finish(hc, s->buf.p ? s->buf.p : "", s->buf.n);
		s->buf.n = 0;
		return 0;
	}
	// HTTP/1.1 (http1.xo stream_end): the rest, the last chunk
	int64_t keep = xo_x_h1_stream_end(XO_ADDR(hc), s->status, s->state, XO_ADDR(s->buf.p ? s->buf.p : ""), s->buf.n);
	s->buf.n = 0;
	return keep;
}

// xo_sink_take is the body a buffer sink collected.
xo_rstr *xo_sink_take(xo_sink *s, int64_t *len) { return nstr(s->buf.p ? s->buf.p : "", s->buf.n, len); }

// The functions of streamed responses: a Response carries the number of
// its function here (0: no stream). Sending a response takes its
// function; a response dispatched in process keeps it, since a copy may
// be collected again (as on the Go backend); xo_streams_free empties the
// table at exit (leak check).
static struct {
	pthread_mutex_t mu;
	void **fs;
	int64_t n;
} streams = {.mu = PTHREAD_MUTEX_INITIALIZER};

// xo_stream_put keeps f (an Any of the stream's function): its number.
int64_t xo_stream_put(void *f) {
	h_retain(f);
	pthread_mutex_lock(&streams.mu);
	if ((streams.n & (streams.n - 1)) == 0) {
		streams.fs = realloc(streams.fs, (size_t)(streams.n == 0 ? 4 : 2 * streams.n) * sizeof(void *));
		if (!streams.fs) {
			xo_oom();
		}
	}
	streams.fs[streams.n++] = f;
	int64_t id = streams.n;
	pthread_mutex_unlock(&streams.mu);
	return id;
}

// xo_stream_get is the function of stream id in a list of one (empty
// when it was taken); take removes it from the table.
xo_rlist *xo_stream_get(int64_t id, int64_t take) {
	xo_rlist *l = xo_rl_new(4, 8);
	pthread_mutex_lock(&streams.mu);
	void *f = id >= 1 && id <= streams.n ? streams.fs[id - 1] : NULL;
	if (f) {
		if (take) {
			streams.fs[id - 1] = NULL;
		} else {
			h_retain(f);
		}
		((void **)rl_data(l))[0] = f;
		l->len = 1;
	}
	pthread_mutex_unlock(&streams.mu);
	return l;
}

int64_t xo_x_h2_pool_free(void);

__attribute__((unused)) static void xo_streams_free(void) {
	pthread_mutex_lock(&streams.mu);
	for (int64_t i = 0; i < streams.n; i++) {
		if (streams.fs[i]) {
			h_release(streams.fs[i]);
			streams.fs[i] = NULL;
		}
	}
	pthread_mutex_unlock(&streams.mu);
	// the client's idle HTTP/2 connections (xo_h2.c)
	xo_x_h2_pool_free();
}

// xo_hc_close closes the connection and ends its registration.
void xo_hc_close(xo_hconn *hc) {
	tls_close_notify(hc->tls);
	pd_close(hc->conn->pd);
	srv_unregister(hc);
	if (hc->h2) {
		// a request h2_next found that no task took (the serving task
		// was canceled): it holds the connection
		xo_hconn *t = h2_untake(hc->h2);
		if (t) {
			h_release(t);
		}
	}
}

// ---------------------------------------------------------------------------
// Values of any type (request values, std.md 4.3)

typedef struct {
	xo_hdr h;
	xo_rlist *l;  // the value, a shared list of one
	xo_ldrop drop;
	char *tag;    // its type
	int64_t tn;
} xo_any;

static void any_free(void *p) {
	xo_any *a = p;
	list_release(a->l, a->drop);
	free(a->tag);
	xo_free(a);
}

xo_any *xo_any_new(xo_rlist *l, int64_t drop, const char *tag, int64_t tn) {
	xo_any *a = xo_alloc(sizeof(xo_any));
	h_init(a, any_free);
	a->l = l;
	a->drop = (xo_ldrop)drop;
	a->tag = cstr_of(tag, tn);
	a->tn = tn;
	return a;
}

// xo_any_get is the list of one holding a's value (a reference) when its
// type is tag, else the empty list.
xo_rlist *xo_any_get(xo_any *a, const char *tag, int64_t tn) {
	if (a->tn != tn || memcmp(a->tag, tag, (size_t)tn) != 0) {
		return &empty_list;
	}
	rc_inc(&a->l->rc);
	return a->l;
}

// xo_fault_at faults with msg at loc (a std function's caller).
XO_NORETURN void xo_fault_at(const char *msg, int64_t n, const char *loc, int64_t ln) { fault_text(cstr_of(msg, n), cstr_of(loc, ln)); }

// ---------------------------------------------------------------------------
// Text of the HTTP/2 client's errors (the rest of std/http's text
// helpers are std/native/llrt/http1.xo's)

// ---------------------------------------------------------------------------
// The client (std.md 4.5): one exchange over a connection of its own, or
// a stream of a pooled HTTP/2 connection. Xo drives it (std/native/llrt/
// http1.xo client_do: the URL, the redirects net/http's Client follows,
// and the reading of an HTTP/1.1 response); C never calls compiled Xo
// (decision 0180), so cl_start returns with the request sent and the
// response for Xo to read (cl_input, cl_fill, cl_take), and cl_end
// closes the connection and makes the error.

typedef struct {
	xo_pd *pd;
	int fd;
	int64_t when; // deadline (monotonic), 0: none
	char *buf;
	int64_t n, cap, off;
	int eof;
	tls_conn *tls; // an https connection (xo_tls.c)
} ccon;

typedef struct {
	xo_hdr h;
	int err; // 0, or the ClientErr tag
	char *msg;
	// an HTTP/2 response (xo_h2.c); Xo reads HTTP/1.1's itself
	int64_t status;
	hlist hdr;
	nbuf body;
	void *h2;       // an HTTP/2 exchange Xo drives (hclient.xo X_*, http1.xo h2_exchange)
	int retry;      // it goes again on a new connection
	// an HTTP/1.1 exchange whose response Xo reads
	int reading;
	ccon c;
	xo_conn *cn;
	char *prefix; // the method and the quoted URL of errors: Get "http://h/"
} xo_cresp;

// cl_close closes r's connection, if it has one.
static void cl_close(xo_cresp *r) {
	if (!r->cn) {
		return;
	}
	tls_conn_free(r->c.tls);
	pd_close(r->c.pd);
	h_release(r->cn);
	free(r->c.buf);
	r->c = (ccon){0};
	r->cn = NULL;
	r->reading = 0;
}

// HTTP/2's connections, their pool and exchanges (hclient.xo); a
// program without std/http links the weak definitions: it never makes
// one, and its exit frees no pool.
int64_t xo_x_h2_conn_new(int64_t cn, int64_t c, int64_t roots, int64_t kp, int64_t kn);
int64_t xo_x_h2_pool_take(int64_t roots, int64_t kp, int64_t kn);
int64_t xo_x_h2_exchange_start(int64_t r, int64_t pc, int64_t pooled, int64_t body, int64_t bn, int64_t when);
int64_t xo_x_h2_exchange_free(int64_t x);
int64_t xo_x_h2_pool_free(void);

__attribute__((weak)) int64_t xo_x_h2_conn_new(int64_t cn, int64_t c, int64_t roots, int64_t kp, int64_t kn) {
	(void)cn, (void)c, (void)roots, (void)kp, (void)kn;
	fault_text("HTTP without std/http's exports", "");
}

__attribute__((weak)) int64_t xo_x_h2_pool_take(int64_t roots, int64_t kp, int64_t kn) {
	(void)roots, (void)kp, (void)kn;
	fault_text("HTTP without std/http's exports", "");
}

__attribute__((weak)) int64_t xo_x_h2_exchange_start(int64_t r, int64_t pc, int64_t pooled, int64_t body, int64_t bn, int64_t when) {
	(void)r, (void)pc, (void)pooled, (void)body, (void)bn, (void)when;
	fault_text("HTTP without std/http's exports", "");
}

__attribute__((weak)) int64_t xo_x_h2_exchange_free(int64_t x) {
	(void)x;
	return 0;
}

__attribute__((weak)) int64_t xo_x_h2_pool_free(void) { return 0; }

static void cresp_free(void *p) {
	xo_cresp *r = p;
	xo_x_h2_exchange_free(XO_ADDR(r->h2));
	cl_close(r);
	free(r->msg);
	free(r->prefix);
	hl_free(&r->hdr);
	free(r->body.p);
	xo_free(r);
}

// The connection's reads and writes are std/native/llrt/hclient.xo's
// fill and send (the zero-C audit's B29), through cabi_http.xo; a
// program without std/http never reaches them, and links the weak
// definitions, which fault.
int64_t xo_x_cc_fill(int64_t c);
int64_t xo_x_cc_send(int64_t c, int64_t p, int64_t n);
int64_t xo_x_cl_fail(int64_t r, int64_t tag, int64_t p, int64_t n);
int64_t xo_x_cl_finish(int64_t r, int64_t s, int64_t wp, int64_t wn, int64_t ap, int64_t an);

__attribute__((weak)) int64_t xo_x_cc_fill(int64_t c) {
	(void)c;
	fault_text("HTTP without std/http's exports", "");
}

__attribute__((weak)) int64_t xo_x_cc_send(int64_t c, int64_t p, int64_t n) {
	(void)c, (void)p, (void)n;
	fault_text("HTTP without std/http's exports", "");
}

__attribute__((weak)) int64_t xo_x_cl_fail(int64_t r, int64_t tag, int64_t p, int64_t n) {
	(void)r, (void)tag, (void)p, (void)n;
	fault_text("HTTP without std/http's exports", "");
}

__attribute__((weak)) int64_t xo_x_cl_finish(int64_t r, int64_t s, int64_t wp, int64_t wn, int64_t ap, int64_t an) {
	(void)r, (void)s, (void)wp, (void)wn, (void)ap, (void)an;
	fault_text("HTTP without std/http's exports", "");
}

static int cc_fill(ccon *c) { return (int)xo_x_cc_fill(XO_ADDR(c)); }

static int cc_send(ccon *c, const char *p, int64_t n) { return (int)xo_x_cc_send(XO_ADDR(c), XO_ADDR(p), n); }

// cr_fail sets r's error (hclient.xo fail): the ClientErr tag and text.
static void cr_fail(xo_cresp *r, int tag, const char *fmt, ...) {
	char b[512];
	va_list ap;
	va_start(ap, fmt);
	vsnprintf(b, sizeof b, fmt, ap);
	va_end(ap);
	xo_x_cl_fail(XO_ADDR(r), tag, XO_ADDR(b), (int64_t)strlen(b));
}

// cl_finish sets r's error for an exchange that ended with s (why: the
// failure's text; alert: a TLS alert of the server; hclient.xo finish).
static void cl_finish(xo_cresp *r, int s, const char *why, const char *alert) {
	xo_x_cl_finish(XO_ADDR(r), s, XO_ADDR(why), (int64_t)strlen(why), XO_ADDR(alert), alert ? (int64_t)strlen(alert) : -1);
}

// xo_cl_deadline is the deadline of a client's timeout (ns, > 0).
int64_t xo_cl_deadline(int64_t timeout) { return deadline(timeout); }

// xo_cl_start makes one request (Xo's client_do hop): method to addr
// (host:port; https: TLS with the server name tls_host and roots), the
// request target (escaped path and query), the Host, the header of names
// and values (canonical, each once), the body, and the deadline (0:
// none); prefix is the start of its errors. An https server whose ALPN
// chose h2 gets the request over HTTP/2 (xo_h2.c) on a connection kept
// for the next request with the same addr and tls_host (key), as
// net/http's Transport keeps it (pool: an idle one may take it): r is
// set up for the exchange, which Xo makes (cl_h2 is true; its header
// blocks are Xo's, http1.xo h2_exchange), and the response is then in r.
// Otherwise r is connected for HTTP/1.1 (reading): hclient.xo sends the
// request, reads the response, and closes (xo_cl_close). A failure sets
// r's error.
xo_cresp *xo_cl_start(const char *m, int64_t mn, int64_t https, const char *ap, int64_t an, const char *tp, int64_t tn, const char *gp,
                      int64_t gn, const char *hp, int64_t hn, xo_rlist *names, xo_rlist *values, const char *body, int64_t bn, int64_t when,
                      xo_troots *roots, const char *pp, int64_t pn, int64_t pool) {
	// the method, target, Host and header: Xo writes the requests (hclient.xo
	// request, http1.xo h2_exchange)
	(void)m, (void)mn, (void)gp, (void)gn, (void)hp, (void)hn, (void)names, (void)values;
	xo_cresp *r = xo_alloc(sizeof(xo_cresp));
	memset(r, 0, sizeof *r);
	h_init(r, cresp_free);
	r->prefix = cstr_of(pp, pn);
	char *addr = cstr_of(ap, an);
	char *tls_host = https ? cstr_of(tp, tn) : NULL;
	char *key = NULL;
	xo_conn *cn = NULL;
	ccon c = {0};
	if (https) {
		// an idle HTTP/2 connection to the same address, server name, and
		// roots
		nbuf k = {0};
		nbuf_str(&k, addr);
		nbuf_byte(&k, '|');
		nbuf_str(&k, tls_host);
		nbuf_byte(&k, 0);
		key = k.p;
		int64_t pc = pool ? xo_x_h2_pool_take(XO_ADDR(roots), XO_ADDR(key), (int64_t)strlen(key)) : 0;
		if (pc) {
			xo_x_h2_exchange_start(XO_ADDR(r), pc, 1, XO_ADDR(body), bn, when);
			goto out;
		}
	}
	int64_t d = when ? when - mono_ns() : 0;
	if (when && d <= 0) {
		cr_fail(r, 3, "");
		goto out;
	}
	cn = xo_net_dial_timeout(addr, (int64_t)strlen(addr), when ? d : 0);
	xo_ctx *x = ctx();
	if (x->net_status != 0) {
		int st = (int)x->net_status;
		const char *nm = x->net_msg ? x->net_msg : "";
		if (st == 5) {
			cr_fail(r, 5, "");
		} else if (st == 2) {
			cr_fail(r, 2, "dial tcp %s: connect: connection refused", addr);
		} else if (strstr(nm, "i/o timeout")) {
			cr_fail(r, 3, "");
		} else {
			cr_fail(r, 2, "%s", nm);
		}
		goto out;
	}
	c = (ccon){.pd = cn->pd, .fd = cn->pd->fd, .when = when};
	if (https) {
		// the TLS handshake (xo_tls.c): its failures are Protocol errors
		// with crypto/tls's texts, or Timeout and Canceled
		char tmsg[1200] = "";
		int kind = 4;
		int ts = tls_client_start(cn->pd, tls_host, roots, when, &c.tls, tmsg, sizeof tmsg, &kind);
		if (ts != CC_OK) {
			pd_close(c.pd);
			h_release(cn);
			if (ts == CC_TIMEOUT) {
				cr_fail(r, 3, "");
			} else if (ts == CC_CANCELED) {
				cr_fail(r, 5, "");
			} else if (kind == 2) {
				cr_fail(r, 2, "%s", tmsg);
			} else {
				cr_fail(r, 4, "%s: %s", r->prefix, tmsg);
			}
			goto out;
		}
	}
	if (c.tls && tls_alpn_h2(c.tls)) {
		int64_t pc = xo_x_h2_conn_new(XO_ADDR(cn), XO_ADDR(&c), XO_ADDR(roots), XO_ADDR(key), (int64_t)strlen(key));
		xo_x_h2_exchange_start(XO_ADDR(r), pc, 0, XO_ADDR(body), bn, when);
		goto out;
	}
	// Xo sends the request and reads the response (hclient.xo request)
	r->c = c;
	r->cn = cn;
	r->reading = 1;
	goto out;
out:
	free(key);
	free(addr);
	free(tls_host);
	return r;
}

// xo_cl_close closes r's connection (hclient.xo ends an HTTP/1.1
// exchange with it).
void xo_cl_close(xo_cresp *r) { cl_close(r); }

// The TLS connection's reads, writes and alert for hclient.xo.
int64_t xo_cl_tls_read(int64_t t, int64_t p, int64_t n) { return tls_read((tls_conn *)(intptr_t)t, (char *)(intptr_t)p, n); }

int64_t xo_cl_tls_write(int64_t t, int64_t p, int64_t n) { return tls_write((tls_conn *)(intptr_t)t, (const char *)(intptr_t)p, n); }

void xo_cl_tls_free(int64_t t) { tls_conn_free((tls_conn *)(intptr_t)t); }

void xo_cl_tls_set_when(int64_t t, int64_t when) { tls_set_when((tls_conn *)(intptr_t)t, when); }

xo_rstr *xo_cl_tls_alert(int64_t t, int64_t *len) {
	const char *a = tls_alert((tls_conn *)(intptr_t)t);
	return nstr(a ? a : "", a ? (int64_t)strlen(a) : 0, len);
}

// The records hclient.xo reads (R_*, C_*).
_Static_assert(offsetof(xo_cresp, err) == 16 && offsetof(xo_cresp, msg) == 24 && offsetof(xo_cresp, status) == 32 && offsetof(xo_cresp, hdr) == 40 &&
                   offsetof(xo_cresp, body) == 64 && offsetof(xo_cresp, h2) == 88 && offsetof(xo_cresp, retry) == 96 && offsetof(xo_cresp, reading) == 100 && offsetof(xo_cresp, c) == 104 && offsetof(xo_cresp, prefix) == 184,
               "hclient.xo R_*");
_Static_assert(offsetof(ccon, pd) == 0 && offsetof(ccon, fd) == 8 && offsetof(ccon, when) == 16 && offsetof(ccon, buf) == 24 && offsetof(ccon, n) == 32 &&
                   offsetof(ccon, cap) == 40 && offsetof(ccon, off) == 48 && offsetof(ccon, eof) == 56 && offsetof(ccon, tls) == 64,
               "hclient.xo C_*");
_Static_assert(sizeof(hpair) == 32 && offsetof(hlist, cap) == 16 && offsetof(nbuf, cap) == 16 && sizeof(ccon) == 72, "http1w.xo key_p, hclient.xo R_*_CAP, P_C");

// HTTP/2 (decision 0121)
#include "xo_h2.c"

// TLS (decision 0105)
#include "xo_tls.c"
