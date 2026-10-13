// xo_http.c: the HTTP/1.1 server of the native std/http (decision 0074),
// included by xo_std.c.
//
// What the Go backend's server observably does comes from net/http (Go
// 1.26: server.go, request.go, transfer.go, textproto, net/url) and from
// std/rt/http.go, which reads the whole body before the handler runs and
// writes the response in one piece. This file reproduces it:
//
//   - reading a request (hp_head, hp_body): the request line, MIME
//     headers with continuation lines, Go's validation and its error
//     answers (400 with or without a reason, 431, 501, 505, 417),
//     Content-Length and chunked bodies, Expect: 100-continue, keep-alive
//     (shouldClose), pipelining, and the request target (ParseRequestURI,
//     ParseQuery);
//   - writing a response (hw_response): chunkWriter.writeHeader with a
//     whole body: the status line, the handler's headers sorted, Date,
//     Content-Length when the body fits the 2048 byte buffer of
//     net/http (chunked above that, in 2048 byte chunks), Content-Type
//     sniffed (DetectContentType) when the handler set none, the
//     Connection header and whether the connection stays open;
//   - the server (xo_srv, xo_hconn): connection tasks in a scope of their
//     own under main's root, so canceling serve lets requests in flight
//     finish (std/rt's reqRoot); a stop or the end of serve closes idle
//     connections and waits for the others.
//
// internal/llvm's TestHTTPParseDiff sends the same bytes to this code and
// to a Go net/http server and compares what both answer; the parser is
// fuzzed there with AddressSanitizer.

// Limits: Go's DefaultMaxHeaderBytes plus the 4096 bytes of slack its
// server allows reads the request line and headers; more is 431.
#define HTTP_MAX_HEAD ((int64_t)(1 << 20) + 4096)
// Header fields per request (Go has no count limit: its bytes limit
// bounds them); more is 431.
#define HTTP_MAX_FIELDS 10000
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
static int h_vbyte(unsigned c) { return c >= 0x80 || (c >= 0x21 && c <= 0x7e) || c == ' ' || c == '\t'; }

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

