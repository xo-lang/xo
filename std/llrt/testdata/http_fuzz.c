// http_fuzz.c: a mutation fuzzer of the native std/http and std/json
// parsers (decision 0074), run by internal/llvm's TestHTTPFuzz with
// AddressSanitizer and UndefinedBehaviorSanitizer for a bounded time.
// Each input goes through:
//
//   - the server's connection code (xo_hc_read and xo_hc_respond on a
//     connection without a socket): request line, header, target and
//     query, Content-Length and chunked bodies, trailers, responses;
//   - the client's response reader (cl_read_response);
//   - the JSON parser and the tape readers (xo_json_parse, members,
//     strings, numbers).
//
// No crash, no sanitizer report, no leak of the runtime's allocator (the
// leak counter is checked after each input). Prints the number of inputs
// run. FUZZ_SECONDS (default 3) bounds the time, FUZZ_SEED the inputs.

#define XO_RT_NO_MAIN
#include "../xo_rt.c"

static const char *seeds[] = {
	"GET / HTTP/1.1\r\nHost: x\r\n\r\n",
	"POST /p?a=1&b=%20&c HTTP/1.1\r\nHost: x\r\nContent-Length: 5\r\n\r\nhello",
	"POST /p HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\n0\r\nT: 1\r\n\r\n",
	"GET /a HTTP/1.0\r\nConnection: keep-alive\r\n\r\nGET /b HTTP/1.1\r\nHost: y\r\n\r\n",
	"GET http://h:80/x?y HTTP/1.1\r\nHost: z\r\nX: a\r\n b\r\n\r\n",
	"OPTIONS * HTTP/1.1\r\nHost: x\r\nExpect: 100-continue\r\nContent-Length: 2\r\n\r\nab",
	"HTTP/1.1 200 OK\r\nContent-Length: 3\r\n\r\nabc",
	"HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n3\r\nabc\r\n0\r\n\r\n",
	"HTTP/1.1 100 Continue\r\n\r\nHTTP/1.0 204 No Content\r\nX-A: 1\r\nX-A: 2\r\n\r\n",
	"{\"a\": [1, 2.5e3, -0, true, false, null, \"s\\u00e9\\ud83d\\ude00\"], \"b\": {\"c\": {}}, \"a\": 3}",
	"[\"\\\"\", 12345678901234567890, 1e400, {\"k\\n\": [[[]]]}]",
};

static const char *dict[] = {"\r\n", "\n", " ", ":", "Content-Length: ", "Transfer-Encoding: chunked\r\n", "0\r\n\r\n", "Host: h\r\n",
                             "HTTP/1.1", "HTTP/1.0", "%", "%2", "?", "&", "=", ";", "Expect: 100-continue\r\n", "Connection: close\r\n",
                             "{", "}", "[", "]", "\"", "\\u", ",", "-1", "18446744073709551616", "ffffffffffffffffff\r\n", "\x00", "\xff"};

static uint64_t rng;

static uint64_t rnd(void) {
	rng ^= rng << 13;
	rng ^= rng >> 7;
	rng ^= rng << 17;
	return rng;
}

static void mutate(nbuf *b) {
	int n = 1 + (int)(rnd() % 6);
	for (int k = 0; k < n; k++) {
		int64_t at = b->n ? (int64_t)(rnd() % (uint64_t)(b->n + 1)) : 0;
		switch (rnd() % 7) {
		case 0: // delete a range
			if (b->n > 0) {
				int64_t len = 1 + (int64_t)(rnd() % 8);
				if (at >= b->n) {
					at = b->n - 1;
				}
				if (at + len > b->n) {
					len = b->n - at;
				}
				memmove(b->p + at, b->p + at + len, (size_t)(b->n - at - len));
				b->n -= len;
			}
			break;
		case 1: { // insert a dictionary token
			const char *t = dict[rnd() % (sizeof dict / sizeof dict[0])];
			int64_t tl = t[0] ? (int64_t)strlen(t) : 1;
			nbuf_grow(b, tl);
			memmove(b->p + at + tl, b->p + at, (size_t)(b->n - at));
			memcpy(b->p + at, t, (size_t)tl);
			b->n += tl;
			break;
		}
		case 2: // flip a byte
			if (b->n > 0) {
				b->p[at % b->n] ^= (char)(1u << (rnd() % 8));
			}
			break;
		case 3: // a random byte
			if (b->n > 0) {
				b->p[at % b->n] = (char)rnd();
			}
			break;
		case 4: // truncate
			b->n = at;
			break;
		case 5: { // duplicate a range
			if (b->n > 0) {
				int64_t from = (int64_t)(rnd() % (uint64_t)b->n);
				int64_t len = 1 + (int64_t)(rnd() % 32);
				if (from + len > b->n) {
					len = b->n - from;
				}
				nbuf_grow(b, len);
				char tmp[32];
				memcpy(tmp, b->p + from, (size_t)len);
				memmove(b->p + at + len, b->p + at, (size_t)(b->n - at));
				memcpy(b->p + at, tmp, (size_t)len);
				b->n += len;
			}
			break;
		}
		case 6: { // splice another seed
			const char *s = seeds[rnd() % (sizeof seeds / sizeof seeds[0])];
			int64_t sl = (int64_t)strlen(s);
			int64_t from = (int64_t)(rnd() % (uint64_t)(sl + 1));
			nbuf_put(b, s + from, sl - from);
			break;
		}
		}
	}
}

