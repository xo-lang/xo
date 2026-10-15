// xo_http.c: the HTTP/1.1 server of the native std/http (decision 0074),
// included by xo_std.c.
//
// What the Go backend's server observably does comes from net/http (Go
// 1.26: server.go, request.go, transfer.go, textproto, net/url) and from
// std/rt/http.go, which reads the whole body before the handler runs and
// writes the response in one piece. This file reproduces it:
//
//   - reading a request (hc_read_one): the parsing is Xo's
//     (std/native/llrt/http1.xo: the request line, MIME headers with
//     continuation lines, Go's validation, the request target, chunked
//     bodies), the C reads the socket and the body and gives Go's error
//     answers (400 with or without a reason, 431, 501, 505, 417),
//     Expect: 100-continue, keep-alive (shouldClose), pipelining;
//   - writing a response (hw_response, whose rules are
//     std/native/llrt/http1w.xo since the zero-C audit's B12): chunkWriter.writeHeader with a
//     whole body: the status line, the handler's headers sorted, Date,
//     Content-Length when the body fits the 2048 byte buffer of
//     net/http (chunked above that, in 2048 byte chunks), Content-Type
//     sniffed (DetectContentType) when the handler set none, the
//     Connection header and whether the connection stays open;
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
// Writing a response
//
// The writer is std/native/llrt/http1w.xo (the zero-C audit's B12): the
// status text, DetectContentType, the Date header, and the rules of
// chunkWriter.writeHeader, through the C ABI exports of cabi_http.xo
// (decision 0204), which a program carries when it uses std/http. A
// program without std/http never reaches the server but links this C:
// the weak definitions here keep it linking, and fault if one ever ran.
// The C reserves the room each writes into.

__attribute__((weak)) int64_t xo_x_hw_response(int64_t d, int64_t ha, int64_t hn, int64_t status, int64_t body, int64_t bn, int64_t flags, int64_t res) {
	(void)d, (void)ha, (void)hn, (void)status, (void)body, (void)bn, (void)flags, (void)res;
	fault_text("HTTP without std/http's exports", "");
}

__attribute__((weak)) int64_t xo_x_hw_fields(int64_t d, int64_t ha, int64_t hn, int64_t excl) {
	(void)d, (void)ha, (void)hn, (void)excl;
	fault_text("HTTP without std/http's exports", "");
}

__attribute__((weak)) int64_t xo_x_hw_error(int64_t d, int64_t kind, int64_t code, int64_t rp, int64_t rn) {
	(void)d, (void)kind, (void)code, (void)rp, (void)rn;
	fault_text("HTTP without std/http's exports", "");
}

__attribute__((weak)) int64_t xo_x_hw_sniff(int64_t buf, int64_t p, int64_t n) {
	(void)buf, (void)p, (void)n;
	fault_text("HTTP without std/http's exports", "");
}

__attribute__((weak)) int64_t xo_x_hw_date(int64_t buf) {
	(void)buf;
	fault_text("HTTP without std/http's exports", "");
}

// http1w.xo's request flags (F_*), results (R_*), and left out fields (X_*)
enum {
	HW_F_HTTP11 = 1,
	HW_F_HEAD = 2,
	HW_F_H2UP = 4,
	HW_F_WANTS10KA = 8,
	HW_F_WANTSCLOSE = 16,
	HW_F_HAS_BODY = 32,
	HW_F_EXPECT_UNREAD = 64,
	HW_F_BODY_BAD = 128,
	HW_F_TOO_BIG = 256,
	HW_F_STREAM = 512,
	HW_R_KEEP = 1,
	HW_R_CHUNKED = 2,
	HW_X_CONTENT_LENGTH = 1,
	HW_X_TRANSFER_ENCODING = 2,
	HW_X_HOST = 16,
	HW_X_USER_AGENT = 32,
	HW_X_TRAILER = 64,
};

// h_sniff is net/http's DetectContentType of p (http1w.xo), written into
// buf (64 bytes).
static const char *h_sniff(const char *p, int64_t n, char *buf) {
	xo_x_hw_sniff(XO_ADDR(buf), XO_ADDR(p), n);
	return buf;
}

// h_date writes the Date header value of now (http.TimeFormat) and a
// zero byte at out (32 bytes).
static void h_date(char *out) { xo_x_hw_date(XO_ADDR(out)); }

static int h_body_allowed(int64_t st) { return !((st >= 100 && st <= 199) || st == 204 || st == 304); }

// hw_fields_room is the room hw_fields needs for h: its names and values
// with 4 bytes each.
static int64_t hw_fields_room(hlist *h) {
	int64_t n = 0;
	for (int64_t i = 0; i < h->n; i++) {
		n += h->a[i].k.n + h->a[i].v.n + 4;
	}
	return n;
}