// h_server_token is net/http's hasToken (server.go: wantsClose,
// expectsContinue, the Connection checks of writeHeader): tok anywhere
// in v between space, comma, or tab boundaries, ASCII case insensitive.
static int h_server_token(const char *v, int64_t n, const char *tok) {
	int64_t tl = (int64_t)strlen(tok);
	if (tl > n || tl == 0) {
		return 0;
	}
	for (int64_t sp = 0; sp + tl <= n; sp++) {
		unsigned b = (unsigned char)v[sp];
		if (b != (unsigned char)tok[0] && (b | 0x20) != (unsigned char)tok[0]) {
			continue;
		}
		if (sp > 0 && !(v[sp - 1] == ' ' || v[sp - 1] == ',' || v[sp - 1] == '\t')) {
			continue;
		}
		int64_t e = sp + tl;
		if (e != n && !(v[e] == ' ' || v[e] == ',' || v[e] == '\t')) {
			continue;
		}
		int eq = 1;
		for (int64_t k = 0; k < tl; k++) {
			if (h_lower((unsigned char)v[sp + k]) != h_lower((unsigned char)tok[k])) {
				eq = 0;
				break;
			}
		}
		if (eq) {
			return 1;
		}
	}
	return 0;
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

static const char *h_status_text(int64_t c) {
	switch (c) {
	case 100: return "Continue";
	case 101: return "Switching Protocols";
	case 102: return "Processing";
	case 103: return "Early Hints";
	case 200: return "OK";
	case 201: return "Created";
	case 202: return "Accepted";
	case 203: return "Non-Authoritative Information";
	case 204: return "No Content";
	case 205: return "Reset Content";
	case 206: return "Partial Content";
	case 207: return "Multi-Status";
	case 208: return "Already Reported";
	case 226: return "IM Used";
	case 300: return "Multiple Choices";
	case 301: return "Moved Permanently";
	case 302: return "Found";
	case 303: return "See Other";
	case 304: return "Not Modified";
	case 305: return "Use Proxy";
	case 307: return "Temporary Redirect";
	case 308: return "Permanent Redirect";
	case 400: return "Bad Request";
	case 401: return "Unauthorized";
	case 402: return "Payment Required";
	case 403: return "Forbidden";
	case 404: return "Not Found";
	case 405: return "Method Not Allowed";
	case 406: return "Not Acceptable";
	case 407: return "Proxy Authentication Required";
	case 408: return "Request Timeout";
	case 409: return "Conflict";
	case 410: return "Gone";
	case 411: return "Length Required";
	case 412: return "Precondition Failed";
	case 413: return "Request Entity Too Large";
	case 414: return "Request URI Too Long";
	case 415: return "Unsupported Media Type";
	case 416: return "Requested Range Not Satisfiable";
	case 417: return "Expectation Failed";
	case 418: return "I'm a teapot";
	case 421: return "Misdirected Request";
	case 422: return "Unprocessable Entity";
	case 423: return "Locked";
	case 424: return "Failed Dependency";
	case 425: return "Too Early";
	case 426: return "Upgrade Required";
	case 428: return "Precondition Required";
	case 429: return "Too Many Requests";
	case 431: return "Request Header Fields Too Large";
	case 451: return "Unavailable For Legal Reasons";
	case 500: return "Internal Server Error";
	case 501: return "Not Implemented";
	case 502: return "Bad Gateway";
	case 503: return "Service Unavailable";
	case 504: return "Gateway Timeout";
	case 505: return "HTTP Version Not Supported";
	case 506: return "Variant Also Negotiates";
	case 507: return "Insufficient Storage";
	case 508: return "Loop Detected";
	case 510: return "Not Extended";
	case 511: return "Network Authentication Required";
	}
	return "";
}

// ---------------------------------------------------------------------------
// Request targets (net/url ParseRequestURI) and queries (ParseQuery)

static int h_ishex(unsigned c) { return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F'); }

static int h_unhex(unsigned c) { return c <= '9' ? (int)(c - '0') : (int)((c | 0x20) - 'a' + 10); }

// host mode (url.shouldEscape for encodeHost): the ASCII bytes allowed
static int h_host_ok(unsigned c) {
	if ((c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
		return 1;
	}
	return strchr("!$&'()*+,;=:[]<>\"-_.~", (int)c) != NULL && c != 0;
}

enum { U_PATH, U_QUERY, U_HOST };

// h_unescape is url.unescape for the path, a query component, or a
// host: 0 on an invalid escape.
static int h_unescape(const char *s, int64_t n, int mode, nbuf *out) {
	for (int64_t i = 0; i < n;) {
		unsigned c = (unsigned char)s[i];
		if (c == '%') {
			if (i + 2 >= n || !h_ishex((unsigned char)s[i + 1]) || !h_ishex((unsigned char)s[i + 2])) {
				return 0;
			}
			if (mode == U_HOST && h_unhex((unsigned char)s[i + 1]) < 8 && !(s[i + 1] == '2' && s[i + 2] == '5')) {
				return 0;
			}
			nbuf_byte(out, (char)(h_unhex((unsigned char)s[i + 1]) << 4 | h_unhex((unsigned char)s[i + 2])));
			i += 3;
			continue;
		}
		if (c == '+' && mode == U_QUERY) {
			nbuf_byte(out, ' ');
		} else {
			if (mode == U_HOST && c < 0x80 && !h_host_ok(c)) {
				return 0;
			}
			nbuf_byte(out, (char)c);
		}
		i++;
	}
	return 1;
}

static int h_valid_port(const char *p, int64_t n) {
	if (n == 0) {
		return 1;
	}
	if (p[0] != ':') {
		return 0;
	}
	for (int64_t i = 1; i < n; i++) {
		if (p[i] < '0' || p[i] > '9') {
			return 0;
		}
	}
	return 1;
}

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

// h_parse_host is url.parseHost (without the IPv6 zone and address
// checks beyond inet_pton).
static int h_parse_host(const char *h, int64_t n, hstr *out) {
	int64_t ob = -1;
	for (int64_t i = n - 1; i >= 0; i--) {
		if (h[i] == '[') {
			ob = i;
			break;
		}
	}
	if (ob > 0) {
		return 0;
	}
	nbuf b = {0};
	if (ob == 0) {
		int64_t cb = -1;
		for (int64_t i = n - 1; i >= 0; i--) {
			if (h[i] == ']') {
				cb = i;
				break;
			}
		}
		if (cb < 0 || !h_valid_port(h + cb + 1, n - cb - 1)) {
			return 0;
		}
		nbuf hb = {0};
		if (!h_unescape(h + 1, cb - 1, U_HOST, &hb)) {
			free(hb.p);
			return 0;
		}
		nbuf_byte(&hb, 0);
		struct in6_addr a6;
		char *zone = strchr(hb.p, '%');
		if (zone) {
			*zone = 0;
		}
		int ok = inet_pton(AF_INET6, hb.p, &a6) == 1;
		free(hb.p);
		if (!ok) {
			return 0;
		}
		if (!h_unescape(h, n, U_HOST, &b)) {
			free(b.p);
			return 0;
		}
	} else {
		const char *colon = memchr(h, ':', (size_t)n);
		if (colon) {
			int64_t last = n - 1;
			while (last >= 0 && h[last] != ':') {
				last--;
			}
			int64_t i = colon - h;
			if (last != i) {
				// strict colons (Go 1.26): only one colon outside brackets
				return 0;
			}
			if (!h_valid_port(h + i, n - i)) {
				return 0;
			}
		}
		if (!h_unescape(h, n, U_HOST, &b)) {
			free(b.p);
			return 0;
		}
	}
	*out = hs_dup(b.p ? b.p : "", b.n);
	free(b.p);
	return 1;
}

// h_valid_userinfo is the userinfo check of url.parseAuthority (after
// the host parsed): validUserinfo's bytes, then unescape's check of the
// % escapes. 0 with Go's reason in err when it fails.
static int h_valid_userinfo(const char *s, int64_t n, char *err, size_t en) {
	for (int64_t i = 0; i < n; i++) {
		unsigned c = (unsigned char)s[i];
		if ((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || (c != 0 && strchr("-._:~!$&'()*+,;=%@", (int)c))) {
			continue;
		}
		snprintf(err, en, "net/url: invalid userinfo");
		return 0;
	}
	for (int64_t i = 0; i < n; i++) {
		if (s[i] != '%') {
			continue;
		}
		int64_t left = n - i;
		if (left < 3 || !h_ishex((unsigned char)s[i + 1]) || !h_ishex((unsigned char)s[i + 2])) {
			snprintf(err, en, "invalid URL escape \"%.*s\"", (int)(left < 3 ? left : 3), s + i);
			return 0;
		}
		i += 2;
	}
	return 1;
}

// h_url_parse is net/url's parse: ParseRequestURI when via_request,
// else Parse after the fragment is cut. 0 when it fails, with Go's
// reason in err (without the `parse "url": ` prefix).
static int h_url_parse(const char *s, int64_t sn, int via_request, hurl *u, char *err, size_t en) {
	memset(u, 0, sizeof *u);
	snprintf(err, en, "invalid URL");
	for (int64_t i = 0; i < sn; i++) {
		unsigned c = (unsigned char)s[i];
		if (c < ' ' || c == 0x7f) {
			snprintf(err, en, "net/url: invalid control character in URL");
			return 0;
		}
	}
	if (sn == 0 && via_request) {
		snprintf(err, en, "empty url");
		return 0;
	}
	if (sn == 1 && s[0] == '*') {
		u->path = hs_dup("*", 1);
		u->scheme = hs_dup("", 0);
		u->host = hs_dup("", 0);
		u->rawquery = hs_dup("", 0);
		return 1;
	}
	// getScheme
	int64_t rest = 0;
	hstr scheme = hs_dup("", 0);
	for (int64_t i = 0; i < sn; i++) {
		unsigned c = (unsigned char)s[i];
		if ((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			continue;
		}
		if ((c >= '0' && c <= '9') || c == '+' || c == '-' || c == '.') {
			if (i == 0) {
				break;
			}
			continue;
		}
		if (c == ':') {
			if (i == 0) {
				hs_free(&scheme);
				snprintf(err, en, "missing protocol scheme");
				return 0;
			}
			hs_free(&scheme);
			scheme = hs_dup(s, i);
			for (int64_t k = 0; k < i; k++) {
				scheme.p[k] = (char)h_lower(scheme.p[k]);
			}
			rest = i + 1;
		}
		break;
	}
	const char *r = s + rest;
	int64_t rn = sn - rest;
	// a single trailing "?" is ForceQuery with no RawQuery
	int64_t q = -1, qcount = 0;
	for (int64_t i = 0; i < rn; i++) {
		if (r[i] == '?') {
			if (q < 0) {
				q = i;
			}
			qcount++;
		}
	}
	if (rn > 0 && r[rn - 1] == '?' && qcount == 1) {
		u->rawquery = hs_dup("", 0);
		rn--;
	} else if (q >= 0) {
		u->rawquery = hs_dup(r + q + 1, rn - q - 1);
		u->has_query = 1;
		rn = q;
	} else {
		u->rawquery = hs_dup("", 0);
	}
	if (!(rn > 0 && r[0] == '/')) {
		if (scheme.n > 0) {
			// opaque: no host, no path
			u->scheme = scheme;
			u->host = hs_dup("", 0);
			u->path = hs_dup("", 0);
			return 1;
		}
		if (via_request) {
			hs_free(&scheme);
			hs_free(&u->rawquery);
			snprintf(err, en, "invalid URI for request");
			return 0;
		}
		const char *sl = memchr(r, '/', (size_t)rn);
		int64_t segn = sl ? sl - r : rn;
		if (memchr(r, ':', (size_t)segn)) {
			hs_free(&scheme);
			hs_free(&u->rawquery);
			snprintf(err, en, "first path segment in URL cannot contain colon");
			return 0;
		}
	}
	u->scheme = scheme;
	int authority = rn >= 2 && r[0] == '/' && r[1] == '/' && (scheme.n > 0 || (!via_request && !(rn >= 3 && r[2] == '/')));
	if (authority) {
		const char *auth = r + 2;
		int64_t an = rn - 2;
		const char *slash = memchr(auth, '/', (size_t)an);
		int64_t al = slash ? slash - auth : an;
		r = auth + al;
		rn = an - al;
		int64_t at = -1;
		for (int64_t i = al - 1; i >= 0; i--) {
			if (auth[i] == '@') {
				at = i;
				break;
			}
		}
		const char *hp = at >= 0 ? auth + at + 1 : auth;
		int64_t hn = at >= 0 ? al - at - 1 : al;
		if (!h_parse_host(hp, hn, &u->host)) {
			hu_free(u);
			char hq[160];
			snprintf(hq, sizeof hq, "%.*s", (int)(hn < 120 ? hn : 120), hp);
			snprintf(err, en, "invalid host \"%s\"", hq);
			return 0;
		}
		if (at >= 0 && !h_valid_userinfo(auth, at, err, en)) {
			hu_free(u);
			return 0;
		}
	} else {
		u->host = hs_dup("", 0);
	}
	nbuf pb = {0};
	if (!h_unescape(r, rn, U_PATH, &pb)) {
		free(pb.p);
		// the bad escape: %, and up to two bytes after it
		const char *pc = memchr(r, '%', (size_t)rn);
		for (; pc; pc = memchr(pc + 1, '%', (size_t)(rn - (pc + 1 - r)))) {
			int64_t left = rn - (pc - r);
			if (left < 3 || !h_ishex((unsigned char)pc[1]) || !h_ishex((unsigned char)pc[2])) {
				snprintf(err, en, "invalid URL escape \"%.*s\"", (int)(left < 3 ? left : 3), pc);
				break;
			}
		}
		hu_free(u);
		return 0;
	}
	u->path = hs_dup(pb.p ? pb.p : "", pb.n);
	free(pb.p);
	return 1;
}

// h_parse_target is url.ParseRequestURI of a request target: 0 when it
// fails (400). A CONNECT target that is not a path is an authority.
static int h_parse_target(const char *raw, int64_t n, int connect, hurl *u) {
	char err[160];
	if (connect && !(n > 0 && raw[0] == '/')) {
		nbuf t = {0};
		nbuf_put(&t, "http://", 7);
		nbuf_put(&t, raw, n);
		int ok = h_url_parse(t.p, t.n, 1, u, err, sizeof err);
		free(t.p);
		if (ok) {
			hs_free(&u->scheme);
			u->scheme = hs_dup("", 0);
		}
		return ok;
	}
	return h_url_parse(raw, n, 1, u, err, sizeof err);
}

static int hp_cmp(const void *a, const void *b) {
	const hpair *x = a, *y = b;
	int64_t n = x->k.n < y->k.n ? x->k.n : y->k.n;
	int c = memcmp(x->k.p, y->k.p, (size_t)n);
	if (c) {
		return c;
	}
	return x->k.n < y->k.n ? -1 : x->k.n > y->k.n;
}

// h_parse_query is std/rt's urlOf over url.ParseQuery: each key once
// with its first value, keys sorted.
static void h_parse_query(const char *q, int64_t n, hlist *out) {
	int64_t amps = 0;
	for (int64_t i = 0; i < n; i++) {
		amps += q[i] == '&';
	}
	if (amps + 1 > 10000) {
		return; // number of URL query parameters exceeded limit
	}
	int64_t i = 0;
	while (i < n) {
		int64_t j = i;
		while (j < n && q[j] != '&') {
			j++;
		}
		const char *kv = q + i;
		int64_t kn = j - i;
		i = j + 1;
		if (memchr(kv, ';', (size_t)kn) || kn == 0) {
			continue;
		}
		const char *eq = memchr(kv, '=', (size_t)kn);
		int64_t kl = eq ? eq - kv : kn;
		nbuf kb = {0}, vb = {0};
		if (!h_unescape(kv, kl, U_QUERY, &kb) || (eq && !h_unescape(eq + 1, kn - kl - 1, U_QUERY, &vb))) {
			free(kb.p);
			free(vb.p);
			continue;
		}
		hstr k = hs_dup(kb.p ? kb.p : "", kb.n), v = hs_dup(vb.p ? vb.p : "", vb.n);
		free(kb.p);
		free(vb.p);
		if (hl_find(out, k.p) >= 0 && memchr(k.p, 0, (size_t)k.n) == NULL) {
			hs_free(&k);
			hs_free(&v);
			continue;
		}
		// a key with a NUL byte cannot be found with hl_find: compare all
		int dup = 0;
		for (int64_t e = 0; e < out->n; e++) {
			if (out->a[e].k.n == k.n && memcmp(out->a[e].k.p, k.p, (size_t)k.n) == 0) {
				dup = 1;
				break;
			}
		}
		if (dup) {
			hs_free(&k);
			hs_free(&v);
			continue;
		}
		hl_add(out, k, v);
	}
	if (out->n > 1) {
		qsort(out->a, (size_t)out->n, sizeof(hpair), hp_cmp);
	}
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

// a raw header field before merging
typedef struct {
	char *k, *v;
	int64_t kn, vn;
} hraw;

// h_line finds the line at b[i..n): its end (without \n, and without the
// \r before it) in *le and the start of the next in *next. 0 when there
// is no \n yet (at eof: the rest is the line, bufio's ReadLine).
static int h_line(const char *b, int64_t i, int64_t n, int eof, int64_t *le, int64_t *next) {
	const char *nl = memchr(b + i, '\n', (size_t)(n - i));
	if (!nl) {
		if (eof && i < n) {
			*le = n;
			*next = n;
			return 1;
		}
		return 0;
	}
	int64_t e = nl - b;
	*next = e + 1;
	if (e > i && b[e - 1] == '\r') {
		e--;
	}
	*le = e;
	return 1;
}

// hp_head reads the request line and the header of the request at the
// start of b (n bytes; eof: no more will come), as net/http's
// readRequest and the checks of conn.readRequest. *used is the length
// of the head on HP_OK.
static int hp_head(const char *b, int64_t n, int eof, hreq *r, int64_t *used) {
	memset(r, 0, sizeof *r);
	if (n == 0) {
		return eof ? HP_EOF : HP_NEED;
	}
	int64_t le, next;
	if (!h_line(b, 0, n, eof, &le, &next)) {
		return n > HTTP_MAX_HEAD ? HP_TOO_LARGE : HP_NEED;
	}
	if (eof && next == n && le == n && n % 4096 == 0) {
		// a request line cut by the end of the stream that fills
		// net/http's 4096 byte reader exactly: its last read finds no
		// byte, so the line reads as io.EOF and the connection closes
		// quietly
		return HP_EOF;
	}
	if (next > HTTP_MAX_HEAD) {
		return HP_TOO_LARGE;
	}
	// METHOD SP target SP proto (strings.Cut on single spaces)
	const char *line = b;
	const char *sp1 = memchr(line, ' ', (size_t)le);
	if (!sp1) {
		return HP_BAD;
	}
	const char *rest = sp1 + 1;
	int64_t restn = le - (rest - line);
	const char *sp2 = memchr(rest, ' ', (size_t)restn);
	if (!sp2) {
		return HP_BAD;
	}
	int64_t mn = sp1 - line;
	if (mn == 0) {
		return HP_BAD;
	}
	for (int64_t i = 0; i < mn; i++) {
		if (!h_tchar((unsigned char)line[i])) {
			return HP_BAD; // invalid method
		}
	}
	const char *proto = sp2 + 1;
	int64_t pn = le - (proto - line);
	int major, minor;
	if (pn == 8 && memcmp(proto, "HTTP/1.1", 8) == 0) {
		major = 1;
		minor = 1;
	} else if (pn == 8 && memcmp(proto, "HTTP/1.0", 8) == 0) {
		major = 1;
		minor = 0;
	} else if (pn == 8 && memcmp(proto, "HTTP/", 5) == 0 && proto[6] == '.' && proto[5] >= '0' && proto[5] <= '9' &&
	           proto[7] >= '0' && proto[7] <= '9') {
		major = proto[5] - '0';
		minor = proto[7] - '0';
	} else {
		return HP_BAD; // malformed HTTP version
	}
	r->method = hs_dup(line, mn);
	r->target = hs_dup(rest, sp2 - rest);
	r->major = major;
	r->minor = minor;
	int connect = hs_eq(r->method, "CONNECT");
	if (!h_parse_target(r->target.p, r->target.n, connect, &r->url)) {
		hr_free(r);
		return HP_BAD;
	}
	if (r->url.rawquery.n > 0) {
		h_parse_query(r->url.rawquery.p, r->url.rawquery.n, &r->query);
	}
	// the header: lines up to an empty one
	int64_t i = next;
	hraw *raw = NULL;
	int64_t nraw = 0, rcap = 0;
	int res = HP_NEED;
	nbuf cont = {0};
	if (i < n && (b[i] == ' ' || b[i] == '\t')) {
		// malformed MIME header initial line (needs the line to say so)
		if (!h_line(b, i, n, eof, &le, &next)) {
			res = eof ? HP_BAD : HP_NEED;
		} else {
			res = HP_BAD;
		}
		goto out;
	}
	for (;;) {
		if (i - 0 > HTTP_MAX_HEAD) {
			res = HP_TOO_LARGE;
			goto out;
		}
		if (!h_line(b, i, n, eof, &le, &next)) {
			res = eof ? HP_BAD : (n > HTTP_MAX_HEAD ? HP_TOO_LARGE : HP_NEED);
			goto out;
		}
		if (le == i) {
			// the empty line ends the header
			i = next;
			break;
		}
		// a field line: it must have a colon
		if (!memchr(b + i, ':', (size_t)(le - i))) {
			res = HP_BAD;
			goto out;
		}
		// trim (spaces and tabs) and join continuation lines
		cont.n = 0;
		int64_t a = i, z = le;
		while (a < z && (b[a] == ' ' || b[a] == '\t')) {
			a++;
		}
		while (z > a && (b[z - 1] == ' ' || b[z - 1] == '\t')) {
			z--;
		}
		nbuf_put(&cont, b + a, z - a);
		i = next;
		for (;;) {
			if (i >= n) {
				if (!eof) {
					res = n > HTTP_MAX_HEAD ? HP_TOO_LARGE : HP_NEED;
					goto out;
				}
				break;
			}
			if (b[i] != ' ' && b[i] != '\t') {
				break;
			}
			// a continuation line: skip its leading spaces and tabs
			int64_t k = i;
			while (k < n && (b[k] == ' ' || b[k] == '\t')) {
				k++;
			}
			if (k >= n && !eof) {
				res = n > HTTP_MAX_HEAD ? HP_TOO_LARGE : HP_NEED;
				goto out;
			}
			nbuf_byte(&cont, ' ');
			if (!h_line(b, k, n, eof, &le, &next)) {
				if (!eof) {
					res = n > HTTP_MAX_HEAD ? HP_TOO_LARGE : HP_NEED;
					goto out;
				}
				// at the end: nothing more (readLineSlice failed)
				i = n;
				break;
			}
			a = k;
			z = le;
			while (a < z && (b[a] == ' ' || b[a] == '\t')) {
				a++;
			}
			while (z > a && (b[z - 1] == ' ' || b[z - 1] == '\t')) {
				z--;
			}
			nbuf_put(&cont, b + a, z - a);
			i = next;
		}
		// key: up to the first colon; token bytes, or spaces (kept as is)
		const char *kv = cont.p;
		const char *colon = memchr(kv, ':', (size_t)cont.n);
		int64_t kn = colon - kv;
		if (kn == 0) {
			res = HP_BAD;
			goto out;
		}
		for (int64_t k = 0; k < kn; k++) {
			unsigned c = (unsigned char)kv[k];
			if (!h_tchar(c) && c != ' ') {
				res = HP_BAD;
				goto out;
			}
		}
		const char *v = colon + 1;
		int64_t vn = cont.n - kn - 1;
		for (int64_t k = 0; k < vn; k++) {
			if (!h_vbyte((unsigned char)v[k])) {
				res = HP_BAD;
				goto out;
			}
		}
		while (vn > 0 && (*v == ' ' || *v == '\t')) {
			v++;
			vn--;
		}
		if (nraw == HTTP_MAX_FIELDS) {
			res = HP_TOO_LARGE;
			goto out;
		}
		if (nraw == rcap) {
			rcap = rcap ? rcap * 2 : 16;
			raw = realloc(raw, (size_t)rcap * sizeof(hraw));
			if (!raw) {
				xo_oom();
			}
		}
		hraw *f = &raw[nraw++];
		f->k = malloc((size_t)kn + 1);
		f->v = malloc((size_t)vn + 1);
		if (!f->k || !f->v) {
			xo_oom();
		}
		memcpy(f->k, kv, (size_t)kn);
		f->k[kn] = 0;
		memcpy(f->v, v, (size_t)vn);
		f->v[vn] = 0;
		f->kn = kn;
		f->vn = vn;
		h_canon(f->k, kn);
	}
	if (i > HTTP_MAX_HEAD) {
		res = HP_TOO_LARGE;
		goto out;
	}
	*used = i;
	// Go's Header: canonical key -> values in order
	{
		// counts per key, for Host and Content-Length checks
		int64_t hosts = 0, tes = 0, cls = 0;
		const char *first_cl = NULL;
		int64_t first_cln = 0;
		int cl_conflict = 0;
		for (int64_t k = 0; k < nraw; k++) {
			if (strcmp(raw[k].k, "Host") == 0) {
				hosts++;
			} else if (strcmp(raw[k].k, "Transfer-Encoding") == 0) {
				tes++;
			} else if (strcmp(raw[k].k, "Content-Length") == 0) {
				// textproto.TrimString: ASCII space, tab, CR, LF
				const char *p = raw[k].v;
				int64_t pl = raw[k].vn;
				while (pl > 0 && strchr(" \t\r\n", p[0]) && p[0]) {
					p++;
					pl--;
				}
				while (pl > 0 && strchr(" \t\r\n", p[pl - 1]) && p[pl - 1]) {
					pl--;
				}
				if (cls == 0) {
					first_cl = p;
					first_cln = pl;
				} else if (pl != first_cln || memcmp(p, first_cl, (size_t)pl) != 0) {
					cl_conflict = 1;
				}
				cls++;
			}
		}
		if (hosts > 1) {
			res = HP_BAD; // too many Host headers
			goto out;
		}
		// readTransfer: Transfer-Encoding (removed from the header)
		int chunked = 0;
		// readTransfer takes HTTP/0.0 for HTTP/1.1
		int tmaj = major, tmin = minor;
		if (tmaj == 0 && tmin == 0) {
			tmaj = tmin = 1;
		}
		if (tes > 0 && (tmaj > 1 || (tmaj == 1 && tmin >= 1))) {
			if (tes != 1) {
				res = HP_BAD_TE;
				goto out;
			}
			for (int64_t k = 0; k < nraw; k++) {
				if (strcmp(raw[k].k, "Transfer-Encoding") == 0) {
					const char *te = raw[k].v;
					if (raw[k].vn != 7 || strncasecmp(te, "chunked", 7) != 0) {
						res = HP_BAD_TE;
						goto out;
					}
				}
			}
			chunked = 1;
		}
		// Content-Length (several must agree)
		int64_t clen = 0;
		if (cl_conflict) {
			res = HP_BAD;
			goto out;
		}
		if (cls > 0) {
			if (first_cln == 0) {
				res = HP_BAD; // invalid empty Content-Length
				goto out;
			}
			uint64_t v = 0;
			for (int64_t k = 0; k < first_cln; k++) {
				if (first_cl[k] < '0' || first_cl[k] > '9' || v > (((uint64_t)1 << 63) - 1 - (uint64_t)(first_cl[k] - '0')) / 10) {
					res = HP_BAD; // bad Content-Length
					goto out;
				}
				v = v * 10 + (uint64_t)(first_cl[k] - '0');
			}
			clen = (int64_t)v;
		}
		if (chunked) {
			clen = -1;
		}
		r->chunked = chunked;
		r->clen = clen;
		// the merged header, without the fields Go removes
		int64_t pragma = -1;
		int have_cc = 0;
		for (int64_t k = 0; k < nraw; k++) {
			const char *key = raw[k].k;
			if (strcmp(key, "Host") == 0 || strcmp(key, "Transfer-Encoding") == 0) {
				continue;
			}
			if (chunked && (strcmp(key, "Content-Length") == 0 || strcmp(key, "Trailer") == 0)) {
				continue;
			}
			if (strcmp(key, "Content-Length") == 0) {
				// deduplicated; its value as written first
				if (hl_find(&r->hdr, key) >= 0) {
					continue;
				}
			}
			int64_t e = -1;
			for (int64_t x = 0; x < r->hdr.n; x++) {
				if (r->hdr.a[x].k.n == raw[k].kn && memcmp(r->hdr.a[x].k.p, key, (size_t)raw[k].kn) == 0) {
					e = x;
					break;
				}
			}
			if (e >= 0) {
				hstr *ov = &r->hdr.a[e].v;
				char *nv = xo_alloc(ov->n + 2 + raw[k].vn + 1);
				memcpy(nv, ov->p, (size_t)ov->n);
				memcpy(nv + ov->n, ", ", 2);
				memcpy(nv + ov->n + 2, raw[k].v, (size_t)raw[k].vn);
				nv[ov->n + 2 + raw[k].vn] = 0;
				xo_free(ov->p);
				ov->p = nv;
				ov->n += 2 + raw[k].vn;
			} else {
				hl_add(&r->hdr, hs_dup(key, raw[k].kn), hs_dup(raw[k].v, raw[k].vn));
			}
			if (strcmp(key, "Pragma") == 0 && pragma < 0) {
				pragma = k;
			}
			if (strcmp(key, "Cache-Control") == 0) {
				have_cc = 1;
			}
		}
		// fixPragmaCacheControl: Pragma: no-cache without Cache-Control
		if (pragma >= 0 && !have_cc && raw[pragma].vn == 8 && memcmp(raw[pragma].v, "no-cache", 8) == 0) {
			hl_add(&r->hdr, hs_dup("Cache-Control", 13), hs_dup("no-cache", 8));
		}
		// req.Host: the URL's host, else the Host header
		const char *host = NULL;
		int64_t hostn = 0;
		for (int64_t k = 0; k < nraw; k++) {
			if (strcmp(raw[k].k, "Host") == 0) {
				host = raw[k].v;
				hostn = raw[k].vn;
			}
		}
		if (r->url.host.n > 0) {
			r->host = hs_dup(r->url.host.p, r->url.host.n);
		} else {
			r->host = hs_dup(host ? host : "", hostn);
		}
		// shouldClose (Connection tokens of every Connection field)
		int has_close = 0, has_ka = 0;
		for (int64_t k = 0; k < nraw; k++) {
			if (strcmp(raw[k].k, "Connection") == 0) {
				has_close |= h_has_token(raw[k].v, raw[k].vn, "close");
				has_ka |= h_has_token(raw[k].v, raw[k].vn, "keep-alive");
			}
		}
		if (major < 1) {
			r->close = 1;
		} else if (major == 1 && minor == 0) {
			r->close = has_close || !has_ka;
		} else {
			r->close = has_close;
		}
		// conn.readRequest's checks
		if (!(major == 1 || (major == 2 && minor == 0 && hs_eq(r->method, "PRI") && hs_eq(r->target, "*")))) {
			r->status = 505;
			r->reason = "unsupported protocol version";
			res = HP_STATUS;
			goto out;
		}
		int h2up = hs_eq(r->method, "PRI") && nraw == 0 && hs_eq(r->url.path, "*") && major == 2 && minor == 0;
		if ((major > 1 || (major == 1 && minor >= 1)) && hosts == 0 && !h2up && !connect) {
			r->status = 400;
			r->reason = "missing required Host header";
			res = HP_STATUS;
			goto out;
		}
		if (hosts == 1) {
			for (int64_t k = 0; k < hostn; k++) {
				unsigned c = (unsigned char)host[k];
				if (!((c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || strchr("!$%&()*+,-.:;=['\\]_~", (int)c)) || c == 0 ||
				    c == '\\') {
					r->status = 400;
					r->reason = "malformed Host header";
					res = HP_STATUS;
					goto out;
				}
			}
		}
		for (int64_t k = 0; k < nraw; k++) {
			for (int64_t x = 0; x < raw[k].kn; x++) {
				if (!h_tchar((unsigned char)raw[k].k[x])) {
					r->status = 400;
					r->reason = "invalid header name";
					res = HP_STATUS;
					goto out;
				}
			}
		}
		// Connection close / keep-alive wishes (Request.wantsClose etc.)
		int64_t ci = hl_find(&r->hdr, "Connection");
		const char *cv = ci >= 0 ? r->hdr.a[ci].v.p : "";
		int64_t cvn = ci >= 0 ? r->hdr.a[ci].v.n : 0;
		// Header.get is the first value only
		for (int64_t k = 0; k < nraw; k++) {
			if (strcmp(raw[k].k, "Connection") == 0) {
				cv = raw[k].v;
				cvn = raw[k].vn;
				break;
			}
		}
		r->wants10ka = major == 1 && minor == 0 && h_server_token(cv, cvn, "keep-alive");
		r->wantsclose = r->close || h_server_token(cv, cvn, "close");
		if (h2up) {
			r->h2up = 1;
			r->clen = 0; // its Body is NoBody (std/rt reads none)
			r->close = 1;
			r->wantsclose = 1;
		}
		const char *ev = NULL;
		int64_t evn = 0;
		for (int64_t k = 0; k < nraw; k++) {
			if (strcmp(raw[k].k, "Expect") == 0) {
				ev = raw[k].v;
				evn = raw[k].vn;
				break;
			}
		}
		if (ev && h_server_token(ev, evn, "100-continue")) {
			r->expect100 = (major > 1 || (major == 1 && minor >= 1)) && r->clen != 0;
		} else if (ev && evn > 0) {
			r->expect_other = 1;
		}
		res = HP_OK;
	}
out:
	for (int64_t k = 0; k < nraw; k++) {
		free(raw[k].k);
		free(raw[k].v);
	}
	free(raw);
	free(cont.p);
	if (res != HP_OK && res != HP_STATUS) {
		hr_free(r);
	}
	return res;
}

enum { HB_NEED, HB_OK, HB_ERR };

// hb_chunked decodes a chunked body at the start of b (n bytes; eof: no
// more will come) into out, as net/http's chunkedReader and the trailer
// reading of body.readTrailer: HB_OK and its length in *used, HB_NEED, or
// HB_ERR (out holds what was decoded before the error).
static int hb_chunked(const char *b, int64_t n, int eof, nbuf *out, int64_t *used) {
	int64_t i = 0, excess = 0;
	out->n = 0;
	for (;;) {
		const char *nl = memchr(b + i, '\n', (size_t)(n - i));
		if (!nl) {
			if (n - i >= 4096) {
				return HB_ERR; // header line too long
			}
			return eof ? HB_ERR : HB_NEED;
		}
		int64_t e = nl - b; // the \n
		// CRLF required, no other CR
		const char *cr = memchr(b + i, '\r', (size_t)(e + 1 - i));
		if (!cr || cr - b != e - 1) {
			return HB_ERR;
		}
		int64_t ll = e - 1 - i; // the line without CRLF
		if (ll >= 4096) {
			return HB_ERR;
		}
		excess += ll + 2;
		int64_t a = i, z = i + ll;
		while (z > a && (b[z - 1] == ' ' || b[z - 1] == '\t')) {
			z--;
		}
		const char *semi = memchr(b + a, ';', (size_t)(z - a));
		if (semi) {
			z = semi - b;
		}
		if (z == a) {
			return HB_ERR; // empty hex number
		}
		uint64_t size = 0;
		for (int64_t k = a; k < z; k++) {
			unsigned c = (unsigned char)b[k];
			if (!h_ishex(c)) {
				return HB_ERR;
			}
			if (k - a == 16) {
				return HB_ERR; // too large
			}
			size = size << 4 | (uint64_t)h_unhex(c);
		}
		excess -= 16 + 2 * (int64_t)(size > (uint64_t)INT64_MAX / 4 ? INT64_MAX / 4 : size);
		if (excess < 0) {
			excess = 0;
		}
		if (excess > 16 * 1024) {
			return HB_ERR; // too much non-data
		}
		i = e + 1;
		if (size == 0) {
			// the trailer: CRLF, or a MIME header up to an empty line
			if (n - i < 2) {
				return eof ? HB_ERR : HB_NEED;
			}
			if (b[i] == '\r' && b[i + 1] == '\n') {
				*used = i + 2;
				return HB_OK;
			}
			// up to a double CRLF within 4096 bytes
			int64_t lim = n - i < 4096 ? n - i : 4096;
			int64_t end = -1;
			for (int64_t k = 3; k < lim; k++) {
				if (b[i + k - 3] == '\r' && b[i + k - 2] == '\n' && b[i + k - 1] == '\r' && b[i + k] == '\n') {
					end = i + k + 1;
					break;
				}
			}
			if (end < 0) {
				if (n - i >= 4096 || eof) {
					return HB_ERR;
				}
				return HB_NEED;
			}
			// the trailer is a MIME header up to its first empty line (a
			// bare LF ends it too)
			hreq tr;
			int64_t tu = 0;
			nbuf fake = {0};
			nbuf_str(&fake, "GET / HTTP/1.0\r\n");
			nbuf_put(&fake, b + i, n - i);
			int t = hp_head(fake.p, fake.n, eof, &tr, &tu);
			free(fake.p);
			if (t == HP_OK || t == HP_STATUS) {
				hr_free(&tr);
			}
			if (t == HP_NEED) {
				return HB_NEED;
			}
			if (t != HP_OK && t != HP_STATUS) {
				return HB_ERR;
			}
			*used = i + tu - 16;
			return HB_OK;
		}
		if (size > (uint64_t)(n - i) || (uint64_t)(n - i) - size < 2) {
			// copy what is there: an error later still delivers it
			// (a size of 2^63 or more is not negative here)
			int64_t have = (uint64_t)(n - i) < size ? n - i : (int64_t)size;
			nbuf_put(out, b + i, have);
			return eof ? HB_ERR : HB_NEED;
		}
		nbuf_put(out, b + i, (int64_t)size);
		i += (int64_t)size;
		if (b[i] != '\r' || b[i + 1] != '\n') {
			return HB_ERR; // malformed chunked encoding
		}
		i += 2;
	}
}

// ---------------------------------------------------------------------------
// Writing a response

// h_sniff is net/http's DetectContentType.
static const char *h_sniff(const unsigned char *d, int64_t n) {
	if (n > 512) {
		n = 512;
	}
	int64_t ws = 0;
	while (ws < n && (d[ws] == '\t' || d[ws] == '\n' || d[ws] == '\x0c' || d[ws] == '\r' || d[ws] == ' ')) {
		ws++;
	}
	static const char *html[] = {"<!DOCTYPE HTML", "<HTML", "<HEAD", "<SCRIPT", "<IFRAME", "<H1", "<DIV", "<FONT", "<TABLE", "<A",
	                             "<STYLE", "<TITLE", "<B", "<BODY", "<BR", "<P", "<!--"};
	for (size_t k = 0; k < sizeof html / sizeof html[0]; k++) {
		const char *h = html[k];
		int64_t hl = (int64_t)strlen(h);
		if (n - ws < hl + 1) {
			continue;
		}
		int ok = 1;
		for (int64_t j = 0; j < hl; j++) {
			unsigned db = d[ws + j];
			if (h[j] >= 'A' && h[j] <= 'Z') {
				db &= 0xDF;
			}
			if ((unsigned char)h[j] != db) {
				ok = 0;
				break;
			}
		}
		if (ok && (d[ws + hl] == ' ' || d[ws + hl] == '>')) {
			return "text/html; charset=utf-8";
		}
	}
	if (n - ws >= 5 && memcmp(d + ws, "<?xml", 5) == 0) {
		return "text/xml; charset=utf-8";
	}
#define PREFIX(s, ct)                                                          \
	if (n >= (int64_t)(sizeof(s) - 1) && memcmp(d, s, sizeof(s) - 1) == 0) { \
		return ct;                                                             \
	}
	PREFIX("%PDF-", "application/pdf");
	PREFIX("%!PS-Adobe-", "application/postscript");
	if (n >= 4 && d[0] == 0xFE && d[1] == 0xFF) {
		return "text/plain; charset=utf-16be";
	}
	if (n >= 4 && d[0] == 0xFF && d[1] == 0xFE) {
		return "text/plain; charset=utf-16le";
	}
	if (n >= 4 && d[0] == 0xEF && d[1] == 0xBB && d[2] == 0xBF) {
		return "text/plain; charset=utf-8";
	}
	PREFIX("\x00\x00\x01\x00", "image/x-icon");
	PREFIX("\x00\x00\x02\x00", "image/x-icon");
	PREFIX("BM", "image/bmp");
	PREFIX("GIF87a", "image/gif");
	PREFIX("GIF89a", "image/gif");
	if (n >= 14 && memcmp(d, "RIFF", 4) == 0 && memcmp(d + 8, "WEBPVP", 6) == 0) {
		return "image/webp";
	}
	PREFIX("\x89PNG\x0D\x0A\x1A\x0A", "image/png");
	PREFIX("\xFF\xD8\xFF", "image/jpeg");
	if (n >= 12 && memcmp(d, "FORM", 4) == 0 && memcmp(d + 8, "AIFF", 4) == 0) {
		return "audio/aiff";
	}
	PREFIX("ID3", "audio/mpeg");
	PREFIX("OggS\x00", "application/ogg");
	PREFIX("MThd\x00\x00\x00\x06", "audio/midi");
	if (n >= 12 && memcmp(d, "RIFF", 4) == 0 && memcmp(d + 8, "AVI ", 4) == 0) {
		return "video/avi";
	}
	if (n >= 12 && memcmp(d, "RIFF", 4) == 0 && memcmp(d + 8, "WAVE", 4) == 0) {
		return "audio/wave";
	}
	if (n >= 12) {
		int64_t box = (int64_t)d[0] << 24 | (int64_t)d[1] << 16 | (int64_t)d[2] << 8 | d[3];
		if (n >= box && box % 4 == 0 && memcmp(d + 4, "ftyp", 4) == 0) {
			for (int64_t st = 8; st < box; st += 4) {
				if (st == 12) {
					continue;
				}
				if (memcmp(d + st, "mp4", 3) == 0) {
					return "video/mp4";
				}
			}
		}
	}
	PREFIX("\x1A\x45\xDF\xA3", "video/webm");
	if (n >= 36) {
		int z = 1;
		for (int k = 0; k < 34; k++) {
			if (d[k] != 0) {
				z = 0;
				break;
			}
		}
		if (z && d[34] == 'L' && d[35] == 'P') {
			return "application/vnd.ms-fontobject";
		}
	}
	PREFIX("\x00\x01\x00\x00", "font/ttf");
	PREFIX("OTTO", "font/otf");
	PREFIX("ttcf", "font/collection");
	PREFIX("wOFF", "font/woff");
	PREFIX("wOF2", "font/woff2");
	PREFIX("\x1F\x8B\x08", "application/x-gzip");
	PREFIX("PK\x03\x04", "application/zip");
	PREFIX("Rar!\x1A\x07\x00", "application/x-rar-compressed");
	PREFIX("Rar!\x1A\x07\x01\x00", "application/x-rar-compressed");
	PREFIX("\x00\x61\x73\x6D", "application/wasm");
#undef PREFIX
	for (int64_t k = ws; k < n; k++) {
		unsigned c = d[k];
		if (c <= 0x08 || c == 0x0B || (c >= 0x0E && c <= 0x1A) || (c >= 0x1C && c <= 0x1F)) {
			return "application/octet-stream";
		}
	}
	return "text/plain; charset=utf-8";
}

// h_itoa writes v in decimal at out (room for 21 bytes and a NUL) and
// returns its length.
static int h_itoa(char *out, int64_t v) {
	char t[24];
	int n = 0;
	uint64_t u = v < 0 ? -(uint64_t)v : (uint64_t)v;
	do {
		t[n++] = (char)('0' + u % 10);
		u /= 10;
	} while (u);
	int k = 0;
	if (v < 0) {
		out[k++] = '-';
	}
	while (n) {
		out[k++] = t[--n];
	}
	out[k] = 0;
	return k;
}

// h_date writes the Date header value of now (http.TimeFormat), made at
// most once a second per thread (each response used to format it with
// gmtime_r and snprintf).
static void h_date(char *out) {
	static const char *days[] = {"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"};
	static const char *mons[] = {"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"};
	static __thread time_t last = -1;
	static __thread char text[40];
	time_t t = time(NULL);
	if (t != last) {
		struct tm tm;
		gmtime_r(&t, &tm);
		snprintf(text, sizeof text, "%s, %02d %s %04d %02d:%02d:%02d GMT", days[tm.tm_wday], tm.tm_mday, mons[tm.tm_mon], tm.tm_year + 1900,
		         tm.tm_hour, tm.tm_min, tm.tm_sec);
		last = t;
	}
	memcpy(out, text, sizeof text);
}

static int h_body_allowed(int64_t st) { return !((st >= 100 && st <= 199) || st == 204 || st == 304); }

// hw_header_block writes the handler's header fields sorted by name, as
// Header.writeSubset: invalid names are left out, newlines in values
// become spaces, values are trimmed.
static void hw_fields(nbuf *o, hlist *h, const char **excl, int nexcl) {
	hpair *s = malloc((size_t)(h->n ? h->n : 1) * sizeof(hpair));
	if (!s) {
		xo_oom();
	}
	if (h->n) {
		memcpy(s, h->a, (size_t)h->n * sizeof(hpair));
	}
	qsort(s, (size_t)h->n, sizeof(hpair), hp_cmp);
	for (int64_t i = 0; i < h->n; i++) {
		int skip = s[i].k.n == 0;
		for (int e = 0; e < nexcl && !skip; e++) {
			skip = hs_eq(s[i].k, excl[e]);
		}
		for (int64_t k = 0; k < s[i].k.n && !skip; k++) {
			skip = !h_tchar((unsigned char)s[i].k.p[k]);
		}
		if (skip) {
			continue;
		}
		nbuf_put(o, s[i].k.p, s[i].k.n);
		nbuf_put(o, ": ", 2);
		const char *v = s[i].v.p;
		int64_t vn = s[i].v.n;
		// textproto.TrimString after the newline replacement
		int64_t a = 0, z = vn;
		while (a < z && strchr(" \t\r\n", v[a]) && v[a]) {
			a++;
		}
		while (z > a && strchr(" \t\r\n", v[z - 1]) && v[z - 1]) {
			z--;
		}
		for (int64_t k = a; k < z; k++) {
			nbuf_byte(o, v[k] == '\n' || v[k] == '\r' ? ' ' : v[k]);
		}
		nbuf_put(o, "\r\n", 2);
	}
	free(s);
}

static void hw_status_line(nbuf *o, int is11, int64_t code) {
	char line[96];
	const char *t = h_status_text(code);
	if (t[0] && code >= 100 && code <= 999) {
		// "HTTP/1.x NNN text\r\n" (snprintf per response showed in profiles)
		int k = 0;
		memcpy(line, is11 ? "HTTP/1.1 " : "HTTP/1.0 ", 9);
		k = 9;
		k += h_itoa(line + k, code);
		line[k++] = ' ';
		size_t tn = strlen(t);
		if (tn > sizeof line - (size_t)k - 3) {
			tn = sizeof line - (size_t)k - 3;
		}
		memcpy(line + k, t, tn);
		k += (int)tn;
		line[k++] = '\r';
		line[k++] = '\n';
		line[k] = 0;
	} else if (t[0]) {
		snprintf(line, sizeof line, "HTTP/1.%d %lld %s\r\n", is11, (long long)code, t);
	} else {
		snprintf(line, sizeof line, "HTTP/1.%d %03lld status code %lld\r\n", is11, (long long)code, (long long)code);
	}
	nbuf_str(o, line);
}

// hw_response writes the response to request r (handler header h,
// status, body of bn bytes) as net/http does when a handler sets the
// header and writes the whole body at once (std/rt's serve), and reports
// whether the connection stays open. expect_unread: the request expected
// 100-continue and its body was not read to the end (net/http then
// closes). body_bad: reading the request body failed (the chunked
// reader's error is sticky, so discarding the rest fails too); too_big:
// 256 KiB or more of a Content-Length body were never sent (net/http's
// requestTooLarge).
static int hw_response_x(nbuf *o, hreq *r, hlist *h, int64_t status, const char *body, int64_t bn, int expect_unread, int body_bad, int too_big, int stream, int *chunked) {
	int is11 = r->major > 1 || (r->major == 1 && r->minor >= 1);
	int head = hs_eq(r->method, "HEAD");
	if (status < 100 || status > 999) {
		// net/http panics in WriteHeader: the connection closes unanswered
		return 0;
	}
	if (status >= 100 && status <= 199 && status != 101) {
		// an informational response, then 200 with the same header
		hw_status_line(o, is11, status);
		static const char *nb[] = {"Content-Length", "Transfer-Encoding"};
		hw_fields(o, h, nb, 2);
		nbuf_put(o, "\r\n", 2);
		status = 200;
	}
	// response.WriteHeader: the handler's Content-Length, when valid
	int64_t contentLength = -1;
	int64_t cli = hl_find(h, "Content-Length");
	if (cli >= 0) {
		// an invalid one is deleted from the handler's header, but the
		// copy being written still has it (the copy was made first)
		int64_t v;
		if (n_parse_int(h->a[cli].v.p, h->a[cli].v.n, &v) && v >= 0) {
			contentLength = v;
		}
	}
	int body_ok = h_body_allowed(status);
	// response.write: the bytes counted, and those reaching the buffer
	int64_t counted = body_ok ? bn : 0;
	int64_t written = counted;
	if (contentLength != -1 && counted > contentLength) {
		written = 0; // ErrContentLength
	}
	// chunkWriter.writeHeader(p): the whole body when it fits the 2048
	// byte buffer (the handler is done by then), else its first 2048. A
	// stream's head goes out with its first write while the handler is
	// not done (p: that write)
	int64_t plen = stream || written <= 2048 ? written : 2048;
	int done = !stream && written <= 2048;
	const char *excl[8];
	int nexcl = 0;
	const char *set_ct = NULL, *set_conn = NULL, *set_te = NULL;
	char set_cl[24] = {0}, date[40] = {0};
	int close = r->h2up;
	int64_t tei = hl_find(h, "Transfer-Encoding");
	const char *te = tei >= 0 ? h->a[tei].v.p : "";
	int hasTE = te[0] != 0;
	if (done && !hasTE && body_ok && cli < 0 && (!head || plen > 0)) {
		contentLength = plen;
		h_itoa(set_cl, plen);
	}
	int64_t conni = hl_find(h, "Connection");
	const char *connv = conni >= 0 ? h->a[conni].v.p : "";
	int64_t connn = conni >= 0 ? h->a[conni].v.n : 0;
	if (r->wants10ka && cli >= 0 && h->a[cli].v.n > 0 && strcmp(connv, "keep-alive") == 0) {
		close = 0;
	}
	int hasCL = contentLength != -1;
	if (r->wants10ka && (head || hasCL || !body_ok)) {
		if (conni < 0) {
			set_conn = "keep-alive";
		}
	} else if (!is11 || r->wantsclose) {
		close = 1;
	}
	if (strcmp(connv, "close") == 0) {
		close = 1;
	}
	if (expect_unread) {
		close = 1;
	}
	int too_large = 0;
	if (r->clen != 0 && !close) {
		// what remains of the request body is discarded
		if (body_bad) {
			close = 1;
		} else if (too_big) {
			close = 1;
			too_large = 1;
		}
	}
	if (body_ok) {
		int64_t cti = hl_find(h, "Content-Type");
		int64_t cei = hl_find(h, "Content-Encoding");
		int hasCE = cei >= 0 && h->a[cei].v.n > 0;
		if (!hasCE && cti < 0 && !hasTE && plen > 0) {
			set_ct = h_sniff((const unsigned char *)body, plen);
		}
	} else {
		excl[nexcl++] = "Content-Length";
		excl[nexcl++] = "Transfer-Encoding";
		if (status == 304) {
			excl[nexcl++] = "Content-Type";
		}
	}
	if (hl_find(h, "Date") < 0) {
		h_date(date);
	}
	if (hasCL && hasTE && strcmp(te, "identity") != 0) {
		excl[nexcl++] = "Content-Length";
		set_cl[0] = 0;
		hasCL = 0;
	}
	int chunking = 0;
	if (head || !body_ok || status == 204) {
		excl[nexcl++] = "Transfer-Encoding";
	} else if (hasCL) {
		excl[nexcl++] = "Transfer-Encoding";
	} else if (is11) {
		if (hasTE && strcmp(te, "identity") == 0) {
			close = 1;
			excl[nexcl++] = "Transfer-Encoding";
		} else {
			chunking = 1;
			set_te = "chunked";
			if (hasTE && strcmp(te, "chunked") == 0) {
				excl[nexcl++] = "Transfer-Encoding";
			}
		}
	} else {
		close = 1;
		excl[nexcl++] = "Transfer-Encoding";
	}
	if (chunking) {
		excl[nexcl++] = "Content-Length";
		set_cl[0] = 0;
	}
	// isProtocolSwitchResponse: 101 with Upgrade and Connection: upgrade
	int64_t upi = hl_find(h, "Upgrade");
	int proto_switch = status == 101 && upi >= 0 && h->a[upi].v.n > 0 && conni >= 0 && h_has_token(connv, connn, "upgrade");
	if (too_large) {
		excl[nexcl++] = "Connection";
		set_conn = "close";
	}
	if (close && !h_server_token(connv, connn, "close") && !proto_switch) {
		excl[nexcl++] = "Connection";
		if (is11) {
			set_conn = "close";
		}
	}
	hw_status_line(o, is11, status);
	hw_fields(o, h, excl, nexcl);
	if (date[0]) {
		nbuf_str(o, "Date: ");
		nbuf_str(o, date);
		nbuf_put(o, "\r\n", 2);
	}
	if (set_cl[0]) {
		nbuf_str(o, "Content-Length: ");
		nbuf_str(o, set_cl);
		nbuf_put(o, "\r\n", 2);
	}
	if (set_ct) {
		nbuf_str(o, "Content-Type: ");
		nbuf_str(o, set_ct);
		nbuf_put(o, "\r\n", 2);
	}
	if (set_conn) {
		nbuf_str(o, "Connection: ");
		nbuf_str(o, set_conn);
		nbuf_put(o, "\r\n", 2);
	}
	if (set_te) {
		nbuf_str(o, "Transfer-Encoding: ");
		nbuf_str(o, set_te);
		nbuf_put(o, "\r\n", 2);
	}
	nbuf_put(o, "\r\n", 2);
	if (chunked) {
		*chunked = chunking && !head && body_ok;
	}
	if (!head && body_ok && written > 0) {
		if (chunking) {
			int64_t left = written;
			const char *p = body;
			while (left > 0) {
				int64_t c = left > 2048 && !stream ? 2048 : left;
				char hx[24];
				snprintf(hx, sizeof hx, "%llx\r\n", (long long)c);
				nbuf_str(o, hx);
				nbuf_put(o, p, c);
				nbuf_put(o, "\r\n", 2);
				p += c;
				left -= c;
			}
		} else {
			nbuf_put(o, body, written);
		}
	}
	if (stream) {
		// the stream's later writes and its end follow (hs_*)
		return !close;
	}
	if (chunking) {
		nbuf_str(o, "0\r\n\r\n");
	}
	// shouldReuseConnection: a body shorter or longer than its length
	if (!head && contentLength != -1 && body_ok && contentLength != counted) {
		close = 1;
	}
	return !close;
}

static int hw_response(nbuf *o, hreq *r, hlist *h, int64_t status, const char *body, int64_t bn, int expect_unread, int body_bad, int too_big) {
	return hw_response_x(o, r, h, status, body, bn, expect_unread, body_bad, too_big, 0, NULL);
}

// hw_error is the answer of conn.serve to a request it could not read.
static void hw_error(nbuf *o, int kind, int code, const char *reason) {
	const char *hdrs = "\r\nContent-Type: text/plain; charset=utf-8\r\nConnection: close\r\n\r\n";
	char line[256];
	switch (kind) {
	case HP_TOO_LARGE:
		snprintf(line, sizeof line, "HTTP/1.1 431 Request Header Fields Too Large%s431 Request Header Fields Too Large", hdrs);
		break;
	case HP_BAD_TE:
		snprintf(line, sizeof line, "HTTP/1.1 501 Not Implemented%sUnsupported transfer encoding", hdrs);
		break;
	case HP_STATUS:
		snprintf(line, sizeof line, "HTTP/1.1 %d %s: %s%s%d %s: %s", code, h_status_text(code), reason, hdrs, code, h_status_text(code), reason);
		break;
	default:
		snprintf(line, sizeof line, "HTTP/1.1 400 Bad Request%s400 Bad Request", hdrs);
	}
	nbuf_str(o, line);
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
	// the parsing in Xo (decision 0193 step 6, xo_hc_xo): where
	// hc_read_one goes on, and the result Xo passed (xo_hc_head,
	// xo_hc_chunked)
	int xo;
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
	h_init(sc, scope_free);
	sc->prune_at = 64; // finished connections leave the list (scope_prune)
	sc->parent_node = c->node;
	xo_node *root = __atomic_load_n(&stop.root, __ATOMIC_ACQUIRE);
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
	if (getpeername(cn->pd->fd, (struct sockaddr *)&ss, &sl) == 0) {
		if (ss.ss_family == AF_INET) {
			inet_ntop(AF_INET, &((struct sockaddr_in *)&ss)->sin_addr, hc->ip, sizeof hc->ip);
		} else if (ss.ss_family == AF_INET6) {
			inet_ntop(AF_INET6, &((struct sockaddr_in6 *)&ss)->sin6_addr, hc->ip, sizeof hc->ip);
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
// With the parsing in Xo (hc->xo, std/native/llrt/http1.xo) also 3, a
// request head to parse at the unread input, and 4, a chunked body to
// decode there: the fiber's Xo code parses and passes the result
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
		} else if (hc->xo) {
			if (!hc->rd_have) {
				hc->rd_phase = RD_HEAD;
				return 3;
			}
			hc->rd_have = 0;
			r = hc->rd_res;
			used = hc->rd_used;
		} else {
			r = hp_head(hc->buf ? hc->buf + hc->off : hc->buf, hc->n - hc->off, hc->eof, &hc->req, &used);
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
				shutdown(hc->conn->pd->fd, SHUT_WR);
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
				int b;
				if (hc->xo) {
					if (!hc->rd_have) {
						hc->rd_phase = RD_BODY;
						return 4;
					}
					hc->rd_have = 0;
					b = hc->rd_res;
					used = hc->rd_used;
				} else {
					b = hb_chunked(hc->buf + hc->off, have, hc->eof, &hc->body, &used);
				}
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
// with the parsing in Xo also 3 and 4 (hc_read_one).
int64_t xo_hc_read(xo_hconn *hc) {
	for (;;) {
		int64_t r = hc_read_one(hc);
		if (r != 2) {
			return r;
		}
	}
}

// xo_hc_xo makes the connection's request heads and chunked bodies parse
// in Xo (std/native/llrt/http1.xo; decision 0193 step 6).
void xo_hc_xo(xo_hconn *hc) { hc->xo = 1; }

// xo_hc_input is the unread input (what hc_read stopped at), or a part
// of it whose parse is the whole's (pipelined requests would otherwise be
// copied again for each): mode 0 up to the end of the first head (the
// first empty line: \n\n or \n\r\n), mode 1 at most 4 KiB (a chunked
// body's first try), 2 all.
xo_rstr *xo_hc_input(xo_hconn *hc, int64_t mode, int64_t *len) {
	const char *p = hc->buf ? hc->buf + hc->off : "";
	int64_t n = hc->n - hc->off;
	if (mode == 0) {
		for (const char *nl = memchr(p, '\n', (size_t)n); nl; nl = memchr(nl + 1, '\n', (size_t)(n - (nl + 1 - p)))) {
			int64_t e = nl + 1 - p;
			if (e < n && p[e] == '\n') {
				n = e + 1;
				break;
			}
			if (e + 1 < n && p[e] == '\r' && p[e + 1] == '\n') {
				n = e + 2;
				break;
			}
		}
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

// xo_hc_host is Request.url.host: the Host without a port (net.
// SplitHostPort), or all of it when it has none.
xo_rstr *xo_hc_host(xo_hconn *hc, int64_t *len) {
	hstr h = hc->req.host;
	const char *p = h.p ? h.p : "";
	int64_t n = h.n;
	// net.SplitHostPort
	const char *colon = NULL;
	for (int64_t i = n - 1; i >= 0; i--) {
		if (p[i] == ':') {
			colon = p + i;
			break;
		}
	}
	if (colon) {
		const char *hp = p;
		int64_t hn = colon - p;
		int ok = 1;
		int64_t j = 0, k = 0; // no '[' from j, no ']' from k
		if (n > 0 && p[0] == '[') {
			const char *end = memchr(p, ']', (size_t)n);
			if (!end || end + 1 != colon) {
				ok = 0;
			} else {
				hp = p + 1;
				hn = end - p - 1;
				j = 1;
				k = end - p + 1;
			}
		} else if (memchr(p, ':', (size_t)hn)) {
			ok = 0;
		}
		if (ok && (memchr(p + j, '[', (size_t)(n - j)) || memchr(p + k, ']', (size_t)(n - k)))) {
			ok = 0;
		}
		if (ok) {
			return nstr(hp, hn, len);
		}
	}
	return nstr(p, n, len);
}

// xo_hc_port is the URL's port (an absolute request target), or -1.
int64_t xo_hc_port(xo_hconn *hc) {
	hstr h = hc->req.url.host;
	if (!h.p) {
		return -1;
	}
	for (int64_t i = h.n - 1; i >= 0; i--) {
		if (h.p[i] == ':') {
			if (!h_valid_port(h.p + i, h.n - i) || i + 1 == h.n) {
				return -1;
			}
			int64_t v;
			if (!n_parse_int(h.p + i + 1, h.n - i - 1, &v)) {
				return -1;
			}
			return v;
		}
	}
	return -1;
}

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
		char cl[24];
		int cn = h_itoa(cl, bn);
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
// Text helpers of std/http

xo_rstr *xo_canonical_key(const char *p, int64_t n, int64_t *len) {
	xo_rstr *s = nstr(p, n, len);
	h_canon(s->data, n);
	return s;
}

int64_t xo_equal_fold(const char *a, int64_t an, const char *b, int64_t bn) {
	if (an != bn) {
		return 0;
	}
	for (int64_t i = 0; i < an; i++) {
		if (h_lower((unsigned char)a[i]) != h_lower((unsigned char)b[i])) {
			return 0;
		}
	}
	return 1;
}

// xo_path_unescape is url.PathUnescape: "" when an escape is invalid.
xo_rstr *xo_path_unescape(const char *p, int64_t n, int64_t *len) {
	if (!memchr(p, '%', (size_t)n)) {
		return nstr(p, n, len);
	}
	nbuf b = {0};
	if (!h_unescape(p, n, U_PATH, &b)) {
		free(b.p);
		return nstr("", 0, len);
	}
	return nbuf_take(&b, len);
}

// h_go_quote appends strconv.Quote(s) (printable: ASCII graphic and
// every rune from U+00A1 but U+00AD; an approximation of IsPrint).
static void h_go_quote(nbuf *o, const char *p, int64_t n) {
	nbuf_byte(o, '"');
	const unsigned char *s = (const unsigned char *)p;
	for (int64_t i = 0; i < n;) {
		int size;
		int32_t r = n_utf8_decode(s + i, n - i, &size);
		char t[16];
		if (r == 0xFFFD && size == 1) {
			snprintf(t, sizeof t, "\\x%02x", s[i]);
			nbuf_str(o, t);
		} else if (r == '"' || r == '\\') {
			t[0] = '\\';
			t[1] = (char)r;
			nbuf_put(o, t, 2);
		} else if ((r >= 0x20 && r < 0x7f) || (r >= 0xA1 && r != 0xAD && r != 0x2028 && r != 0x2029)) {
			nbuf_put(o, p + i, size);
		} else {
			switch (r) {
			case '\a':
				nbuf_str(o, "\\a");
				break;
			case '\b':
				nbuf_str(o, "\\b");
				break;
			case '\f':
				nbuf_str(o, "\\f");
				break;
			case '\n':
				nbuf_str(o, "\\n");
				break;
			case '\r':
				nbuf_str(o, "\\r");
				break;
			case '\t':
				nbuf_str(o, "\\t");
				break;
			case '\v':
				nbuf_str(o, "\\v");
				break;
			default:
				if (r < 0x20 || r == 0x7f) {
					snprintf(t, sizeof t, "\\x%02x", (unsigned)r);
				} else if (r < 0x10000) {
					snprintf(t, sizeof t, "\\u%04x", (unsigned)r);
				} else {
					snprintf(t, sizeof t, "\\U%08x", (unsigned)r);
				}
				nbuf_str(o, t);
			}
		}
		i += size;
	}
	nbuf_byte(o, '"');
}

xo_rstr *xo_go_quote(const char *p, int64_t n, int64_t *len) {
	nbuf b = {0};
	h_go_quote(&b, p, n);
	return nbuf_take(&b, len);
}

// url.QueryEscape
xo_rstr *xo_query_escape(const char *p, int64_t n, int64_t *len) {
	nbuf b = {0};
	for (int64_t i = 0; i < n; i++) {
		unsigned c = (unsigned char)p[i];
		if ((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '~') {
			nbuf_byte(&b, (char)c);
		} else if (c == ' ') {
			nbuf_byte(&b, '+');
		} else {
			char t[4];
			snprintf(t, sizeof t, "%%%02X", c);
			nbuf_put(&b, t, 3);
		}
	}
	return nbuf_take(&b, len);
}

// h_escape_path is url.escape(s, encodePath).
static void h_escape_path(nbuf *b, const char *p, int64_t n) {
	for (int64_t i = 0; i < n; i++) {
		unsigned c = (unsigned char)p[i];
		if ((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || strchr("-_.~$&+,/:;=@", (int)c) != NULL) {
			if (c != 0) {
				nbuf_byte(b, (char)c);
				continue;
			}
		}
		char t[4];
		snprintf(t, sizeof t, "%%%02X", c);
		nbuf_put(b, t, 3);
	}
}

static xo_rlist *strs_list(hstr *xs, int n) {
	xo_rlist *l = n_str_list_new(n);
	for (int i = 0; i < n; i++) {
		l = n_str_list_add(l, xs[i].p ? xs[i].p : "", xs[i].n);
	}
	return l;
}

// h_split_hostport is url's splitHostPort: the hostname (brackets
// removed) and the port when it is valid.
static void h_split_hostport(hstr host, hstr *name, hstr *port) {
	const char *p = host.p ? host.p : "";
	int64_t n = host.n, hn = n;
	*port = hs_dup("", 0);
	for (int64_t i = n - 1; i >= 0; i--) {
		if (p[i] == ':') {
			if (h_valid_port(p + i, n - i)) {
				hs_free(port);
				*port = hs_dup(p + i + 1, n - i - 1);
				hn = i;
			}
			break;
		}
	}
	if (hn >= 2 && p[0] == '[' && p[hn - 1] == ']') {
		*name = hs_dup(p + 1, hn - 2);
	} else {
		*name = hs_dup(p, hn);
	}
}

// xo_url_parts is url.Parse (fragment cut) or ParseRequestURI: [] when it
// fails, else [scheme, hostname, port, path, raw query, host].
xo_rlist *xo_url_parts(const char *p, int64_t n, int64_t request) {
	int64_t un = n;
	if (!request) {
		const char *h = memchr(p, '#', (size_t)n);
		if (h) {
			un = h - p;
		}
	}
	hurl u;
	char err[200];
	if (!h_url_parse(p, un, (int)request, &u, err, sizeof err)) {
		return n_str_list_new(0);
	}
	hstr name, port;
	h_split_hostport(u.host, &name, &port);
	hstr parts[6] = {u.scheme, name, port, u.path, u.rawquery, u.host};
	xo_rlist *l = strs_list(parts, 6);
	hs_free(&name);
	hs_free(&port);
	hu_free(&u);
	return l;
}

// xo_url_error is the text of url.Parse's error: parse "url": reason.
xo_rstr *xo_url_error(const char *p, int64_t n, int64_t *len) {
	int64_t un = n;
	const char *h = memchr(p, '#', (size_t)n);
	if (h) {
		un = h - p;
	}
	hurl u;
	char err[200];
	nbuf b = {0};
	if (h_url_parse(p, un, 0, &u, err, sizeof err)) {
		hu_free(&u);
		return nstr("", 0, len);
	}
	nbuf_str(&b, "parse ");
	h_go_quote(&b, p, un);
	nbuf_str(&b, ": ");
	nbuf_str(&b, err);
	return nbuf_take(&b, len);
}

// xo_query_parts is url.ParseQuery as std/rt's urlOf uses it: the keys
// sorted with their first values.
xo_rlist *xo_query_parts(const char *p, int64_t n) {
	hlist q = {0};
	h_parse_query(p, n, &q);
	xo_rlist *l = n_str_list_new(q.n * 2);
	for (int64_t i = 0; i < q.n; i++) {
		l = n_str_list_add(l, q.a[i].k.p, q.a[i].k.n);
		l = n_str_list_add(l, q.a[i].v.p, q.a[i].v.n);
	}
	hl_free(&q);
	return l;
}

// ---------------------------------------------------------------------------
// Logging (std.md 6)

static struct {
	int level; // 0: not configured (Info)
	int json;
	int out; // log.configure was called: records go to standard error
	         // (the fake one in a test) instead of the test's buffer
} xlog;

int64_t xo_log_level(void) {
	int l = __atomic_load_n(&xlog.level, __ATOMIC_ACQUIRE);
	return l ? l : 2;
}

int64_t xo_log_json(void) { return __atomic_load_n(&xlog.json, __ATOMIC_ACQUIRE); }

int64_t xo_log_configured(void) { return __atomic_load_n(&xlog.out, __ATOMIC_ACQUIRE); }

void xo_log_configure(int64_t level, int64_t json) {
	__atomic_store_n(&xlog.out, 1, __ATOMIC_RELEASE);
	__atomic_store_n(&xlog.json, (int)json, __ATOMIC_RELEASE);
	__atomic_store_n(&xlog.level, (int)level, __ATOMIC_RELEASE);
}

// The dependency that writes the next record (decision 0173): the
// native code generator calls xo_log_from just before a dependency's call
// of log.debug/info/warn/error, whose implementation takes it at entry
// (no task switch in between, so a thread local holds it).
static __thread char *xlog_from;
static __thread int64_t xlog_from_n;

void xo_log_from(const char *p, int64_t n) {
	free(xlog_from);
	xlog_from = malloc((size_t)n + 1);
	if (!xlog_from) {
		xo_oom();
	}
	memcpy(xlog_from, p, (size_t)n);
	xlog_from[n] = 0;
	xlog_from_n = n;
}

xo_rstr *xo_log_from_take(int64_t *len) {
	if (!xlog_from) {
		return nstr("", 0, len);
	}
	xo_rstr *s = nstr(xlog_from, xlog_from_n, len);
	free(xlog_from);
	xlog_from = NULL;
	xlog_from_n = 0;
	return s;
}

// xo_log_now is the time now in UTC as RFC 3339 (with nanoseconds when
// nano, as time.RFC3339Nano).
xo_rstr *xo_log_now(int64_t nano, int64_t *len) {
	char b[64];
	int64_t t = xo_clock_now();
	if (!nano) {
		t -= ((t % 1000000000) + 1000000000) % 1000000000;
	}
	int k = (int)xo_x_fmt_time(XO_ADDR(b), t);
	return nstr(b, k, len);
}

// ---------------------------------------------------------------------------
// The client (std.md 4.5): one request, with the redirects net/http's
// Client follows, over a connection of its own.

typedef struct {
	xo_hdr h;
	int err; // 0, or the ClientErr tag
	char *msg;
	int64_t status;
	hlist hdr;
	nbuf body;
} xo_cresp;

static void cresp_free(void *p) {
	xo_cresp *r = p;
	free(r->msg);
	hl_free(&r->hdr);
	free(r->body.p);
	xo_free(r);
}

typedef struct {
	xo_pd *pd;
	int fd;
	int64_t when; // deadline (monotonic), 0: none
	char *buf;
	int64_t n, cap, off;
	int eof;
	tls_conn *tls; // an https connection (xo_tls.c)
} ccon;

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

// urlErrorOp is the method as net/http's url.Error names it: "Get".
static void url_op(char *out, const char *m, int64_t mn) {
	int64_t k = mn < 30 ? mn : 30;
	for (int64_t i = 0; i < k; i++) {
		out[i] = (char)(i == 0 ? m[i] : h_lower((unsigned char)m[i]));
	}
	out[k] = 0;
}

// cl_read_response reads a response to a request of method into r (its
// header and body, as net/http's ReadResponse and readTransfer).
static int cl_read_response(ccon *c, const char *method, xo_cresp *r, char *why, size_t wn) {
	for (;;) {
		// the status line and the header, up to an empty line
		int64_t end = -1;
		for (;;) {
			const char *b = c->buf ? c->buf + c->off : c->buf;
			int64_t n = c->n - c->off;
			for (int64_t i = 0; i + 1 < n; i++) {
				if (b[i] == '\n' && (b[i + 1] == '\n' || (b[i + 1] == '\r' && i + 2 < n && b[i + 2] == '\n'))) {
					end = i + (b[i + 1] == '\n' ? 2 : 3);
					break;
				}
			}
			if (end >= 0) {
				break;
			}
			if (n > HTTP_MAX_HEAD) {
				snprintf(why, wn, "net/http: server response headers exceeded %d bytes; aborted", 1 << 20);
				return CC_ERR;
			}
			int f = c->eof ? CC_EOF : cc_fill(c);
			if (f == CC_EOF) {
				snprintf(why, wn, n == 0 ? "EOF" : "unexpected EOF");
				return CC_ERR;
			}
			if (f != CC_OK) {
				return f;
			}
		}
		const char *b = c->buf ? c->buf + c->off : c->buf;
		// status line: HTTP/x.y code reason
		const char *nl = memchr(b, '\n', (size_t)end);
		int64_t ll = nl - b;
		if (ll > 0 && b[ll - 1] == '\r') {
			ll--;
		}
		const char *sp = memchr(b, ' ', (size_t)ll);
		if (!sp || sp - b < 8 || memcmp(b, "HTTP/", 5) != 0) {
			snprintf(why, wn, "net/http: HTTP/1.x transport connection broken: malformed HTTP response %.*s", (int)(ll < 80 ? ll : 80), b);
			return CC_ERR;
		}
		const char *cs = sp + 1;
		int64_t code = 0, digits = 0;
		while (cs < b + ll && *cs >= '0' && *cs <= '9' && digits < 4) {
			code = code * 10 + (*cs - '0');
			cs++;
			digits++;
		}
		if (digits != 3) {
			snprintf(why, wn, "net/http: HTTP/1.x transport connection broken: malformed HTTP status code");
			return CC_ERR;
		}
		// the header fields
		hl_free(&r->hdr);
		int64_t i = nl - b + 1;
		int chunked = 0;
		int64_t clen = -1;
		int conn_close = 0;
		while (i < end) {
			const char *ln = b + i;
			const char *e = memchr(ln, '\n', (size_t)(end - i));
			int64_t len = e - ln;
			int64_t next = i + len + 1;
			if (len > 0 && ln[len - 1] == '\r') {
				len--;
			}
			if (len == 0) {
				break;
			}
			const char *colon = memchr(ln, ':', (size_t)len);
			if (colon) {
				hstr k = hs_dup(ln, colon - ln);
				h_canon(k.p, k.n);
				const char *v = colon + 1;
				int64_t vn = len - (colon - ln) - 1;
				while (vn > 0 && (*v == ' ' || *v == '\t')) {
					v++;
					vn--;
				}
				while (vn > 0 && (v[vn - 1] == ' ' || v[vn - 1] == '\t')) {
					vn--;
				}
				if (hs_eq(k, "Transfer-Encoding")) {
					chunked = vn == 7 && strncasecmp(v, "chunked", 7) == 0;
					hs_free(&k);
				} else {
					if (hs_eq(k, "Content-Length")) {
						int64_t x;
						if (n_parse_int(v, vn, &x)) {
							clen = x;
						}
					}
					if (hs_eq(k, "Connection") && h_has_token(v, vn, "close")) {
						conn_close = 1;
					}
					int64_t at = -1;
					for (int64_t q = 0; q < r->hdr.n; q++) {
						if (r->hdr.a[q].k.n == k.n && memcmp(r->hdr.a[q].k.p, k.p, (size_t)k.n) == 0) {
							at = q;
						}
					}
					if (at >= 0) {
						nbuf j = {0};
						nbuf_put(&j, r->hdr.a[at].v.p, r->hdr.a[at].v.n);
						nbuf_put(&j, ", ", 2);
						nbuf_put(&j, v, vn);
						hs_free(&r->hdr.a[at].v);
						r->hdr.a[at].v = hs_dup(j.p, j.n);
						free(j.p);
						hs_free(&k);
					} else {
						hl_add(&r->hdr, k, hs_dup(v, vn));
					}
				}
			}
			i = next;
		}
		c->off += end;
		if (code >= 100 && code <= 199 && code != 101) {
			// an informational response: the real one follows
			continue;
		}
		r->status = code;
		if (chunked) {
			hl_del(&r->hdr, "Content-Length");
		}
		r->body.n = 0;
		int nobody = strcmp(method, "HEAD") == 0 || code == 204 || code == 304 || (code >= 100 && code <= 199);
		if (nobody) {
			return CC_OK;
		}
		if (chunked) {
			for (;;) {
				int64_t used = 0;
				int h = hb_chunked(c->buf + c->off, c->n - c->off, c->eof, &r->body, &used);
				if (h == HB_OK) {
					c->off += used;
					return CC_OK;
				}
				if (h == HB_ERR) {
					snprintf(why, wn, c->eof ? "unexpected EOF" : "malformed chunked encoding");
					return CC_ERR;
				}
				int f = cc_fill(c);
				if (f == CC_EOF) {
					continue;
				}
				if (f != CC_OK) {
					return f;
				}
			}
		}
		for (;;) {
			int64_t have = c->n - c->off;
			if (clen >= 0 && have >= clen) {
				nbuf_put(&r->body, c->buf + c->off, clen);
				c->off += clen;
				return CC_OK;
			}
			if (c->eof) {
				if (clen >= 0) {
					snprintf(why, wn, "unexpected EOF");
					return CC_ERR;
				}
				nbuf_put(&r->body, c->buf + c->off, have);
				c->off = c->n;
				return CC_OK;
			}
			(void)conn_close;
			int f = cc_fill(c);
			if (f != CC_OK && f != CC_EOF) {
				return f;
			}
		}
	}
}

// HTTP/2 (xo_h2.c)
typedef struct h2cc h2cc;
static int h2_cl_pooled(xo_troots *roots, const char *key, const char *method, hurl *u, hstr host, hlist *hdr, const char *body,
                        int64_t bn, int64_t when, xo_cresp *r, char *why, size_t wn, int *done);
static int h2_cl_fresh(xo_conn *cn, ccon *c, xo_troots *roots, const char *key, const char *method, hurl *u, hstr host, hlist *hdr,
                       const char *body, int64_t bn, int64_t when, xo_cresp *r, char *why, size_t wn, char **alert);

// cl_once makes one request: CC_OK and the response in r, or the
// failure (r's error set). An https server whose ALPN chose h2 gets the
// request over HTTP/2 (xo_h2.c), on a connection kept for the next
// request as net/http's Transport keeps it; HTTP/1.1 requests have a
// connection each.
static int cl_once(const char *method, hurl *u, hstr host, hlist *hdr, const char *body, int64_t bn, int64_t when, const char *raw, xo_troots *roots, xo_cresp *r) {
	xo_ctx *x = ctx();
	char op[32];
	url_op(op, method, (int64_t)strlen(method));
	nbuf qraw = {0};
	h_go_quote(&qraw, raw, (int64_t)strlen(raw));
	nbuf_byte(&qraw, 0);
	// the address: host:port, 80 (http) or 443 (https) by default
	int https = hs_eq(u->scheme, "https");
	const char *dport = https ? "443" : "80";
	hstr name, port;
	h_split_hostport(u->host, &name, &port);
	char addr[600];
	if (strchr(name.p, ':')) {
		snprintf(addr, sizeof addr, "[%s]:%s", name.p, port.n ? port.p : dport);
	} else {
		snprintf(addr, sizeof addr, "%s:%s", name.p, port.n ? port.p : dport);
	}
	char *tls_host = https ? cstr_of(name.p, name.n) : NULL;
	hs_free(&name);
	hs_free(&port);
	char why[400] = "";
	char *alert = NULL;
	char *key = NULL;
	int s;
	if (https) {
		// an idle HTTP/2 connection to the same address, server name, and
		// roots
		nbuf k = {0};
		nbuf_str(&k, addr);
		nbuf_byte(&k, '|');
		nbuf_str(&k, tls_host);
		nbuf_byte(&k, 0);
		key = k.p;
		int done = 0;
		s = h2_cl_pooled(roots, key, method, u, host, hdr, body, bn, when, r, why, sizeof why, &done);
		if (done) {
			goto finish;
		}
	}
	int64_t d = when ? when - mono_ns() : 0;
	if (when && d <= 0) {
		cr_fail(r, 3, "");
		free(qraw.p);
		free(tls_host);
		free(key);
		return CC_TIMEOUT;
	}
	xo_conn *cn = xo_net_dial_timeout(addr, (int64_t)strlen(addr), when ? d : 0);
	if (x->net_status != 0) {
		int st = (int)x->net_status;
		const char *m = x->net_msg ? x->net_msg : "";
		if (st == 5) {
			cr_fail(r, 5, "");
		} else if (st == 2) {
			cr_fail(r, 2, "dial tcp %s: connect: connection refused", addr);
		} else if (strstr(m, "i/o timeout")) {
			cr_fail(r, 3, "");
		} else {
			cr_fail(r, 2, "%s", m);
		}
		free(qraw.p);
		free(tls_host);
		free(key);
		return CC_ERR;
	}
	ccon c = {.pd = cn->pd, .fd = cn->pd->fd, .when = when};
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
				cr_fail(r, 4, "%s %s: %s", op, qraw.p, tmsg);
			}
			free(qraw.p);
			free(tls_host);
			free(key);
			return ts;
		}
	}
	if (c.tls && tls_alpn_h2(c.tls)) {
		s = h2_cl_fresh(cn, &c, roots, key, method, u, host, hdr, body, bn, when, r, why, sizeof why, &alert);
		goto finish;
	}
	// the request (net/http Request.write with Transport's additions)
	nbuf o = {0};
	nbuf_str(&o, method);
	nbuf_byte(&o, ' ');
	if (hs_eq(u->path, "*")) {
		nbuf_byte(&o, '*');
	} else {
		if (u->path.n == 0) {
			nbuf_byte(&o, '/');
		}
		h_escape_path(&o, u->path.p, u->path.n);
	}
	if (u->rawquery.n > 0) {
		nbuf_byte(&o, '?');
		nbuf_put(&o, u->rawquery.p, u->rawquery.n);
	}
	nbuf_str(&o, " HTTP/1.1\r\nHost: ");
	nbuf_put(&o, host.p, host.n);
	nbuf_str(&o, "\r\n");
	int64_t ua = hl_find(hdr, "User-Agent");
	if (ua < 0) {
		nbuf_str(&o, "User-Agent: Go-http-client/1.1\r\n");
	} else if (hdr->a[ua].v.n > 0) {
		nbuf_str(&o, "User-Agent: ");
		nbuf_put(&o, hdr->a[ua].v.p, hdr->a[ua].v.n);
		nbuf_str(&o, "\r\n");
	}
	if (bn > 0 || strcmp(method, "POST") == 0 || strcmp(method, "PUT") == 0 || strcmp(method, "PATCH") == 0) {
		char cl[48];
		snprintf(cl, sizeof cl, "Content-Length: %lld\r\n", (long long)bn);
		nbuf_str(&o, cl);
	}
	static const char *ex[] = {"Host", "User-Agent", "Content-Length", "Transfer-Encoding", "Trailer"};
	hw_fields(&o, hdr, ex, 5);
	if (hl_find(hdr, "Accept-Encoding") < 0 && hl_find(hdr, "Range") < 0 && strcmp(method, "HEAD") != 0) {
		nbuf_str(&o, "Accept-Encoding: gzip\r\n");
	}
	nbuf_str(&o, "\r\n");
	nbuf_put(&o, body, bn);
	s = cc_send(&c, o.p, o.n);
	free(o.p);
	if (s == CC_OK) {
		s = cl_read_response(&c, method, r, why, sizeof why);
	}
	{
		// an alert of the server after the handshake (a TLS 1.3 server
		// that wanted a client certificate): Go's net.OpError, a Connect
		// error
		const char *ta = s != CC_OK && s != CC_TIMEOUT && s != CC_CANCELED && !why[0] ? tls_alert(c.tls) : NULL;
		alert = ta ? strdup(ta) : NULL;
	}
	tls_conn_free(c.tls);
	pd_close(c.pd);
	h_release(cn);
	free(c.buf);
finish:
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
			cr_fail(r, 4, "%s %s: %s", op, qraw.p, why[0] ? why : "EOF");
		}
	}
	free(alert);
	free(qraw.p);
	free(tls_host);
	free(key);
	return s;
}

static int valid_method(const char *m, int64_t n) {
	if (n == 0) {
		return 0;
	}
	for (int64_t i = 0; i < n; i++) {
		if (!h_tchar((unsigned char)m[i])) {
			return 0;
		}
	}
	return 1;
}

// xo_cl_do is `client.send(req)` over the network: method to url (the
// text of the request's Url), with the header of names and values, the
// body, and the client's timeout (ns, 0: none).
xo_cresp *xo_cl_do(const char *m, int64_t mn, const char *up, int64_t un, xo_rlist *names, xo_rlist *values, const char *b,
                   int64_t bn, int64_t timeout, xo_troots *roots, const char *lp, int64_t ln) {
	xo_cresp *r = xo_alloc(sizeof(xo_cresp));
	memset(r, 0, sizeof *r);
	h_init(r, cresp_free);
	char *method = cstr_of(m, mn);
	char *raw = cstr_of(up, un);
	if (!valid_method(m, mn)) {
		nbuf q = {0};
		h_go_quote(&q, m, mn);
		nbuf_byte(&q, 0);
		cr_fail(r, 1, "net/http: invalid method %s", q.p);
		free(q.p);
		free(method);
		free(raw);
		return r;
	}
	// the initial request's header (Header.Set: canonical names, the last
	// value of a name)
	hlist ihdr = {0};
	for (int64_t i = 0; i < names->len && i < values->len; i++) {
		xo_rstr *k = ((xo_rstr **)rl_data(names))[2 * i];
		int64_t kn = ((int64_t *)rl_data(names))[2 * i + 1];
		xo_rstr *v = ((xo_rstr **)rl_data(values))[2 * i];
		int64_t vn = ((int64_t *)rl_data(values))[2 * i + 1];
		hstr key = hs_dup(k->data, kn);
		h_canon(key.p, kn);
		int64_t at = -1;
		for (int64_t q = 0; q < ihdr.n; q++) {
			if (ihdr.a[q].k.n == kn && memcmp(ihdr.a[q].k.p, key.p, (size_t)kn) == 0) {
				at = q;
			}
		}
		if (at >= 0) {
			hs_free(&key);
			hs_free(&ihdr.a[at].v);
			ihdr.a[at].v = hs_dup(v->data, vn);
		} else {
			hl_add(&ihdr, key, hs_dup(v->data, vn));
		}
	}
	int64_t when = timeout > 0 ? deadline(timeout) : 0;
	char op[32];
	url_op(op, m, mn);
	const char *body = b;
	int64_t blen = bn;
	int include_body = 1, strip_sensitive = 0;
	char *ihost = NULL;     // the initial request's host
	char *referer = NULL;   // the URL of the previous request
	char *last_loc = NULL;  // the Location being followed
	for (int redirects = 0;; redirects++) {
		hurl u;
		char err[200];
		int64_t cut = (int64_t)strlen(raw);
		const char *hash = strchr(raw, '#');
		if (hash) {
			cut = hash - raw;
		}
		if (!h_url_parse(raw, cut, 0, &u, err, sizeof err)) {
			nbuf q = {0};
			h_go_quote(&q, raw, (int64_t)strlen(raw));
			nbuf_byte(&q, 0);
			cr_fail(r, redirects == 0 ? 1 : 4, "parse %s: %s", q.p, err);
			free(q.p);
			break;
		}
		if (redirects > 0 && ln > 0) {
			// a redirect must stay within the client's attenuated Net
			// (Net.only, decision 0175): std/rt's CheckRedirect, whose
			// error names the Location
			hstr name, port;
			h_split_hostport(u.host, &name, &port);
			if (port.n == 0) {
				hs_free(&port);
				port = hs_eq(u.scheme, "https") ? hs_dup("443", 3) : hs_dup("80", 2);
			}
			if (!net_admits(lp, (size_t)ln, name.p ? name.p : "", (size_t)name.n, port.p, (size_t)port.n)) {
				nbuf lq = {0};
				h_go_quote(&lq, last_loc, (int64_t)strlen(last_loc));
				nbuf_byte(&lq, 0);
				char op0[32];
				url_op(op0, m, mn);
				int v6 = name.n > 0 && memchr(name.p, ':', (size_t)name.n) != NULL;
				cr_fail(r, 4, "%s %s: denied by Net.only: redirect to %s%.*s%s:%.*s", op0, lq.p, v6 ? "[" : "", (int)name.n,
				        name.p ? name.p : "", v6 ? "]" : "", (int)port.n, port.p);
				free(lq.p);
				hs_free(&name);
				hs_free(&port);
				hu_free(&u);
				break;
			}
			hs_free(&name);
			hs_free(&port);
		}
		nbuf q = {0};
		h_go_quote(&q, raw, (int64_t)strlen(raw));
		nbuf_byte(&q, 0);
		if (!hs_eq(u.scheme, "http") && !hs_eq(u.scheme, "https")) {
			nbuf s = {0};
			h_go_quote(&s, u.scheme.p, u.scheme.n);
			nbuf_byte(&s, 0);
			cr_fail(r, 4, "%s %s: unsupported protocol scheme %s", op, q.p, s.p);
			free(s.p);
			free(q.p);
			hu_free(&u);
			break;
		}
		if (u.host.n == 0) {
			cr_fail(r, 4, "%s %s: http: no Host in request URL", op, q.p);
			free(q.p);
			hu_free(&u);
			break;
		}
		free(q.p);
		if (!ihost) {
			ihost = cstr_of(u.host.p, u.host.n);
		} else if (strcmp(ihost, u.host.p) != 0) {
			// another host: Authorization and cookies are not sent there
			strip_sensitive = 1;
		}
		// this hop's header: the initial one, less what a redirect drops
		hlist hdr = {0};
		for (int64_t i = 0; i < ihdr.n; i++) {
			hstr k = ihdr.a[i].k;
			int sensitive = hs_eq(k, "Authorization") || hs_eq(k, "Www-Authenticate") || hs_eq(k, "Cookie") || hs_eq(k, "Cookie2") ||
			                hs_eq(k, "Proxy-Authorization") || hs_eq(k, "Proxy-Authenticate");
			int bodyh = hs_eq(k, "Content-Encoding") || hs_eq(k, "Content-Language") || hs_eq(k, "Content-Location") || hs_eq(k, "Content-Type");
			if (redirects > 0 && ((sensitive && strip_sensitive) || (bodyh && !include_body))) {
				continue;
			}
			hl_add(&hdr, hs_dup(k.p, k.n), hs_dup(ihdr.a[i].v.p, ihdr.a[i].v.n));
		}
		// refererForURL: no Referer from an https URL to an http one
		if (redirects > 0 && referer && strncmp(referer, "https://", 8) == 0 && hs_eq(u.scheme, "http")) {
			free(referer);
			referer = NULL;
		}
		if (redirects > 0 && referer && hl_find(&hdr, "Referer") < 0) {
			hl_add(&hdr, hs_dup("Referer", 7), hs_dup(referer, (int64_t)strlen(referer)));
		}
		hstr host = hs_dup(u.host.p, u.host.n);
		int s = cl_once(method, &u, host, &hdr, body, blen, when, raw, roots, r);
		hs_free(&host);
		hl_free(&hdr);
		if (s != CC_OK) {
			hu_free(&u);
			break;
		}
		// redirects (Client.Do's redirectBehavior)
		int64_t st = r->status;
		int64_t li = hl_find(&r->hdr, "Location");
		int follow = (st == 301 || st == 302 || st == 303 || st == 307 || st == 308) && li >= 0 && r->hdr.a[li].v.n > 0;
		if (!follow) {
			hu_free(&u);
			break;
		}
		free(last_loc);
		last_loc = cstr_of(r->hdr.a[li].v.p, r->hdr.a[li].v.n);
		if (redirects == 9) {
			// defaultCheckRedirect: the error names the Location
			nbuf lq = {0};
			h_go_quote(&lq, last_loc, (int64_t)strlen(last_loc));
			nbuf_byte(&lq, 0);
			char op0[32];
			url_op(op0, m, mn);
			cr_fail(r, 4, "%s %s: stopped after 10 redirects", op0, lq.p);
			free(lq.p);
			hu_free(&u);
			break;
		}
		// the Referer: this request's URL (url.URL.String)
		nbuf ref = {0};
		nbuf_put(&ref, u.scheme.p, u.scheme.n);
		nbuf_str(&ref, "://");
		nbuf_put(&ref, u.host.p, u.host.n);
		h_escape_path(&ref, u.path.p, u.path.n);
		if (u.rawquery.n > 0) {
			nbuf_byte(&ref, '?');
			nbuf_put(&ref, u.rawquery.p, u.rawquery.n);
		}
		nbuf_byte(&ref, 0);
		free(referer);
		referer = ref.p;
		// the new URL: the Location resolved against this one
		hstr loc = hs_dup(last_loc, (int64_t)strlen(last_loc));
		nbuf next = {0};
		if (memchr(loc.p, ':', (size_t)loc.n) && !(loc.n > 0 && loc.p[0] == '/')) {
			nbuf_put(&next, loc.p, loc.n);
		} else if (loc.n >= 2 && loc.p[0] == '/' && loc.p[1] == '/') {
			nbuf_put(&next, u.scheme.p, u.scheme.n);
			nbuf_byte(&next, ':');
			nbuf_put(&next, loc.p, loc.n);
		} else {
			nbuf_put(&next, u.scheme.p, u.scheme.n);
			nbuf_str(&next, "://");
			nbuf_put(&next, u.host.p, u.host.n);
			if (loc.n > 0 && loc.p[0] == '/') {
				nbuf_put(&next, loc.p, loc.n);
			} else {
				// relative to the current path's directory
				const char *slash = NULL;
				for (int64_t k = u.path.n - 1; k >= 0; k--) {
					if (u.path.p[k] == '/') {
						slash = u.path.p + k;
						break;
					}
				}
				if (slash) {
					h_escape_path(&next, u.path.p, slash - u.path.p + 1);
				} else {
					nbuf_byte(&next, '/');
				}
				nbuf_put(&next, loc.p, loc.n);
			}
		}
		nbuf_byte(&next, 0);
		hs_free(&loc);
		hu_free(&u);
		free(raw);
		raw = next.p;
		if (st == 301 || st == 302 || st == 303) {
			if (strcmp(method, "GET") != 0 && strcmp(method, "HEAD") != 0) {
				free(method);
				method = strdup("GET");
			}
			// once a hop drops the body, it is never sent again
			include_body = 0;
		}
		if (!include_body) {
			body = "";
			blen = 0;
		}
		hl_free(&r->hdr);
		r->body.n = 0;
	}
	hl_free(&ihdr);
	free(method);
	free(raw);
	free(ihost);
	free(referer);
	free(last_loc);
	return r;
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