static void run_server(const char *in, int64_t n) {
	xo_hconn hc;
	memset(&hc, 0, sizeof hc);
	hc.buf = malloc((size_t)n + 1);
	memcpy(hc.buf, in, (size_t)n);
	hc.n = hc.cap = n;
	hc.eof = 1;
	nbuf out = {0};
	hc.sink = &out;
	for (int k = 0; k < 64 && xo_hc_read(&hc) == 1; k++) {
		hreq *r = &hc.req;
		int64_t l;
		xo_rstr *s = xo_hc_host(&hc, &l);
		xo_free(s);
		(void)xo_hc_port(&hc);
		for (int64_t i = 0; i < r->hdr.n; i++) {
			xo_hc_header(&hc, r->hdr.a[i].k.p, r->hdr.a[i].k.n, r->hdr.a[i].v.p, r->hdr.a[i].v.n);
		}
		int64_t status = 100 + (int64_t)(rnd() % 500);
		if (!xo_hc_respond(&hc, status, hc.body.p ? hc.body.p : "", hc.body.n)) {
			break;
		}
	}
	if (hc.have_req) {
		hr_free(&hc.req);
	}
	free(hc.buf);
	free(hc.body.p);
	hl_free(&hc.resp);
	free(out.p);
}

static void run_client(const char *in, int64_t n) {
	ccon c = {.fd = -1};
	c.buf = malloc((size_t)n + 1);
	memcpy(c.buf, in, (size_t)n);
	c.n = c.cap = n;
	xo_cresp r;
	memset(&r, 0, sizeof r);
	char why[300];
	cl_read_response(&c, rnd() % 4 == 0 ? "HEAD" : "GET", &r, why, sizeof why);
	free(c.buf);
	hl_free(&r.hdr);
	free(r.body.p);
}

static void run_json(const char *in, int64_t n) {
	xo_rlist *t = xo_json_parse(in, n);
	if (t->len == 0) {
		int64_t l;
		xo_rstr *e = xo_json_parse_err(in, n, &l);
		xo_free(e);
		xo_free(t);
		return;
	}
	int64_t *tp = (int64_t *)rl_data(t);
	for (int64_t i = 0; i < t->len; i += 3) {
		int64_t l;
		switch (tp[i]) {
		case JK_STR: {
			xo_rstr *s = xo_json_str(in, n, t, i, &l);
			xo_free(s);
			break;
		}
		case JK_NUM:
			(void)xo_json_int_ok(in, n, t, i);
			(void)xo_json_float(in, n, t, i);
			break;
		case JK_OBJ: {
			xo_rlist *m = xo_json_members(in, n, t, i);
			xo_free(m);
			(void)xo_json_find(in, n, t, i, "a", 1);
			break;
		}
		}
	}
	xo_free(t);
}

int main(void) {
	const char *sec = getenv("FUZZ_SECONDS");
	const char *seed = getenv("FUZZ_SEED");
	double limit = sec ? atof(sec) : 3;
	rng = seed ? (uint64_t)atoll(seed) : 88172645463325252ULL;
	if (rng == 0) {
		rng = 1;
	}
	int64_t live0, total0;
	xo_rt_live(&live0, &total0);
	int64_t start = mono_ns(), runs = 0;
	while ((double)(mono_ns() - start) / 1e9 < limit) {
		for (int k = 0; k < 64; k++) {
			nbuf b = {0};
			const char *s = seeds[rnd() % (sizeof seeds / sizeof seeds[0])];
			nbuf_str(&b, s);
			int rounds = 1 + (int)(rnd() % 4);
			for (int j = 0; j < rounds; j++) {
				mutate(&b);
			}
			run_server(b.p ? b.p : "", b.n);
			run_client(b.p ? b.p : "", b.n);
			run_json(b.p ? b.p : "", b.n);
			free(b.p);
			runs++;
		}
		int64_t live, total;
		xo_rt_live(&live, &total);
		if (live != live0) {
			fprintf(stderr, "leak: %lld runtime allocations live\n", (long long)(live - live0));
			return 1;
		}
	}
	printf("runs %lld\n", (long long)runs);
	return 0;
}