// hw_fields writes the fields of h but those of excl (HW_X_*) sorted by
// name, as Header.writeSubset (http1w.fields).
static void hw_fields(nbuf *o, hlist *h, int64_t excl) {
	if (h->n == 0) {
		return;
	}
	nbuf_grow(o, hw_fields_room(h));
	o->n += xo_x_hw_fields(XO_ADDR(o->p + o->n), XO_ADDR(h->a), h->n, excl);
}

// hw_response_x writes the response to request r (handler header h,
// status, body of bn bytes) as net/http does when a handler sets the
// header and writes the whole body at once (std/rt's serve), or with
// stream the head of a stream with its first write, and reports whether
// the connection stays open (http1w.response). expect_unread: the
// request expected 100-continue and its body was not read to the end;
// body_bad: reading the request body failed; too_big: 256 KiB or more of
// a Content-Length body were never sent. *chunked: the body goes chunked.
static int hw_response_x(nbuf *o, hreq *r, hlist *h, int64_t status, const char *body, int64_t bn, int expect_unread, int body_bad, int too_big, int stream, int *chunked) {
	int64_t f = 0;
	if (r->major > 1 || (r->major == 1 && r->minor >= 1)) {
		f |= HW_F_HTTP11;
	}
	f |= hs_eq(r->method, "HEAD") ? HW_F_HEAD : 0;
	f |= r->h2up ? HW_F_H2UP : 0;
	f |= r->wants10ka ? HW_F_WANTS10KA : 0;
	f |= r->wantsclose ? HW_F_WANTSCLOSE : 0;
	f |= r->clen != 0 ? HW_F_HAS_BODY : 0;
	f |= expect_unread ? HW_F_EXPECT_UNREAD : 0;
	f |= body_bad ? HW_F_BODY_BAD : 0;
	f |= too_big ? HW_F_TOO_BIG : 0;
	f |= stream ? HW_F_STREAM : 0;
	// the room: the status lines and the fields the server adds, the
	// handler's fields twice (an informational status repeats them), the
	// body, and a chunk's size line and end per 2048 bytes
	nbuf_grow(o, 640 + 2 * hw_fields_room(h) + bn + 24 * (bn / 2048 + 2));
	int64_t res = 0;
	o->n += xo_x_hw_response(XO_ADDR(o->p + o->n), XO_ADDR(h->a), h->n, status, XO_ADDR(body), bn, f, XO_ADDR(&res));
	if (chunked) {
		*chunked = (res & HW_R_CHUNKED) != 0;
	}
	return (res & HW_R_KEEP) != 0;
}

static int hw_response(nbuf *o, hreq *r, hlist *h, int64_t status, const char *body, int64_t bn, int expect_unread, int body_bad, int too_big) {
	return hw_response_x(o, r, h, status, body, bn, expect_unread, body_bad, too_big, 0, NULL);
}

// hw_error is the answer of conn.serve to a request it could not read
// (http1w.error_answer, at most 255 bytes).
static void hw_error(nbuf *o, int kind, int code, const char *reason) {
	const char *rs = kind == HP_STATUS && reason ? reason : "";
	nbuf_grow(o, 256);
	o->n += xo_x_hw_error(XO_ADDR(o->p + o->n), kind, code, XO_ADDR(rs), (int64_t)strlen(rs));
}

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
	int eof;
	int last_post;
	int served; // a response was written
	hreq req;
	int have_req;
	nbuf body;
	int body_bad;
	int too_big;
	int expect_unread;
	hlist resp;
	nbuf out; // the response being written (reused)
	char ip[INET6_ADDRSTRLEN];
	// tests (http_diff.c): no socket, no server; the input is in buf
	// and the output goes to sink
	nbuf *sink;
	tls_conn *tls; // a connection of a TLS server (xo_tls.c)
	// HTTP/2 (xo_h2.c): a connection whose ALPN chose h2 has h2; a
	// stream's request (h2_take) has h2s and holds its connection in
	// h2owner
	struct h2c *h2;
	struct h2s *h2s;
	struct xo_hconn *h2owner;
	// the parsing in Xo (decision 0193 step 6): where hc_read_one goes
	// on, and the result Xo passed (xo_hc_head, xo_hc_chunked)
	int rd_phase;
	int rd_have, rd_res;
	int64_t rd_used;
	char *rd_reason;
} xo_hconn;

enum { RD_START, RD_HEAD, RD_BODY };

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
		free(hc->out.p);
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
	free(hc->rd_reason);
	free(hc->body.p);
	free(hc->out.p);
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

