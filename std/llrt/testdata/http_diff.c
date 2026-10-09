// http_diff.c: the native HTTP/1.1 server's reading and writing
// (xo_http.c) on connections given as inputs, for internal/llvm's
// TestHTTPParseDiff, which sends the same bytes to a Go net/http server
// with the same handler and compares the answers byte for byte (Date
// values aside).
//
// Input: records of an 8 byte little endian length and the bytes a client
// sends on one connection (then it closes its side). Output: for each, an
// 8 byte length and the bytes the server answers, as conn.serve with
// std/rt's handler would.
//
// The handler (the same in the Go test): the body describes the request
// as std/rt's serve hands it to Xo code; the status is <n> for a path
// /status/<n>; the query parameters ct, cl, conn, te set the header fields
// Content-Type, Content-Length, Connection, Transfer-Encoding; big=<n>
// appends n bytes "x" to the body and empty=1 leaves it empty.

#define XO_RT_NO_MAIN
#include "../xo_rt.c"

static void hexs(nbuf *b, const char *p, int64_t n) {
	for (int64_t i = 0; i < n; i++) {
		char h[3];
		snprintf(h, sizeof h, "%02x", (unsigned char)p[i]);
		nbuf_put(b, h, 2);
	}
}

static hstr *qget(hreq *r, const char *k) {
	for (int64_t i = 0; i < r->query.n; i++) {
		if (hs_eq(r->query.a[i].k, k)) {
			return &r->query.a[i].v;
		}
	}
	return NULL;
}

// serve answers the requests of one connection's input with the
// server's own reading and writing (xo_hc_read, xo_hc_respond on a
// connection without a socket).
static void serve(const char *in, int64_t n, nbuf *out) {
	xo_hconn hc;
	memset(&hc, 0, sizeof hc);
	hc.buf = malloc((size_t)n + 1);
	memcpy(hc.buf, in, (size_t)n);
	hc.n = hc.cap = n;
	hc.eof = 1;
	hc.sink = out;
	while (xo_hc_read(&hc) == 1) {
		hreq *r = &hc.req;
		// the description std/rt's handler builds
		nbuf d = {0};
		nbuf_str(&d, "method=");
		nbuf_put(&d, r->method.p, r->method.n);
		nbuf_str(&d, "\npath=");
		nbuf_put(&d, r->url.path.p, r->url.path.n);
		nbuf_str(&d, "\nurl=");
		nbuf_put(&d, r->url.scheme.p, r->url.scheme.n);
		nbuf_byte(&d, '|');
		int64_t hl;
		xo_rstr *hs = xo_hc_host(&hc, &hl);
		nbuf_put(&d, hs->data, hl);
		xo_free(hs);
		nbuf_byte(&d, '|');
		int64_t port = xo_hc_port(&hc);
		if (port >= 0) {
			char pb[32];
			snprintf(pb, sizeof pb, "%lld", (long long)port);
			nbuf_str(&d, pb);
		} else {
			nbuf_byte(&d, '-');
		}
		nbuf_byte(&d, '|');
		if (r->url.path.n == 0) {
			nbuf_byte(&d, '/');
		} else {
			nbuf_put(&d, r->url.path.p, r->url.path.n);
		}
		nbuf_byte(&d, '|');
		for (int64_t i = 0; i < r->query.n; i++) {
			if (i > 0) {
				nbuf_byte(&d, '&');
			}
			nbuf_put(&d, r->query.a[i].k.p, r->query.a[i].k.n);
			nbuf_byte(&d, '=');
			nbuf_put(&d, r->query.a[i].v.p, r->query.a[i].v.n);
		}
		nbuf_str(&d, "\nheaders=");
		hpair *s = malloc((size_t)(r->hdr.n ? r->hdr.n : 1) * sizeof(hpair));
		if (r->hdr.n) {
			memcpy(s, r->hdr.a, (size_t)r->hdr.n * sizeof(hpair));
		}
		qsort(s, (size_t)r->hdr.n, sizeof(hpair), hp_cmp);
		for (int64_t i = 0; i < r->hdr.n; i++) {
			if (i > 0) {
				nbuf_byte(&d, '|');
			}
			nbuf_put(&d, s[i].k.p, s[i].k.n);
			nbuf_str(&d, ": ");
			nbuf_put(&d, s[i].v.p, s[i].v.n);
		}
		free(s);
		nbuf_str(&d, "\nbody=");
		hexs(&d, hc.body.p ? hc.body.p : "", hc.body.n);
		nbuf_byte(&d, '\n');
		int64_t status = 200;
		if (r->url.path.n > 8 && memcmp(r->url.path.p, "/status/", 8) == 0) {
			int64_t v;
			if (n_parse_int(r->url.path.p + 8, r->url.path.n - 8, &v)) {
				status = v;
			}
		}
		hstr *big = qget(r, "big");
		if (big) {
			int64_t k = 0;
			if (!n_parse_int(big->p, big->n, &k)) {
				k = 0;
			}
			if (k > (1 << 20)) {
				k = 1 << 20; // as the Go handler caps it
			}
			for (int64_t i = 0; i < k; i++) {
				nbuf_byte(&d, 'x');
			}
		}
		if (qget(r, "empty")) {
			d.n = 0;
		}
		static const char *qs[] = {"ct", "cl", "conn", "te"};
		static const char *names[] = {"Content-Type", "Content-Length", "Connection", "Transfer-Encoding"};
		for (int k = 0; k < 4; k++) {
			hstr *v = qget(r, qs[k]);
			if (v) {
				xo_hc_header(&hc, names[k], (int64_t)strlen(names[k]), v->p, v->n);
			}
		}
		int64_t keep = xo_hc_respond(&hc, status, d.p ? d.p : "", d.n);
		free(d.p);
		if (!keep) {
			break;
		}
	}
	if (hc.have_req) {
		hr_free(&hc.req);
	}
	free(hc.buf);
	free(hc.body.p);
	hl_free(&hc.resp);
}

int main(void) {
	for (;;) {
		unsigned char lb[8];
		if (fread(lb, 1, 8, stdin) != 8) {
			break;
		}
		int64_t n = 0;
		for (int k = 7; k >= 0; k--) {
			n = n * 256 + lb[k];
		}
		char *in = malloc((size_t)n + 1);
		if (n > 0 && fread(in, 1, (size_t)n, stdin) != (size_t)n) {
			return 2;
		}
		nbuf out = {0};
		serve(in, n, &out);
		unsigned char ob[8];
		for (int k = 0; k < 8; k++) {
			ob[k] = (unsigned char)((uint64_t)out.n >> (8 * k));
		}
		fwrite(ob, 1, 8, stdout);
		fwrite(out.p ? out.p : "", 1, (size_t)out.n, stdout);
		if (getenv("FLUSH")) {
			// one record at a time (FuzzHTTPDiff)
			fflush(stdout);
		}
		free(out.p);
		free(in);
	}
	return 0;
}
