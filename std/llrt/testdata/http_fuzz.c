// http_fuzz.c: a mutation fuzzer of the native std/http
// parsers (decision 0074), run by internal/llvm's TestHTTPFuzz with
// AddressSanitizer and UndefinedBehaviorSanitizer for a bounded time.
// Each input goes through:
//
//   - the server's connection code (xo_hc_read and xo_hc_respond on a
//     connection without a socket): request line, header, target and
//     query, Content-Length and chunked bodies, trailers, responses (the
//     writer std/native/llrt/http1w.xo, linked from the LLVM IR of a
//     std/http program, cOnlyHTTP);
//   - HTTP/2 (xo_h2.c, decision 0121): mutated frame sequences through the
//     server's frame code (preface, frames, HPACK, streams, the handlers'
//     whole and streamed responses and resets, the sending of it all)
//     and the client's (h2cc_process).
//
// No crash, no sanitizer report, no leak of the runtime's allocator (the
// leak counter is checked after each input). Prints the number of inputs
// run. FUZZ_SECONDS (default 3) bounds the time, FUZZ_SEED the inputs.

#define XO_RT_NO_MAIN
#include "../xo_rt.c"
#include "http_ref.c"

// read_req is xo_hc_read with the C request parser (hp_head, hb_chunked,
// http_ref.c's test copies) in the part std/native/llrt/http1.xo plays in
// a program: decision 0198 deleted the server's own C path, so
// xo_hc_read returns 3 for a head and 4 for a chunked body to parse.
static int64_t read_req(xo_hconn *hc) {
	for (;;) {
		int64_t r = xo_hc_read(hc);
		if (r == 3) {
			int64_t used = 0;
			hc->rd_res = hp_head(hc->buf ? hc->buf + hc->off : hc->buf, hc->n - hc->off, hc->eof, &hc->req, &used);
			hc->rd_used = used;
			hc->rd_have = 1;
		} else if (r == 4) {
			int64_t used = 0;
			hc->rd_res = hb_chunked(hc->buf + hc->off, hc->n - hc->off, hc->eof, &hc->body, &used);
			hc->rd_used = used;
			hc->rd_have = 1;
		} else {
			return r;
		}
	}
}

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
	for (int k = 0; k < 64 && read_req(&hc) == 1; k++) {
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

// HTTP/2 (decision 0121): frame sequences for the server's frame code
// (h2_input, the handlers' responses, h2_schedule) and the client's
// (h2cc_process), without sockets.

static void fz_frame(nbuf *b, int type, int flags, uint32_t sid, const char *p, int64_t n) {
	h2_fh(b, (uint32_t)n, type, flags, sid);
	nbuf_put(b, p, n);
}

// h2_seed makes HTTP/2 input number k: a client's frames after its
// preface (k even) or a server's (k odd).
static void h2_seed(nbuf *b, int k) {
	// :method GET, :path /, :scheme https, :authority www.example.com
	// (Huffman, RFC 7541 C.4.1)
	static const char get[] = "\x82\x84\x87\x41\x8c\xf1\xe3\xc2\xe5\xf2\x3a\x6b\xa0\xab\x90\xf4\xff";
	// :method POST, :path /echo, :scheme https, then a field indexed
	// dynamically, a never indexed one, and a table size update first
	static const char post[] = "\x3f\xe1\x1f\x83\x04\x05/echo\x87\x40\x01x\x01y\xbe\x10\x01z\x02zz";
	static const char settings[] = "\x00\x04\x00\x00\x40\x00\x00\x01\x00\x00\x10\x00";
	switch (k % 6) {
	case 0:
		fz_frame(b, H2_SETTINGS, 0, 0, settings, 12);
		fz_frame(b, H2_HEADERS, H2F_END_HEADERS | H2F_END_STREAM, 1, get, sizeof get - 1);
		fz_frame(b, H2_HEADERS, H2F_END_HEADERS | H2F_END_STREAM, 3, "\x82\x84\x87\xbe", 4);
		break;
	case 2:
		fz_frame(b, H2_SETTINGS, 0, 0, "", 0);
		fz_frame(b, H2_HEADERS, 0, 1, post, 10);
		fz_frame(b, H2_CONT, H2F_END_HEADERS, 1, post + 10, sizeof post - 11);
		fz_frame(b, H2_DATA, 0, 1, "hello", 5);
		fz_frame(b, H2_DATA, H2F_PADDED, 1, "\x02 world\x00\x00", 9);
		fz_frame(b, H2_HEADERS, H2F_END_HEADERS | H2F_END_STREAM, 1, "\x40\x01t\x01v", 5);
		fz_frame(b, H2_PING, 0, 0, "12345678", 8);
		fz_frame(b, H2_WINUP, 0, 0, "\x00\x00\x10\x00", 4);
		break;
	case 4:
		fz_frame(b, H2_SETTINGS, 0, 0, settings, 6);
		fz_frame(b, H2_HEADERS, H2F_END_HEADERS | H2F_PRIORITY, 1, "\x00\x00\x00\x00\x10\x83\x84\x87\x41\x01h", 11);
		fz_frame(b, H2_PRIORITY, 0, 3, "\x00\x00\x00\x01\x10", 5);
		fz_frame(b, H2_RST, 0, 1, "\x00\x00\x00\x08", 4);
		fz_frame(b, H2_HEADERS, H2F_END_HEADERS | H2F_END_STREAM, 5, get, sizeof get - 1);
		fz_frame(b, H2_SETTINGS, H2F_ACK, 0, "", 0);
		fz_frame(b, H2_GOAWAY, 0, 0, "\x00\x00\x00\x05\x00\x00\x00\x00", 8);
		break;
	case 1:
		fz_frame(b, H2_SETTINGS, 0, 0, settings, 12);
		fz_frame(b, H2_HEADERS, H2F_END_HEADERS, 1, "\x88\x5f\x87\x49\x7c\xa5\x8a\xe8\x19\xaa", 10);
		fz_frame(b, H2_DATA, 0, 1, "body", 4);
		fz_frame(b, H2_DATA, H2F_END_STREAM, 1, "", 0);
		break;
	case 3:
		fz_frame(b, H2_HEADERS, H2F_END_HEADERS, 1, "\x08\x03" "100", 5);
		fz_frame(b, H2_HEADERS, 0, 1, "\x48\x03" "404", 5);
		fz_frame(b, H2_CONT, H2F_END_HEADERS, 1, "\x40\x01" "a\x01" "b", 5);
		fz_frame(b, H2_WINUP, 0, 1, "\x00\x00\x00\x10", 4);
		fz_frame(b, H2_DATA, H2F_PADDED | H2F_END_STREAM, 1, "\x01xy\x00", 4);
		break;
	default:
		fz_frame(b, H2_PING, 0, 0, "12345678", 8);
		fz_frame(b, H2_GOAWAY, 0, 0, "\x00\x00\x00\x00\x00\x00\x00\x02why", 11);
		fz_frame(b, H2_RST, 0, 1, "\x00\x00\x00\x02", 4);
		break;
	}
}

static int64_t h2_answered; // requests that reached a handler

// run_h2_server feeds input to a server connection, answers the requests
// it completes (whole and streamed bodies, faults), and sends.
static void run_h2_server(const char *in, int64_t n) {
	xo_hconn hc;
	memset(&hc, 0, sizeof hc);
	h2c *c = h2c_new(&hc);
	int64_t off = 0;
	pthread_mutex_lock(&c->mu);
	(void)h2_input(c, in, n, &off);
	for (int k = 0; k < 64; k++) {
		xo_hconn *t = h2_take_locked(c);
		if (!t) {
			break;
		}
		pthread_mutex_unlock(&c->mu);
		h2_answered++;
		int64_t l;
		xo_rstr *s = xo_hc_host(t, &l);
		xo_free(s);
		(void)xo_hc_port(t);
		for (int64_t i = 0; i < t->req.hdr.n; i++) {
			xo_hc_header(t, t->req.hdr.a[i].k.p, t->req.hdr.a[i].k.n, t->req.hdr.a[i].v.p, t->req.hdr.a[i].v.n);
		}
		int64_t status = 100 + (int64_t)(rnd() % 500);
		switch (rnd() % 3) {
		case 0:
			(void)xo_hc_respond(t, status, t->body.p ? t->body.p : "", t->body.n);
			break;
		case 1: {
			xo_sink *sk = xo_sink_conn(t, status);
			(void)xo_sink_write(sk, "streamed ", 9);
			(void)xo_sink_flush(sk);
			(void)xo_sink_write(sk, t->body.p ? t->body.p : "", t->body.n);
			(void)xo_sink_end(sk);
			h_release(sk);
			break;
		}
		default:
			break; // a fault: the stream is reset
		}
		xo_h2_end(t);
		h_release(t);
		pthread_mutex_lock(&c->mu);
	}
	h2_schedule(c);
	pthread_mutex_unlock(&c->mu);
	h2c_free(c);
}

// run_h2_client feeds a server's frames to the client's frame code.
static void run_h2_client(const char *in, int64_t n) {
	h2cc pc;
	memset(&pc, 0, sizeof pc);
	hp_dec_init(&pc.dec, H2_TABLE);
	hp_enc_init(&pc.enc);
	pc.peer_frame = 16384;
	pc.peer_win = pc.win = 65535;
	pc.in.avail = (int64_t)1 << 30;
	pc.c = (ccon){.fd = -1};
	pc.c.buf = malloc((size_t)n + 1);
	memcpy(pc.c.buf, in, (size_t)n);
	pc.c.n = pc.c.cap = n;
	pc.c.eof = 1;
	h2rq q;
	memset(&q, 0, sizeof q);
	q.id = 1;
	q.swin = 65535;
	q.sin.avail = (int64_t)4 << 20;
	xo_cresp r;
	memset(&r, 0, sizeof r);
	char why[300];
	const char *method = rnd() % 4 == 0 ? "HEAD" : "GET";
	for (;;) {
		const uint8_t *h;
		if (h2cc_frame(&pc.c, &h) != CC_OK || !h2cc_process(&pc, &q, h, &r, method, why, sizeof why) || q.ended) {
			break;
		}
	}
	free(pc.c.buf);
	hp_dec_free(&pc.dec);
	hp_tab_free(&pc.enc.t);
	free(pc.out.p);
	free(pc.goaway_debug);
	hl_free(&q.fields);
	free(q.hb.p);
	hl_free(&r.hdr);
	free(r.body.p);
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
			free(b.p);
			// HTTP/2 frames
			nbuf h = {0};
			int hk = (int)(rnd() % 6);
			h2_seed(&h, hk);
			rounds = 1 + (int)(rnd() % 4);
			for (int j = 0; j < rounds; j++) {
				mutate(&h);
			}
			if (hk % 2 == 1) {
				run_h2_client(h.p ? h.p : "", h.n);
			} else {
				// the client preface stays whole (mostly)
				nbuf in = {0};
				nbuf_str(&in, rnd() % 16 == 0 ? "PRI * HTTP/2.0\r\n\r\nSX\r\n\r\n" : H2_PREFACE);
				nbuf_put(&in, h.p, h.n);
				run_h2_server(in.p, in.n);
				free(in.p);
			}
			free(h.p);
			runs++;
		}
		int64_t live, total;
		xo_rt_live(&live, &total);
		if (live != live0) {
			fprintf(stderr, "leak: %lld runtime allocations live\n", (long long)(live - live0));
			return 1;
		}
	}
	printf("runs %lld, HTTP/2 requests answered %lld\n", (long long)runs, (long long)h2_answered);
	return 0;
}