// hc_fill reads more input: bytes read, 0 at the end of the stream, -1
// when the connection was closed or the task canceled.
static int64_t hc_fill(xo_hconn *hc) {
	if (!hc->conn) {
		return 0;
	}
	xo_ctx *c = ctx();
	xo_pd *pd = hc->conn->pd;
	if (hc->off > 0 && hc->off == hc->n) {
		hc->off = hc->n = 0;
	}
	if (hc->cap - hc->n < 4096) {
		if (hc->off > 0) {
			memmove(hc->buf, hc->buf + hc->off, (size_t)(hc->n - hc->off));
			hc->n -= hc->off;
			hc->off = 0;
		}
		if (hc->cap - hc->n < 4096) {
			hc->cap = hc->cap ? hc->cap * 2 : 8192;
			hc->buf = realloc(hc->buf, (size_t)hc->cap);
			if (!hc->buf) {
				xo_oom();
			}
		}
	}
	if (hc->tls) {
		// the handshake first, then decrypted bytes (xo_tls.c)
		if (ctx_canceled(c)) {
			return -1;
		}
		int64_t r = tls_hc_read(hc->tls, hc->buf + hc->n, hc->cap - hc->n);
		if (r > 0) {
			hc->n += r;
		}
		return r;
	}
	if (ctx_canceled(c) || !pd_begin(pd)) {
		return -1;
	}
	for (;;) {
		int64_t r = x_read(pd->fd, hc->buf + hc->n, (size_t)(hc->cap - hc->n));
		if (r >= 0) {
			pd_end(pd);
			hc->n += r;
			return r;
		}
		if (r == -EINTR) {
			continue;
		}
		if (r == -EAGAIN || r == -EWOULDBLOCK) {
			if (pd_wait(pd, 0, c) == PD_READY) {
				continue;
			}
		}
		pd_end(pd);
		return -1;
	}
}

// hc_send writes the bytes of o: 0 when the connection broke.
static int hc_send(xo_hconn *hc, const char *p, int64_t n) {
	if (hc->sink) {
		nbuf_put(hc->sink, p, n);
		return 1;
	}
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

// hc_read_one reads a request: 1 when it is ready for the handler, 2
// when it was answered here (OPTIONS *), 0 when the connection is done.
// The parsing is Xo's (std/native/llrt/http1.xo): also 3, a request
// head to parse at the unread input, and 4, a chunked body to decode
// there: the fiber's Xo code parses and passes the result
// (xo_hc_head, xo_hc_chunked), then calls again, which goes on where it
// stopped (hc->rd_phase).
static int64_t hc_read_one(xo_hconn *hc) {
	xo_srv *s = hc->srv;
	int64_t max = http_max_body();
	int skip = 0, peek = 0, short_body = 0;
	hreq *q = &hc->req;
	int phase = hc->rd_phase;
	hc->rd_phase = RD_START;
	if (phase == RD_BODY) {
		goto body;
	}
	if (phase == RD_START) {
		if (hc->have_req) {
			hr_free(&hc->req);
			hc->have_req = 0;
		}
		hc->body.n = 0;
		hc->body_bad = 0;
		hc->too_big = 0;
		hc->expect_unread = 0;
		hl_clear(&hc->resp);
		// after a POST, up to 4 CR or LF bytes before the next request
		// (conn.readRequest's tolerance for old clients)
		skip = hc->last_post;
		// after a response, conn.serve peeks 4 bytes before it reads the
		// next request: fewer at the end of the stream close the
		// connection
		peek = hc->served;
	}
	for (;;) {
		int64_t have = hc->n - hc->off;
		if (peek) {
			if (have >= 4) {
				peek = 0;
			} else if (hc->eof) {
				return 0;
			}
		}
		if (!peek && skip && (have >= 4 || hc->eof)) {
			for (int k = 0; k < 4 && hc->off < hc->n && (hc->buf[hc->off] == '\r' || hc->buf[hc->off] == '\n'); k++) {
				hc->off++;
			}
			skip = 0;
		}
		int64_t used = 0;
		int r;
		if (peek || skip) {
			r = HP_NEED;
		} else {
			if (!hc->rd_have) {
				hc->rd_phase = RD_HEAD;
				return 3;
			}
			hc->rd_have = 0;
			r = hc->rd_res;
			used = hc->rd_used;
		}
		if (r == HP_NEED) {
			if (hc->eof) {
				skip = 0;
				continue;
			}
			// wait for more; idle when nothing of a request came yet
			if (s) {
				pthread_mutex_lock(&s->mu);
				if (s->stopping && hc->off == hc->n) {
					pthread_mutex_unlock(&s->mu);
					return 0;
				}
				hc->idle = hc->off == hc->n;
				pthread_mutex_unlock(&s->mu);
			}
			int64_t got = hc_fill(hc);
			if (s) {
				pthread_mutex_lock(&s->mu);
				hc->idle = 0;
				pthread_mutex_unlock(&s->mu);
			}
			if (got < 0) {
				return 0;
			}
			if (got == 0) {
				hc->eof = 1;
			}
			continue;
		}
		if (r == HP_EOF) {
			return 0;
		}
		if (r != HP_OK) {
			nbuf o = {0};
			hw_error(&o, r, hc->req.status, hc->req.reason);
			if (r == HP_STATUS) {
				hr_free(&hc->req);
			}
			hc_send(hc, o.p, o.n);
			free(o.p);
			if (r == HP_TOO_LARGE && hc->conn) {
				nx_shutdown(hc->conn->pd->fd, SHUT_WR);
			}
			return 0;
		}
		hc->have_req = 1;
		hc->off += used;
		break;
	}
	hc->last_post = hs_eq(q->method, "POST");
	if (q->expect_other) {
		// sendExpectationFailed
		hlist h = {0};
		hl_add(&h, hs_dup("Connection", 10), hs_dup("close", 5));
		nbuf o = {0};
		hw_response(&o, q, &h, 417, "", 0, 0, 0, 0);
		hl_free(&h);
		hc_send(hc, o.p, o.n);
		free(o.p);
		return 0;
	}
	if (q->clen != 0) {
		if (q->expect100) {
			hc_send(hc, "HTTP/1.1 100 Continue\r\n\r\n", 25);
		}
	body:
		for (;;) {
			int64_t have = hc->n - hc->off;
			if (q->chunked) {
				int64_t used = 0;
				if (!hc->rd_have) {
					hc->rd_phase = RD_BODY;
					return 4;
				}
				hc->rd_have = 0;
				int b = hc->rd_res;
				used = hc->rd_used;
				if (b == HB_OK) {
					hc->off += used;
					break;
				}
				if (b == HB_ERR || hc->body.n > max) {
					// a sticky error of the chunked reader: net/http
					// closes the connection after the response
					hc->body_bad = 1;
					hc->off = hc->n;
					break;
				}
			} else {
				int64_t want = q->clen < max ? q->clen : max;
				if (have >= want) {
					nbuf_put(&hc->body, hc->buf + hc->off, want);
					hc->off += want;
					if (q->clen > max) {
						hc->too_big = 1;
					}
					break;
				}
				if (hc->eof) {
					// the client ended the body early: the handler gets
					// what came (std/rt ignores the error)
					nbuf_put(&hc->body, hc->buf + hc->off, have);
					hc->off = hc->n;
					short_body = 1;
					hc->too_big = q->clen - have >= 256 << 10;
					break;
				}
			}
			int64_t got = hc_fill(hc);
			if (got < 0) {
				return 0;
			}
			if (got == 0) {
				hc->eof = 1;
			}
		}
		// the 100-continue reader saw no end of the body: net/http
		// closes after the response
		hc->expect_unread = q->expect100 && (short_body || hc->body_bad);
	}
	if (hs_eq(q->method, "OPTIONS") && hs_eq(q->target, "*")) {
		// net/http answers it itself (globalOptionsHandler), reading at
		// most 4 KiB of its body
		hlist h = {0};
		hl_add(&h, hs_dup("Content-Length", 14), hs_dup("0", 1));
		if (hc->body.n > 4096) {
			// MaxBytesReader: requestTooLarge sets the handler's header
			hl_add(&h, hs_dup("Connection", 10), hs_dup("close", 5));
		}
		nbuf o = {0};
		int keep = hw_response(&o, q, &h, 200, "", 0, hc->expect_unread, hc->body_bad, hc->too_big);
		hl_free(&h);
		int ok = hc_send(hc, o.p, o.n);
		free(o.p);
		hc->served = 1;
		return ok && keep ? 2 : 0;
	}
	return 1;
}

// xo_hc_read reads the next request for the handler: 1 when one is
// ready (its body read), 0 when the connection is done (the end of the
// stream, an error answered, the server stopping, the task canceled);
// also 3 and 4 for the parsing in Xo (hc_read_one).
int64_t xo_hc_read(xo_hconn *hc) {
	for (;;) {
		int64_t r = hc_read_one(hc);
		if (r != 2) {
			return r;
		}
	}
}

// head_len is the length of p[:n] up to the end of its first head (the
// first empty line: \n\n or \n\r\n), or n when it has none yet.
static int64_t head_len(const char *p, int64_t n) {
	for (const char *nl = memchr(p, '\n', (size_t)n); nl; nl = memchr(nl + 1, '\n', (size_t)(n - (nl + 1 - p)))) {
		int64_t e = nl + 1 - p;
		if (e < n && p[e] == '\n') {
			return e + 1;
		}
		if (e + 1 < n && p[e] == '\r' && p[e + 1] == '\n') {
			return e + 2;
		}
	}
	return n;
}

// xo_hc_input is the unread input (what hc_read stopped at), or a part
// of it whose parse is the whole's (pipelined requests would otherwise be
// copied again for each): mode 0 up to the end of the first head
// (head_len), mode 1 at most 4 KiB (a chunked body's first try), 2 all.
xo_rstr *xo_hc_input(xo_hconn *hc, int64_t mode, int64_t *len) {
	const char *p = hc->buf ? hc->buf + hc->off : "";
	int64_t n = hc->n - hc->off;
	if (mode == 0) {
		n = head_len(p, n);
	} else if (mode == 1 && n > 4096) {
		n = 4096;
	}
	return nstr(p, n, len);
}

// xo_hc_avail is the length of the unread input.
int64_t xo_hc_avail(xo_hconn *hc) { return hc->n - hc->off; }

int64_t xo_hc_eof(xo_hconn *hc) { return hc->eof; }

// xo_hc_head takes the parse of a head: its result (HP_*) and length,
// and for HP_OK what the C reads the body and answers with (the method,
// the version in the low 8 bits of flags, then chunked, close,
// wants10ka, wantsclose, expect100, expect_other, h2up, and a target of
// "*" from bit 8 on; the Content-Length), for HP_STATUS the code and
// text.
void xo_hc_head(xo_hconn *hc, int64_t res, int64_t used, const char *m, int64_t mn, int64_t flags, int64_t clen, int64_t status, const char *rs, int64_t rn) {
	hreq *r = &hc->req;
	hr_free(r);
	free(hc->rd_reason);
	hc->rd_reason = NULL;
	hc->rd_res = (int)res;
	hc->rd_used = used;
	hc->rd_have = 1;
	if (res == HP_STATUS) {
		r->status = (int)status;
		hc->rd_reason = cstr_of(rs, rn);
		r->reason = hc->rd_reason;
	}
	if (res != HP_OK) {
		return;
	}
	r->method = hs_dup(m, mn);
	r->target = flags & 0x8000 ? hs_dup("*", 1) : hs_dup("", 0);
	r->major = (int)(flags & 15);
	r->minor = (int)((flags >> 4) & 15);
	r->chunked = (flags & 0x100) != 0;
	r->close = (flags & 0x200) != 0;
	r->wants10ka = (flags & 0x400) != 0;
	r->wantsclose = (flags & 0x800) != 0;
	r->expect100 = (flags & 0x1000) != 0;
	r->expect_other = (flags & 0x2000) != 0;
	r->h2up = (flags & 0x4000) != 0;
	r->clen = clen;
}

// xo_hc_chunked takes the decoding of a chunked body: its result (HB_*),
// its length, and the data decoded (before an error too).
void xo_hc_chunked(xo_hconn *hc, int64_t res, int64_t used, const char *p, int64_t n) {
	hc->body.n = 0;
	nbuf_put(&hc->body, p, n);
	hc->rd_res = (int)res;
	hc->rd_used = used;
	hc->rd_have = 1;
}

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

// xo_hc_respond writes the response: 1 when the connection stays open
// for another request.
int64_t xo_hc_respond(xo_hconn *hc, int64_t status, const char *body, int64_t bn) {
	if (hc->h2s) {
		return h2_respond(hc, status, body, bn);
	}
	nbuf *o = &hc->out;
	o->n = 0;
	if (bn > 2048 && h_body_allowed(status) && hl_find(&hc->resp, "Content-Length") < 0) {
		// std/rt sets Content-Length on a whole body net/http would
		// otherwise chunk (above its 2048 byte buffer); match it
		char cl[32];
		int cn = fmt_int(cl, bn);
		hl_add(&hc->resp, hs_dup("Content-Length", 14), hs_dup(cl, cn));
	}
	int keep = hw_response(o, &hc->req, &hc->resp, status, body, bn, hc->expect_unread, hc->body_bad, hc->too_big);
	int ok = hc_send(hc, o->p, o->n);
	if (o->cap > (64 << 10)) {
		// a large body: do not keep its buffer for the connection's life
		free(o->p);
		*o = (nbuf){0};
	}
	hl_clear(&hc->resp);
	hc->served = 1;
	if (!ok || !keep) {
		return 0;
	}
	if (!hc->srv) {
		return 1;
	}
	pthread_mutex_lock(&hc->srv->mu);
	int stopping = hc->srv->stopping;
	pthread_mutex_unlock(&hc->srv->mu);
	return !stopping;
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
	int started, chunking, raw, keep, failed;
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
// first data, then a chunk per write.
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
	nbuf *o = &hc->out;
	o->n = 0;
	if (!s->started) {
		s->started = 1;
		int ch = 0;
		s->keep = hw_response_x(o, &hc->req, &hc->resp, s->status, p, n, hc->expect_unread, hc->body_bad, hc->too_big, 1, &ch);
		s->chunking = ch;
		s->raw = !ch && !hs_eq(hc->req.method, "HEAD") && h_body_allowed(s->status);
		hl_clear(&hc->resp);
	} else if (n > 0 && s->chunking) {
		char hx[24];
		snprintf(hx, sizeof hx, "%llx\r\n", (long long)n);
		nbuf_str(o, hx);
		nbuf_put(o, p, n);
		nbuf_put(o, "\r\n", 2);
	} else if (n > 0 && s->raw) {
		nbuf_put(o, p, n);
	}
	if (o->n > 0 && !hc_send(hc, o->p, o->n)) {
		s->failed = 1;
	}
	o->n = 0;
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
	if (s->buf.n > 0) {
		sink_emit(s, s->buf.p, s->buf.n);
		s->buf.n = 0;
	}
	if (s->chunking && !s->failed && !hc_send(hc, "0\r\n\r\n", 5)) {
		s->failed = 1;
	}
	if (hc->out.cap > (64 << 10)) {
		free(hc->out.p);
		hc->out = (nbuf){0};
	}
	hc->served = 1;
	if (s->failed || !s->keep) {
		return 0;
	}
	if (!hc->srv) {
		return 1;
	}
	pthread_mutex_lock(&hc->srv->mu);
	int stopping = hc->srv->stopping;
	pthread_mutex_unlock(&hc->srv->mu);
	return !stopping;
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

static void h2_pool_free(void);

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
	h2_pool_free();
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
	struct h2x *h2; // an HTTP/2 exchange Xo drives (http1.xo h2_exchange)
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

// HTTP/2 (xo_h2.c)
typedef struct h2cc h2cc;
typedef struct h2x h2x;
static h2cc *h2_pool_take(xo_troots *roots, const char *key);
static h2cc *h2cc_new(xo_conn *cn, ccon *c, xo_troots *roots, const char *key);
static void h2x_start(xo_cresp *r, h2cc *pc, int pooled, const char *method, const char *body, int64_t bn, int64_t when, hlist *hdr);
static void h2x_free(h2x *x);

static void cresp_free(void *p) {
	xo_cresp *r = p;
	h2x_free(r->h2);
	cl_close(r);
	free(r->msg);
	free(r->prefix);
	hl_free(&r->hdr);
	free(r->body.p);
	xo_free(r);
}

static int cc_fill(ccon *c) {
	if (c->fd < 0) {
		// tests (http_fuzz.c): the whole response is in buf
		c->eof = 1;
		return CC_EOF;
	}
	xo_ctx *x = ctx();
	if (c->cap - c->n < 4096) {
		if (c->off > 0) {
			memmove(c->buf, c->buf + c->off, (size_t)(c->n - c->off));
			c->n -= c->off;
			c->off = 0;
		}
		if (c->cap - c->n < 4096) {
			c->cap = c->cap ? c->cap * 2 : 8192;
			c->buf = realloc(c->buf, (size_t)c->cap);
			if (!c->buf) {
				xo_oom();
			}
		}
	}
	if (c->tls) {
		if (ctx_canceled(x)) {
			return CC_CANCELED;
		}
		int64_t r = tls_read(c->tls, c->buf + c->n, c->cap - c->n);
		if (r > 0) {
			c->n += r;
			return CC_OK;
		}
		if (r == 0) {
			c->eof = 1;
			return CC_EOF;
		}
		return (int)-r;
	}
	for (;;) {
		if (ctx_canceled(x)) {
			return CC_CANCELED;
		}
		int64_t r = x_read(c->fd, c->buf + c->n, (size_t)(c->cap - c->n));
		if (r > 0) {
			c->n += r;
			return CC_OK;
		}
		if (r == 0) {
			c->eof = 1;
			return CC_EOF;
		}
		if (r == -EINTR) {
			continue;
		}
		if (r == -EAGAIN || r == -EWOULDBLOCK) {
			int w = pd_wait_until(c->pd, 0, x, c->when);
			if (w == PD_READY) {
				continue;
			}
			return w == PD_TIMEOUT ? CC_TIMEOUT : w == PD_CANCELED ? CC_CANCELED : CC_ERR;
		}
		return CC_ERR;
	}
}

static int cc_send(ccon *c, const char *p, int64_t n) {
	if (c->tls) {
		return tls_write(c->tls, p, n);
	}
	xo_ctx *x = ctx();
	int64_t off = 0;
	while (off < n) {
		int64_t w = x_write(c->fd, p + off, (size_t)(n - off));
		if (w >= 0) {
			off += w;
			continue;
		}
		if (w == -EINTR) {
			continue;
		}
		if (w == -EAGAIN || w == -EWOULDBLOCK) {
			int r = pd_wait_until(c->pd, 1, x, c->when);
			if (r == PD_READY) {
				continue;
			}
			return r == PD_TIMEOUT ? CC_TIMEOUT : r == PD_CANCELED ? CC_CANCELED : CC_ERR;
		}
		return CC_ERR;
	}
	return CC_OK;
}

static void cr_fail(xo_cresp *r, int tag, const char *fmt, ...) {
	char b[512];
	va_list ap;
	va_start(ap, fmt);
	vsnprintf(b, sizeof b, fmt, ap);
	va_end(ap);
	free(r->msg);
	r->msg = strdup(b);
	r->err = tag;
}

// cl_finish sets r's error for an exchange that ended with s (why: the
// failure's text; alert: a TLS alert of the server).
static void cl_finish(xo_cresp *r, int s, const char *why, const char *alert) {
	switch (s) {
	case CC_OK:
		break;
	case CC_TIMEOUT:
		cr_fail(r, 3, "");
		break;
	case CC_CANCELED:
		cr_fail(r, 5, "");
		break;
	default:
		if (alert) {
			cr_fail(r, 2, "%s", alert);
		} else if (why[0] == '\1') {
			// a failure reading an HTTP/2 body: net/http's body Read
			// error, without the request
			cr_fail(r, 4, "%s", why + 1);
		} else {
			cr_fail(r, 4, "%s: %s", r->prefix, why[0] ? why : "EOF");
		}
	}
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
// Otherwise the request goes out over HTTP/1.1 and cl_reading is true:
// Xo reads the response and calls cl_end. A failure sets r's error.
xo_cresp *xo_cl_start(const char *m, int64_t mn, int64_t https, const char *ap, int64_t an, const char *tp, int64_t tn, const char *gp,
                      int64_t gn, const char *hp, int64_t hn, xo_rlist *names, xo_rlist *values, const char *body, int64_t bn, int64_t when,
                      xo_troots *roots, const char *pp, int64_t pn, int64_t pool) {
	xo_cresp *r = xo_alloc(sizeof(xo_cresp));
	memset(r, 0, sizeof *r);
	h_init(r, cresp_free);
	r->prefix = cstr_of(pp, pn);
	char *method = cstr_of(m, mn);
	char *addr = cstr_of(ap, an);
	char *tls_host = https ? cstr_of(tp, tn) : NULL;
	hstr target = hs_dup(gp, gn);
	hstr host = hs_dup(hp, hn);
	hlist hdr = {0};
	for (int64_t i = 0; i < names->len && i < values->len; i++) {
		xo_rstr *k = ((xo_rstr **)rl_data(names))[2 * i];
		int64_t kn = ((int64_t *)rl_data(names))[2 * i + 1];
		xo_rstr *v = ((xo_rstr **)rl_data(values))[2 * i];
		int64_t vn = ((int64_t *)rl_data(values))[2 * i + 1];
		hl_add(&hdr, hs_dup(k->data, kn), hs_dup(v->data, vn));
	}
	char why[400] = "";
	char *alert = NULL;
	char *key = NULL;
	int s = CC_OK;
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
		h2cc *pc = pool ? h2_pool_take(roots, key) : NULL;
		if (pc) {
			h2x_start(r, pc, 1, method, body, bn, when, &hdr);
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
		h2x_start(r, h2cc_new(cn, &c, roots, key), 0, method, body, bn, when, &hdr);
		goto out;
	}
	// the request (net/http Request.write with Transport's additions)
	nbuf o = {0};
	nbuf_str(&o, method);
	nbuf_byte(&o, ' ');
	nbuf_put(&o, target.p, target.n);
	nbuf_str(&o, " HTTP/1.1\r\nHost: ");
	nbuf_put(&o, host.p, host.n);
	nbuf_str(&o, "\r\n");
	int64_t ua = hl_find(&hdr, "User-Agent");
	if (ua < 0) {
		nbuf_str(&o, "User-Agent: Go-http-client/1.1\r\n");
	} else if (hdr.a[ua].v.n > 0) {
		nbuf_str(&o, "User-Agent: ");
		nbuf_put(&o, hdr.a[ua].v.p, hdr.a[ua].v.n);
		nbuf_str(&o, "\r\n");
	}
	if (bn > 0 || strcmp(method, "POST") == 0 || strcmp(method, "PUT") == 0 || strcmp(method, "PATCH") == 0) {
		char cl[48];
		snprintf(cl, sizeof cl, "Content-Length: %lld\r\n", (long long)bn);
		nbuf_str(&o, cl);
	}
	hw_fields(&o, &hdr, HW_X_HOST | HW_X_USER_AGENT | HW_X_CONTENT_LENGTH | HW_X_TRANSFER_ENCODING | HW_X_TRAILER);
	if (hl_find(&hdr, "Accept-Encoding") < 0 && hl_find(&hdr, "Range") < 0 && strcmp(method, "HEAD") != 0) {
		nbuf_str(&o, "Accept-Encoding: gzip\r\n");
	}
	nbuf_str(&o, "\r\n");
	nbuf_put(&o, body, bn);
	r->c = c;
	r->cn = cn;
	s = cc_send(&r->c, o.p, o.n);
	free(o.p);
	if (s == CC_OK) {
		r->reading = 1;
		goto out;
	}
	{
		// an alert of the server after the handshake (a TLS 1.3 server
		// that wanted a client certificate): Go's net.OpError, a Connect
		// error
		const char *ta = s != CC_TIMEOUT && s != CC_CANCELED ? tls_alert(r->c.tls) : NULL;
		alert = ta ? strdup(ta) : NULL;
	}
	cl_close(r);
finish:
	cl_finish(r, s, why, alert);
out:
	free(alert);
	free(key);
	free(method);
	free(addr);
	free(tls_host);
	hs_free(&target);
	hs_free(&host);
	hl_free(&hdr);
	return r;
}

// xo_cl_reading: the request went out over HTTP/1.1 and Xo reads the
// response (cl_input, cl_fill, cl_take), then calls cl_end.
int64_t xo_cl_reading(xo_cresp *r) { return r->reading; }

// xo_cl_input is the unread input up to the end of its first head (the
// first empty line, mode 0, as xo_hc_input) or all of it (mode 2).
xo_rstr *xo_cl_input(xo_cresp *r, int64_t mode, int64_t *len) {
	const char *p = r->c.buf ? r->c.buf + r->c.off : "";
	int64_t n = r->c.n - r->c.off;
	return nstr(p, mode == 0 ? head_len(p, n) : n, len);
}

int64_t xo_cl_avail(xo_cresp *r) { return r->c.n - r->c.off; }

int64_t xo_cl_eof(xo_cresp *r) { return r->c.eof; }

// xo_cl_fill reads more of the response: CC_OK, CC_EOF (none came),
// or the failure (CC_TIMEOUT, CC_CANCELED, CC_ERR).
int64_t xo_cl_fill(xo_cresp *r) { return r->c.eof ? CC_EOF : cc_fill(&r->c); }

// xo_cl_take takes n bytes of the input (n <= cl_avail).
xo_rstr *xo_cl_take(xo_cresp *r, int64_t n, int64_t *len) {
	xo_rstr *s = nstr(r->c.buf ? r->c.buf + r->c.off : "", n, len);
	r->c.off += n;
	return s;
}

void xo_cl_skip(xo_cresp *r, int64_t n) { r->c.off += n; }

// xo_cl_end ends the exchange Xo read with s (CC_*; why: the failure's
// text): the connection closes and a failure becomes r's error.
void xo_cl_end(xo_cresp *r, int64_t s, const char *wp, int64_t wn) {
	char why[400];
	snprintf(why, sizeof why, "%.*s", (int)wn, wp);
	char *alert = NULL;
	if (s != CC_OK && s != CC_TIMEOUT && s != CC_CANCELED && !why[0]) {
		const char *ta = tls_alert(r->c.tls);
		alert = ta ? strdup(ta) : NULL;
	}
	cl_close(r);
	cl_finish(r, (int)s, why, alert);
	free(alert);
}

int64_t xo_cr_err(xo_cresp *r) { return r->err; }

xo_rstr *xo_cr_msg(xo_cresp *r, int64_t *len) { return nstr(r->msg ? r->msg : "", r->msg ? (int64_t)strlen(r->msg) : 0, len); }

int64_t xo_cr_status(xo_cresp *r) { return r->status; }

int64_t xo_cr_nheaders(xo_cresp *r) { return r->hdr.n; }

xo_rstr *xo_cr_hname(xo_cresp *r, int64_t i, int64_t *len) { return hstr_out(r->hdr.a[i].k, len); }

xo_rstr *xo_cr_hval(xo_cresp *r, int64_t i, int64_t *len) { return hstr_out(r->hdr.a[i].v, len); }

xo_rstr *xo_cr_body(xo_cresp *r, int64_t *len) { return nstr(r->body.p ? r->body.p : "", r->body.n, len); }

// HTTP/2 (decision 0121)
#include "xo_h2.c"

// TLS (decision 0105)
#include "xo_tls.c"
