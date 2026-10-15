// xo_h2.c: HTTP/2 for the native std/http (RFC 9113, HPACK RFC 7541;
// decision 0121), included by xo_http.c before xo_tls.c.
//
// HTTP/2 is spoken over TLS when ALPN agrees on "h2", by the server
// (Server.with_tls) and the client (https URLs); cleartext stays HTTP/1.1
// (no h2c), as net/http behaves with std/rt's settings. What a peer
// observes copies net/http's bundled HTTP/2 (Go 1.26, h2_bundle.go):
//
//   - the server: its SETTINGS (MAX_FRAME_SIZE 1 MiB, 250 concurrent
//     streams, MAX_HEADER_LIST_SIZE 1048896, a 4096 byte HPACK table,
//     1 MiB stream and connection windows), the request checks and their
//     stream and connection errors, 431 for a header list over the limit
//     and 400 for connection specific fields, responses as
//     responseWriterState.writeChunk sends them (Content-Length,
//     sniffed Content-Type, Date, the handler's fields sorted), RST_STREAM
//     NO_ERROR after a response whose request body is still open, GOAWAY
//     NO_ERROR when serve stops (streams in flight finish), and the rapid
//     reset limit of scheduleHandler (at most 250 handlers run, 1000 more
//     wait, beyond that GOAWAY ENHANCE_YOUR_CALM);
//   - the client: connections kept for the next request (Transport's
//     pool), the header fields of httpcommon.EncodeHeaders
//     (User-Agent: Go-http-client/2.0, Accept-Encoding: gzip,
//     Content-Length; listed and encoded by Xo, std/native/llrt/
//     http1.xo h2_exchange), flow control both ways, and Go's error texts
//     ("stream error: stream ID 1; INTERNAL_ERROR; received from peer").
//
// HPACK is Xo's (std/native/llrt/hpack.xo): the server's since decision
// 0193 step 6, the client's since zero-C audit item B11 (2026-10-14).
//
// A connection's frames are read and written by one fiber, the
// connection's task (h2_next): it decodes requests and hands each
// complete one (its body read, as std/rt reads it before the handler) to
// std/native/http.xo, which runs it as a task of its own. A stream task
// queues its response on the stream (h2s, under the connection's lock)
// and wakes the connection's fiber, which encodes and sends it within the
// peer's flow control windows. The frame code works on bytes only
// (h2_input, h2_schedule), so http_fuzz.c runs it without a socket.

#define H2_PREFACE "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"

enum { H2_DATA, H2_HEADERS, H2_PRIORITY, H2_RST, H2_SETTINGS, H2_PUSH, H2_PING, H2_GOAWAY, H2_WINUP, H2_CONT };

#define H2F_END_STREAM 0x1
#define H2F_ACK 0x1
#define H2F_END_HEADERS 0x4
#define H2F_PADDED 0x8
#define H2F_PRIORITY 0x20

enum {
	H2E_NO,
	H2E_PROTOCOL,
	H2E_INTERNAL,
	H2E_FLOW,
	H2E_SETTINGS_TIMEOUT,
	H2E_STREAM_CLOSED,
	H2E_FRAME_SIZE,
	H2E_REFUSED,
	H2E_CANCEL,
	H2E_COMPRESSION,
	H2E_CONNECT,
	H2E_CALM,
	H2E_INADEQUATE,
	H2E_HTTP11,
};

enum { H2S_HEADER_TABLE = 1, H2S_ENABLE_PUSH, H2S_MAX_STREAMS, H2S_INITIAL_WINDOW, H2S_MAX_FRAME, H2S_MAX_HLIST, H2S_CONNECT = 8 };

// The server's limits: net/http's defaults (http2configFromServer).
#define H2_MAX_STREAMS 250
#define H2_MAX_FRAME (1 << 20)
#define H2_MAX_HLIST (1048576 + 320) // DefaultMaxHeaderBytes + 10 * 32
#define H2_WINDOW (1 << 20)          // per stream and per connection
#define H2_TABLE 4096
// handlerChunkWriteSize: the buffer of a streamed body
#define H2_CHUNK 4096
// a stream task waits while more than this of its body is unsent
#define H2_BACKLOG ((int64_t)256 << 10)

// h2_err_name is http2ErrCode.String.
static const char *h2_err_name(uint32_t c, char *buf, size_t bn) {
	static const char *names[] = {"NO_ERROR",  "PROTOCOL_ERROR",    "INTERNAL_ERROR",    "FLOW_CONTROL_ERROR", "SETTINGS_TIMEOUT",
	                              "STREAM_CLOSED", "FRAME_SIZE_ERROR", "REFUSED_STREAM", "CANCEL",             "COMPRESSION_ERROR",
	                              "CONNECT_ERROR", "ENHANCE_YOUR_CALM", "INADEQUATE_SECURITY", "HTTP_1_1_REQUIRED"};
	if (c < sizeof names / sizeof names[0]) {
		return names[c];
	}
	snprintf(buf, bn, "unknown error code 0x%x", (unsigned)c);
	return buf;
}

// ---------------------------------------------------------------------------
// Frames

static uint32_t h2_get32(const uint8_t *p) { return (uint32_t)p[0] << 24 | (uint32_t)p[1] << 16 | (uint32_t)p[2] << 8 | p[3]; }

// The frames are written by Xo (std/native/llrt/h2w.xo, the zero-C
// audit's B28) through the exports of cabi_http.xo, into room the C
// reserves; a program without std/http links the weak definitions,
// which fault if one ever ran (it never reaches HTTP/2).

__attribute__((weak)) int64_t xo_x_h2_fh(int64_t d, int64_t len, int64_t type, int64_t flags, int64_t sid) {
	(void)d, (void)len, (void)type, (void)flags, (void)sid;
	fault_text("HTTP without std/http's exports", "");
}

__attribute__((weak)) int64_t xo_x_h2_rst(int64_t d, int64_t sid, int64_t code) {
	(void)d, (void)sid, (void)code;
	fault_text("HTTP without std/http's exports", "");
}

__attribute__((weak)) int64_t xo_x_h2_winup(int64_t d, int64_t sid, int64_t inc) {
	(void)d, (void)sid, (void)inc;
	fault_text("HTTP without std/http's exports", "");
}

__attribute__((weak)) int64_t xo_x_h2_goaway(int64_t d, int64_t last, int64_t code, int64_t dp, int64_t dn) {
	(void)d, (void)last, (void)code, (void)dp, (void)dn;
	fault_text("HTTP without std/http's exports", "");
}

__attribute__((weak)) int64_t xo_x_h2_setting(int64_t d, int64_t id, int64_t v) {
	(void)d, (void)id, (void)v;
	fault_text("HTTP without std/http's exports", "");
}

__attribute__((weak)) int64_t xo_x_h2_block(int64_t d, int64_t sid, int64_t bp, int64_t bn, int64_t end_stream, int64_t max) {
	(void)d, (void)sid, (void)bp, (void)bn, (void)end_stream, (void)max;
	fault_text("HTTP without std/http's exports", "");
}

__attribute__((weak)) int64_t xo_x_h2_head_room(int64_t ha, int64_t hn) {
	(void)ha, (void)hn;
	fault_text("HTTP without std/http's exports", "");
}

__attribute__((weak)) int64_t xo_x_h2_head(int64_t d, int64_t ha, int64_t hn, int64_t status, int64_t head_req, int64_t p, int64_t pn, int64_t done, int64_t res) {
	(void)d, (void)ha, (void)hn, (void)status, (void)head_req, (void)p, (void)pn, (void)done, (void)res;
	fault_text("HTTP without std/http's exports", "");
}

// frame_check's results
enum { H2_FC_OK, H2_FC_IGNORE, H2_FC_CONN, H2_FC_STREAM };

__attribute__((weak)) int64_t xo_x_h2_frame_check(int64_t h, int64_t p, int64_t hb_on, int64_t hb_sid, int64_t res) {
	(void)h, (void)p, (void)hb_on, (void)hb_sid, (void)res;
	fault_text("HTTP without std/http's exports", "");
}

static void h2_fh(nbuf *o, uint32_t len, int type, int flags, uint32_t sid) {
	nbuf_grow(o, 9);
	o->n += xo_x_h2_fh(XO_ADDR(o->p + o->n), len, type, flags, sid);
}

static void h2_put_rst(nbuf *o, uint32_t sid, uint32_t code) {
	nbuf_grow(o, 13);
	o->n += xo_x_h2_rst(XO_ADDR(o->p + o->n), sid, code);
}

static void h2_put_winup(nbuf *o, uint32_t sid, uint32_t inc) {
	nbuf_grow(o, 13);
	o->n += xo_x_h2_winup(XO_ADDR(o->p + o->n), sid, inc);
}

static void h2_put_goaway(nbuf *o, uint32_t last, uint32_t code, const char *debug) {
	int64_t dn = debug ? (int64_t)strlen(debug) : 0;
	nbuf_grow(o, 17 + dn);
	o->n += xo_x_h2_goaway(XO_ADDR(o->p + o->n), last, code, XO_ADDR(debug), dn);
}

static void h2_put_setting(nbuf *o, int id, uint32_t v) {
	nbuf_grow(o, 6);
	o->n += xo_x_h2_setting(XO_ADDR(o->p + o->n), id, v);
}

// h2_put_block writes header block blk as a HEADERS frame and
// CONTINUATION frames of at most max bytes.
static void h2_put_block(nbuf *o, uint32_t sid, const char *blk, int64_t bn, int end_stream, uint32_t max) {
	nbuf_grow(o, bn + 9 * (bn / max + 1));
	o->n += xo_x_h2_block(XO_ADDR(o->p + o->n), sid, XO_ADDR(blk), bn, end_stream, max);
}

// A window of received bytes (http2inflow): updates are batched until
// 4 KiB are owed or the peer's window would run out.
typedef struct {
	int64_t avail, unsent;
} h2_inflow;

static int64_t h2_inflow_add(h2_inflow *f, int64_t n) {
	int64_t u = f->unsent + n;
	if (u < 4096 && u < f->avail) {
		f->unsent = u;
		return 0;
	}
	f->avail += u;
	f->unsent = 0;
	return u;
}

// h2_valid_name is validWireHeaderFieldName: token bytes, no upper case.
static int h2_valid_name(const char *n, int64_t nn) {
	if (nn == 0) {
		return 0;
	}
	for (int64_t i = 0; i < nn; i++) {
		unsigned c = (unsigned char)n[i];
		if (!h_tchar(c) || (c >= 'A' && c <= 'Z')) {
			return 0;
		}
	}
	return 1;
}

// h2_valid_value is httpguts.ValidHeaderFieldValue: no control bytes but
// tab.
static int h2_valid_value(const char *v, int64_t vn) {
	for (int64_t i = 0; i < vn; i++) {
		unsigned c = (unsigned char)v[i];
		if ((c < ' ' && c != '\t') || c == 0x7f) {
			return 0;
		}
	}
	return 1;
}

// ---------------------------------------------------------------------------
// The server
//
// A connection (h2c) belongs to its xo_hconn (hc->h2). A stream (h2s) is
// referenced by the connection's table while it is open, by the queue of
// requests waiting for a handler, by the list of streams with output to
// send, and by the xo_hconn of its request (hc->h2s) once dispatched; it
// is freed when no reference is left (refs, under the connection's lock).
// A stream's xo_hconn holds its connection's xo_hconn, so the connection
// outlives the streams handed to Xo.

enum { H2ST_OPEN, H2ST_HCR }; // the request is still arriving, it ended

enum { H2K_APP, H2K_431, H2K_400, H2K_OPTIONS };

// The header fields of a block, checked as Framer.readMetaFrame does.
typedef struct {
	hlist f;
	int64_t remain;
	int invalid, truncated, regular;
} h2_fields;

typedef struct h2s h2s;
typedef struct h2c h2c;

struct h2s {
	h2c *c;
	uint32_t id;
	int refs;
	int in_table;
	int state;
	// the request (the connection's fiber only, until dispatched)
	hreq req;
	nbuf body;
	int64_t declen; // its Content-Length, -1: none
	int64_t got;    // body bytes received
	int trailers;   // trailers were received
	int over;       // the body passed the maximum: the rest is dropped
	int completed;  // handed to a handler (or answered here)
	int kind;
	char *msg; // H2K_400: the error text
	int expect100;
	h2_inflow in;
	// the response (under c->mu)
	nbuf rh; // the head in the encoder job's form (h2_head), ready when rh_ready
	int rh_ready, rh_sent;
	int nobody; // HEAD or a status without a body: no DATA
	nbuf out;
	int64_t off; // out[off:] is not sent yet
	int end, end_sent;
	int64_t win; // the peer's window for the stream
	int rst;     // a RST_STREAM to send: its code plus one
	int rst_sent;
	int dead; // closed or reset: output is dropped
	int queued;
	h2s *rnext;
	xo_park *waiter; // the stream's task, waiting for its backlog to drain
	int dispatched, ended;
	int rst_late; // the reset follows the output queued before it (a handler's fault)
	// HPACK in Xo: the encoded header block of rh, from Xo
	nbuf hblk;
	int hblk_ok;
};

struct h2c {
	pthread_mutex_t mu;
	xo_hconn *hc;
	int npreface; // bytes of the client preface seen
	int got_settings;
	// a header block being received (HEADERS and CONTINUATION frames):
	// its fields so far, and the bytes of a field the last frame cut
	h2_fields hf;
	int hfirst;
	nbuf hb;
	uint32_t hb_sid, hb_dep;
	int hb_on, hb_flags, hb_prio;
	// the streams
	h2s **tab;
	int64_t ntab, ctab;
	uint32_t max_id; // the largest client stream id seen
	int64_t nopen;   // streams in tab
	int64_t nhandlers;
	h2s **dq; // complete requests waiting for a handler
	int64_t dqn, dqoff, dqcap;
	struct xo_hconn *take; // the request h2_next found
	// the peer's settings and windows
	uint32_t peer_frame;
	int64_t peer_win; // its initial stream window
	int64_t win;      // its connection window
	int unacked;      // SETTINGS of ours not acknowledged
	h2_inflow in;     // our connection window
	// output
	nbuf wb;
	h2s *rfirst, *rlast;
	xo_park *waiter; // the connection's fiber waiting (h2_wait)
	int woken;
	// shutdown
	int goaway;  // a GOAWAY was queued
	int failed;  // a connection error: GOAWAY with its code, then close
	int stop;    // the server stops: GOAWAY NO_ERROR, then close when idle
	int reject;  // close right after the GOAWAY (INADEQUATE_SECURITY)
	int dead;    // the connection is unusable
	int64_t deadline; // preface, first frame, or the linger after an error (monotonic)
	uint32_t cur_sid; // the stream of the frame being processed (0 while it is parsed)
	// HPACK in Xo (decision 0193 step 6, std/native/llrt/hpack.xo): the
	// fiber's Xo code decodes header blocks and encodes response heads;
	// C never calls Xo, so h2_next stops at them
	int phase;        // where h2_next resumes (H2P_*)
	int dec_pending;  // c->hb holds a fragment for Xo (h2_block)
	int pend_flags;   // the flags and stream of its frame
	uint32_t pend_sid;
	uint32_t want100; // the stream a 100 Continue goes to (h2_enc_job)
	uint32_t *encset; // the peer's table size settings Xo applies next
	int64_t nencset, cencset;
	h2s **encq; // the streams whose heads Xo encodes (the lock is held)
	int64_t nencq, cencq;
	// the header block's :path as Xo parsed it (h2_fields; url.
	// ParseRequestURI and the query, http1.xo): tgt_set once a fragment
	// of the block had one, tgt_ok when it parsed
	int tgt_set, tgt_ok;
	hstr tgt_raw;
	hurl tgt_url;
	hlist tgt_query;
};

enum { H2P_NONE, H2P_INPUT, H2P_ENCODED };

// h2_tgt_clear forgets the :path of the header block before.
static void h2_tgt_clear(h2c *c) {
	hs_free(&c->tgt_raw);
	hu_free(&c->tgt_url);
	hl_free(&c->tgt_query);
	c->tgt_set = 0;
	c->tgt_ok = 0;
}

static void h2_wake(h2c *c) {
	if (c->waiter) {
		park_signal(c->waiter);
		c->waiter = NULL;
	} else {
		c->woken = 1;
	}
}

static void h2s_free(h2s *st) {
	if (st->req.method.p || st->req.hdr.a || st->req.target.p) {
		hr_free(&st->req);
	}
	free(st->body.p);
	free(st->rh.p);
	free(st->out.p);
	free(st->hblk.p);
	free(st->msg);
	free(st);
}

static void h2s_unref(h2s *st) {
	if (--st->refs == 0) {
		h2s_free(st);
	}
}

static h2s *h2_find(h2c *c, uint32_t id) {
	for (int64_t i = 0; i < c->ntab; i++) {
		if (c->tab[i]->id == id) {
			return c->tab[i];
		}
	}
	return NULL;
}

// h2_ready queues st's output for the connection's fiber.
static void h2_ready(h2c *c, h2s *st) {
	if (!st->queued) {
		st->queued = 1;
		st->refs++;
		st->rnext = NULL;
		if (c->rlast) {
			c->rlast->rnext = st;
		} else {
			c->rfirst = st;
		}
		c->rlast = st;
	}
	h2_wake(c);
}

static void h2_signal_stream(h2s *st) {
	if (st->waiter) {
		park_signal(st->waiter);
		st->waiter = NULL;
	}
}

// h2_close_stream takes st out of the table (closed): nothing more is
// sent on it.
static void h2_close_stream(h2c *c, h2s *st) {
	st->dead = 1;
	h2_signal_stream(st);
	if (st->in_table) {
		st->in_table = 0;
		for (int64_t i = 0; i < c->ntab; i++) {
			if (c->tab[i] == st) {
				c->tab[i] = c->tab[--c->ntab];
				break;
			}
		}
		c->nopen--;
		h2s_unref(st);
	}
}

// h2_conn_err ends the connection with a GOAWAY of code (a connection
// error); nothing more is read or answered.
static void h2_conn_err(h2c *c, uint32_t code) {
	if (c->failed) {
		return;
	}
	// a frame that parsed counts in the GOAWAY's last stream id
	// (processFrameFromReader)
	if (c->cur_sid > c->max_id) {
		c->max_id = c->cur_sid;
	}
	c->failed = 1;
	c->goaway = 1;
	h2_put_goaway(&c->wb, c->max_id, code, NULL);
	// net/http closes a second after the GOAWAY (goAwayTimeout)
	c->deadline = deadline((int64_t)1000000000);
}

// h2_stream_err resets stream sid with code (a stream error).
static void h2_stream_err(h2c *c, uint32_t sid, uint32_t code) {
	h2s *st = h2_find(c, sid);
	if (!st) {
		h2_put_rst(&c->wb, sid, code);
		return;
	}
	if (!st->rst) {
		st->rst = (int)code + 1;
		st->dead = 1;
		h2_signal_stream(st);
		h2_ready(c, st);
	}
}


static void h2_collect(void *arg, const char *n, int64_t nn, const char *v, int64_t vn) {
	h2_fields *f = arg;
	if (f->invalid || f->truncated) {
		return;
	}
	if (!h2_valid_value(v, vn)) {
		f->invalid = 1;
		return;
	}
	if (nn > 0 && n[0] == ':') {
		if (f->regular) {
			f->invalid = 1;
			return;
		}
	} else {
		f->regular = 1;
		if (!h2_valid_name(n, nn)) {
			f->invalid = 1;
			return;
		}
	}
	int64_t size = nn + vn + 32;
	if (size > f->remain) {
		f->truncated = 1;
		f->remain = 0;
		return;
	}
	f->remain -= size;
	hl_add(&f->f, hs_dup(n, nn), hs_dup(v, vn));
}

// h2_pseudos_ok is MetaHeadersFrame.checkPseudos.
static int h2_pseudos_ok(hlist *f, int *response) {
	int req = 0, resp = 0;
	for (int64_t i = 0; i < f->n; i++) {
		hstr k = f->a[i].k;
		if (k.n == 0 || k.p[0] != ':') {
			continue;
		}
		if (hs_eq(k, ":method") || hs_eq(k, ":path") || hs_eq(k, ":scheme") || hs_eq(k, ":authority") || hs_eq(k, ":protocol")) {
			req = 1;
		} else if (hs_eq(k, ":status")) {
			resp = 1;
		} else {
			return 0;
		}
		for (int64_t j = 0; j < i; j++) {
			if (f->a[j].k.n == k.n && memcmp(f->a[j].k.p, k.p, (size_t)k.n) == 0) {
				return 0;
			}
		}
	}
	if (response) {
		*response = resp;
	}
	return !(req && resp);
}

static hstr h2_pseudo(hlist *f, const char *name) {
	for (int64_t i = 0; i < f->n; i++) {
		if (hs_eq(f->a[i].k, name)) {
			return f->a[i].v;
		}
	}
	return (hstr){NULL, 0};
}

static int h2_respond_internal(h2c *c, h2s *st, int64_t status, hlist *h, const char *body, int64_t bn);

static void h2_dispatch(h2c *c, h2s *st);

// h2_complete hands a request whose body arrived (or passed the maximum)
// to a handler, or answers it here.
static void h2_complete(h2c *c, h2s *st) {
	if (st->completed) {
		return;
	}
	st->completed = 1;
	if (st->kind == H2K_OPTIONS) {
		// globalOptionsHandler
		hlist h = {0};
		hl_add(&h, hs_dup("Content-Length", 14), hs_dup("0", 1));
		h2_respond_internal(c, st, 200, &h, "", 0);
		hl_free(&h);
		return;
	}
	if (st->kind != H2K_APP) {
		return; // answered when its header came
	}
	h2_dispatch(c, st);
}

// h2_dispatch queues st for a handler: at most H2_MAX_STREAMS run, and
// four times as many wait (scheduleHandler); more is a rapid reset
// attack, ENHANCE_YOUR_CALM.
static void h2_dispatch(h2c *c, h2s *st) {
	int64_t waiting = c->dqn - c->dqoff;
	int64_t free_slots = H2_MAX_STREAMS - c->nhandlers;
	if (free_slots < 0) {
		free_slots = 0;
	}
	if (waiting - free_slots > 4 * H2_MAX_STREAMS) {
		h2_conn_err(c, H2E_CALM);
		return;
	}
	if (c->dqn == c->dqcap) {
		if (c->dqoff > 0) {
			memmove(c->dq, c->dq + c->dqoff, sizeof(h2s *) * (size_t)(c->dqn - c->dqoff));
			c->dqn -= c->dqoff;
			c->dqoff = 0;
		}
		if (c->dqn == c->dqcap) {
			c->dqcap = c->dqcap ? c->dqcap * 2 : 16;
			c->dq = realloc(c->dq, sizeof(h2s *) * (size_t)c->dqcap);
			if (!c->dq) {
				xo_oom();
			}
		}
	}
	st->refs++;
	c->dq[c->dqn++] = st;
}

// h2_join_values appends v to the value of field k of h (", " between
// values; "; " for Cookie, as http2 joins its fields).
static void h2_add_field(hlist *h, const char *k, int64_t kn, const char *v, int64_t vn) {
	for (int64_t x = 0; x < h->n; x++) {
		if (h->a[x].k.n == kn && memcmp(h->a[x].k.p, k, (size_t)kn) == 0) {
			const char *sep = kn == 6 && memcmp(k, "Cookie", 6) == 0 ? "; " : ", ";
			hstr *ov = &h->a[x].v;
			char *nv = xo_alloc(ov->n + 2 + vn + 1);
			memcpy(nv, ov->p, (size_t)ov->n);
			memcpy(nv + ov->n, sep, 2);
			if (vn) {
				memcpy(nv + ov->n + 2, v, (size_t)vn);
			}
			nv[ov->n + 2 + vn] = 0;
			xo_free(ov->p);
			ov->p = nv;
			ov->n += 2 + vn;
			return;
		}
	}
	hl_add(h, hs_dup(k, kn), hs_dup(v, vn));
}

// h2_parse_uint63 is strconv.ParseUint(s, 10, 63).
static int h2_parse_uint63(const char *s, int64_t n, int64_t *out) {
	if (n == 0) {
		return 0;
	}
	uint64_t v = 0;
	for (int64_t i = 0; i < n; i++) {
		if (s[i] < '0' || s[i] > '9' || v > (((uint64_t)1 << 63) - 1 - (uint64_t)(s[i] - '0')) / 10) {
			return 0;
		}
		v = v * 10 + (uint64_t)(s[i] - '0');
	}
	*out = (int64_t)v;
	return 1;
}

// h2_request makes st's request from the fields f (processHeaders,
// newWriterAndRequest): 0 when it is malformed (a stream error).
static int h2_request(h2c *c, h2s *st, h2_fields *f, int end_stream) {
	hstr method = h2_pseudo(&f->f, ":method"), path = h2_pseudo(&f->f, ":path"), scheme = h2_pseudo(&f->f, ":scheme");
	hstr auth = h2_pseudo(&f->f, ":authority"), proto = h2_pseudo(&f->f, ":protocol");
	if (proto.p) {
		return 0; // extended CONNECT is off
	}
	int connect = method.p && hs_eq(method, "CONNECT");
	if (connect) {
		if (path.n || scheme.n || auth.n == 0) {
			return 0;
		}
	} else if (method.n == 0 || path.n == 0 || !(hs_eq(scheme, "https") || hs_eq(scheme, "http"))) {
		return 0;
	}
	hreq *r = &st->req;
	memset(r, 0, sizeof *r);
	// the header: canonical keys, values joined
	for (int64_t i = 0; i < f->f.n; i++) {
		hstr k = f->f.a[i].k;
		if (k.n > 0 && k.p[0] == ':') {
			continue;
		}
		char *ck = xo_alloc(k.n + 1);
		memcpy(ck, k.p, (size_t)k.n);
		ck[k.n] = 0;
		h_canon(ck, k.n);
		h2_add_field(&r->hdr, ck, k.n, f->f.a[i].v.p, f->f.a[i].v.n);
		xo_free(ck);
	}
	hstr host = auth;
	int64_t hi = hl_find(&r->hdr, "Host");
	if (host.n == 0 && hi >= 0) {
		host = r->hdr.a[hi].v;
	}
	// NewServerRequest: Expect: 100-continue is taken out, Trailer too
	int64_t ei = hl_find(&r->hdr, "Expect");
	if (ei >= 0 && h_has_token(r->hdr.a[ei].v.p, r->hdr.a[ei].v.n, "100-continue")) {
		st->expect100 = 1;
		hl_del(&r->hdr, "Expect");
	}
	hl_del(&r->hdr, "Trailer");
	if (memchr(auth.p ? auth.p : "", '@', (size_t)auth.n) && (hs_eq(scheme, "http") || hs_eq(scheme, "https"))) {
		return 0;
	}
	r->method = hs_dup(method.p, method.n);
	r->major = 2;
	r->host = hs_dup(host.p ? host.p : "", host.n);
	if (connect) {
		r->target = hs_dup(auth.p, auth.n);
		r->url.host = hs_dup(auth.p, auth.n);
	} else {
		r->target = hs_dup(path.p, path.n);
		// the URL and its query, parsed by Xo (h2_fields)
		if (!c->tgt_set || !c->tgt_ok || c->tgt_raw.n != path.n || memcmp(c->tgt_raw.p, path.p, (size_t)path.n) != 0) {
			return 0;
		}
		r->url = c->tgt_url;
		c->tgt_url = (hurl){0};
		r->query = c->tgt_query;
		c->tgt_query = (hlist){0};
	}
	st->declen = -1;
	if (!end_stream) {
		int64_t ci = hl_find(&r->hdr, "Content-Length");
		if (ci >= 0) {
			int64_t v;
			// the first value (a joined value does not parse: 0)
			st->declen = h2_parse_uint63(r->hdr.a[ci].v.p, r->hdr.a[ci].v.n, &v) ? v : 0;
		}
	}
	st->kind = H2K_APP;
	if (f->truncated) {
		st->kind = H2K_431;
	} else {
		// checkValidHTTP2RequestHeaders
		static const char *conn[] = {"Connection", "Keep-Alive", "Proxy-Connection", "Transfer-Encoding", "Upgrade"};
		for (int i = 0; i < 5 && st->kind == H2K_APP; i++) {
			if (hl_find(&r->hdr, conn[i]) >= 0) {
				char m[120];
				snprintf(m, sizeof m, "request header \"%s\" is not valid in HTTP/2", conn[i]);
				st->msg = strdup(m);
				st->kind = H2K_400;
			}
		}
		int tes = 0, te_ok = 1;
		for (int64_t i = 0; i < f->f.n; i++) {
			if (hs_eq(f->f.a[i].k, "te")) {
				tes++;
				te_ok = te_ok && (hs_eq(f->f.a[i].v, "trailers") || f->f.a[i].v.n == 0);
			}
		}
		if (st->kind == H2K_APP && tes > 0 && (tes > 1 || !te_ok)) {
			st->msg = strdup("request header \"TE\" may only be \"trailers\" in HTTP/2");
			st->kind = H2K_400;
		}
	}
	if (st->kind == H2K_APP && hs_eq(r->method, "OPTIONS") && hs_eq(r->target, "*")) {
		st->kind = H2K_OPTIONS;
	}
	return 1;
}

// h2_on_headers handles the fields of a complete header block of stream
// sid (taking them over).
static void h2_on_headers(h2c *c, uint32_t sid, int flags, int prio, uint32_t dep, h2_fields *fp) {
	h2_fields f = *fp;
	*fp = (h2_fields){0};
	if (c->goaway && (c->failed || sid > c->max_id)) {
		hl_free(&f.f);
		return;
	}
	int resp = 0;
	if (f.invalid || !h2_pseudos_ok(&f.f, &resp)) {
		hl_free(&f.f);
		h2_stream_err(c, sid, H2E_PROTOCOL);
		return;
	}
	if (sid % 2 != 1) {
		hl_free(&f.f);
		h2_conn_err(c, H2E_PROTOCOL);
		return;
	}
	int end_stream = flags & H2F_END_STREAM;
	h2s *st = h2_find(c, sid);
	if (st) {
		// trailers (processTrailerHeaders)
		int has_pseudo = f.f.n > 0 && f.f.a[0].k.n > 0 && f.f.a[0].k.p[0] == ':';
		hl_free(&f.f);
		if (st->rst) {
			return;
		}
		if (st->state == H2ST_HCR) {
			h2_stream_err(c, sid, H2E_STREAM_CLOSED);
			return;
		}
		if (st->trailers) {
			h2_conn_err(c, H2E_PROTOCOL);
			return;
		}
		st->trailers = 1;
		if (!end_stream || has_pseudo) {
			h2_stream_err(c, sid, H2E_PROTOCOL);
			return;
		}
		st->state = H2ST_HCR;
		h2_complete(c, st);
		return;
	}
	if (sid <= c->max_id) {
		hl_free(&f.f);
		h2_conn_err(c, H2E_PROTOCOL);
		return;
	}
	c->max_id = sid;
	if (c->nopen + 1 > H2_MAX_STREAMS) {
		hl_free(&f.f);
		h2_stream_err(c, sid, c->unacked ? H2E_REFUSED : H2E_PROTOCOL);
		return;
	}
	st = calloc(1, sizeof(h2s));
	if (!st) {
		xo_oom();
	}
	st->c = c;
	st->id = sid;
	st->refs = 1;
	st->in_table = 1;
	st->state = end_stream ? H2ST_HCR : H2ST_OPEN;
	st->win = c->peer_win;
	st->in.avail = H2_WINDOW;
	st->declen = -1;
	if (c->ntab == c->ctab) {
		c->ctab = c->ctab ? c->ctab * 2 : 16;
		c->tab = realloc(c->tab, sizeof(h2s *) * (size_t)c->ctab);
		if (!c->tab) {
			xo_oom();
		}
	}
	c->tab[c->ntab++] = st;
	c->nopen++;
	if (prio && dep == sid) {
		hl_free(&f.f);
		h2_stream_err(c, sid, H2E_PROTOCOL);
		return;
	}
	int ok = h2_request(c, st, &f, end_stream);
	hl_free(&f.f);
	if (!ok) {
		h2_stream_err(c, sid, H2E_PROTOCOL);
		return;
	}
	if (st->kind == H2K_431) {
		h2_respond_internal(c, st, 431, NULL, "<h1>HTTP Error 431</h1><p>Request Header Field(s) Too Large</p>", 63);
	} else if (st->kind == H2K_400) {
		// http.Error
		hlist h = {0};
		hl_add(&h, hs_dup("Content-Type", 12), hs_dup("text/plain; charset=utf-8", 25));
		hl_add(&h, hs_dup("X-Content-Type-Options", 22), hs_dup("nosniff", 7));
		nbuf b = {0};
		nbuf_str(&b, st->msg);
		nbuf_byte(&b, '\n');
		h2_respond_internal(c, st, 400, &h, b.p, b.n);
		free(b.p);
		hl_free(&h);
	} else if (!end_stream && st->expect100 && st->kind == H2K_APP) {
		// encoded by Xo before the next frame (h2_fields, h2_enc_job)
		c->want100 = sid;
	}
	if (end_stream) {
		h2_complete(c, st);
	}
}

static int64_t h2_max_body(void) { return http_max_body(); }

// h2_on_data handles a DATA frame of flen bytes carrying p[0:dn].
static void h2_on_data(h2c *c, uint32_t sid, int flags, int64_t flen, const uint8_t *p, int64_t dn) {
	h2s *st = h2_find(c, sid);
	if (!st && sid > c->max_id) {
		h2_conn_err(c, H2E_PROTOCOL);
		return;
	}
	if (!st || st->state != H2ST_OPEN || st->trailers || st->rst) {
		if (c->in.avail < flen) {
			h2_stream_err(c, sid, H2E_FLOW);
			return;
		}
		c->in.avail -= flen;
		int64_t u = h2_inflow_add(&c->in, flen);
		if (u > 0) {
			h2_put_winup(&c->wb, 0, (uint32_t)u);
		}
		if (st && st->rst) {
			return;
		}
		h2_stream_err(c, sid, H2E_STREAM_CLOSED);
		return;
	}
	if (st->declen != -1 && st->got + dn > st->declen) {
		if (c->in.avail < flen) {
			h2_stream_err(c, sid, H2E_FLOW);
			return;
		}
		c->in.avail -= flen;
		int64_t u = h2_inflow_add(&c->in, flen);
		if (u > 0) {
			h2_put_winup(&c->wb, 0, (uint32_t)u);
		}
		h2_stream_err(c, sid, H2E_PROTOCOL);
		return;
	}
	if (flen > 0) {
		if (c->in.avail < flen || st->in.avail < flen) {
			h2_stream_err(c, sid, H2E_FLOW);
			return;
		}
		c->in.avail -= flen;
		st->in.avail -= flen;
		st->got += dn;
		if (!st->over && st->kind == H2K_APP) {
			int64_t max = h2_max_body();
			int64_t k = dn;
			if (st->body.n + k > max) {
				k = max - st->body.n;
				st->over = 1;
			}
			nbuf_put(&st->body, (const char *)p, k);
		}
		// the body is read at once: its window comes back now (the
		// stream's while it is wanted)
		int64_t u = h2_inflow_add(&c->in, flen);
		if (u > 0) {
			h2_put_winup(&c->wb, 0, (uint32_t)u);
		}
		if (!(flags & H2F_END_STREAM) && !st->over) {
			u = h2_inflow_add(&st->in, flen);
			if (u > 0) {
				h2_put_winup(&c->wb, sid, (uint32_t)u);
			}
		}
		if (st->over) {
			h2_complete(c, st);
		}
	}
	if (flags & H2F_END_STREAM) {
		st->state = H2ST_HCR;
		h2_complete(c, st);
		if (st->end_sent && !st->rst) {
			// the response went out first (431, 400): the stream closes
			h2_close_stream(c, st);
		}
	}
}

static void h2_enqueue_pending(h2c *c) {
	for (int64_t i = 0; i < c->ntab; i++) {
		h2s *st = c->tab[i];
		if (st->rh_sent && !st->end_sent && !st->dead) {
			h2_ready(c, st);
		}
	}
}

// h2_on_settings applies the peer's SETTINGS frame p (n bytes).
static void h2_on_settings(h2c *c, int flags, const uint8_t *p, int64_t n) {
	if (flags & H2F_ACK) {
		if (n > 0) {
			h2_conn_err(c, H2E_FRAME_SIZE);
			return;
		}
		if (--c->unacked < 0) {
			h2_conn_err(c, H2E_PROTOCOL);
		}
		return;
	}
	if (n % 6 != 0) {
		h2_conn_err(c, H2E_FRAME_SIZE);
		return;
	}
	int64_t k = n / 6;
	if (k > 100) {
		h2_conn_err(c, H2E_PROTOCOL);
		return;
	}
	for (int64_t i = 0; i < k; i++) {
		for (int64_t j = 0; j < i; j++) {
			if (p[6 * i] == p[6 * j] && p[6 * i + 1] == p[6 * j + 1]) {
				h2_conn_err(c, H2E_PROTOCOL);
				return;
			}
		}
	}
	for (int64_t i = 0; i < k; i++) {
		int id = p[6 * i] << 8 | p[6 * i + 1];
		uint32_t v = h2_get32(p + 6 * i + 2);
		switch (id) {
		case H2S_HEADER_TABLE:
			// applied by Xo's encoder before its next block
			if (c->nencset == c->cencset) {
				c->cencset = c->cencset ? 2 * c->cencset : 4;
				c->encset = realloc(c->encset, sizeof(uint32_t) * (size_t)c->cencset);
				if (!c->encset) {
					xo_oom();
				}
			}
			c->encset[c->nencset++] = v;
			break;
		case H2S_ENABLE_PUSH:
		case H2S_CONNECT:
			if (v > 1) {
				h2_conn_err(c, H2E_PROTOCOL);
				return;
			}
			break;
		case H2S_INITIAL_WINDOW: {
			if (v > 0x7fffffff) {
				h2_conn_err(c, H2E_FLOW);
				return;
			}
			int64_t growth = (int64_t)v - c->peer_win;
			c->peer_win = v;
			for (int64_t s = 0; s < c->ntab; s++) {
				c->tab[s]->win += growth;
				if (c->tab[s]->win > 0x7fffffff) {
					h2_conn_err(c, H2E_FLOW);
					return;
				}
			}
			break;
		}
		case H2S_MAX_FRAME:
			if (v < 16384 || v > (1 << 24) - 1) {
				h2_conn_err(c, H2E_PROTOCOL);
				return;
			}
			c->peer_frame = v;
			break;
		default:
			break;
		}
	}
	h2_fh(&c->wb, 0, H2_SETTINGS, H2F_ACK, 0);
	h2_enqueue_pending(c);
}

// h2_frame handles one frame (header h, payload p): the frame order and
// the frame checks of Framer.ReadFrame, then serverConn.processFrame.
static void h2_block_end(h2c *c, int ok, int from_data, const uint8_t *data, int64_t dn, int64_t used, int flags);
static void h2_frame_rest(h2c *c, int type, int flags, uint32_t sid, uint32_t len, const uint8_t *p, const uint8_t *data, int64_t dn);

static void h2_frame(h2c *c, const uint8_t *h, const uint8_t *p) {
	uint32_t len = (uint32_t)h[0] << 16 | (uint32_t)h[1] << 8 | h[2];
	int type = h[3], flags = h[4];
	uint32_t sid = h2_get32(h + 5) & 0x7fffffff;
	c->cur_sid = 0;
	// checkFrameOrder and the Framer's parsers (connection errors of the
	// frame's shape): std/native/llrt/h2w.xo frame_check
	int64_t fr[3] = {0, 0, 0};
	switch (xo_x_h2_frame_check(XO_ADDR(h), XO_ADDR(p), c->hb_on, c->hb_sid, XO_ADDR(fr))) {
	case H2_FC_IGNORE:
		return; // unknown frame types are ignored
	case H2_FC_CONN:
		h2_conn_err(c, (uint32_t)fr[0]);
		return;
	case H2_FC_STREAM:
		h2_stream_err(c, sid, (uint32_t)fr[0]);
		return;
	}
	const uint8_t *data = p + fr[0];
	int64_t dn = fr[1];
	uint32_t dep = (uint32_t)fr[2];
	// the frame parsed: processFrame's errors count its stream
	c->cur_sid = sid;
	// the first frame must be SETTINGS (a header block is decoded first)
	if (!c->got_settings && type != H2_HEADERS && type != H2_CONT) {
		if (type != H2_SETTINGS) {
			h2_conn_err(c, H2E_PROTOCOL);
			return;
		}
		c->got_settings = 1;
		c->deadline = 0;
	}
	if (type == H2_HEADERS || type == H2_CONT) {
		if (type == H2_HEADERS) {
			c->hb.n = 0;
			c->hb_sid = sid;
			c->hb_flags = flags;
			c->hb_prio = (flags & H2F_PRIORITY) != 0;
			c->hb_dep = dep;
			hl_free(&c->hf.f);
			c->hf = (h2_fields){.remain = H2_MAX_HLIST};
			c->hfirst = 1;
			h2_tgt_clear(c);
		}
		// a fragment far over what the header list may still hold, or
		// one after an invalid field: the connection ends (CVE-2024-27316,
		// readMetaFrame)
		if (dn > 2 * c->hf.remain || c->hf.invalid) {
			h2_conn_err(c, H2E_PROTOCOL);
			return;
		}
		// Xo decodes the fields the fragment completes (c->hb, after the
		// bytes of a field the last one cut; h2_block), then h2_fields
		// goes on
		nbuf_put(&c->hb, (const char *)data, dn);
		c->dec_pending = 1;
		c->pend_flags = flags;
		c->pend_sid = sid;
		return;
	}
	h2_frame_rest(c, type, flags, sid, len, p, data, dn);
}

// h2_block_end goes on with a header block fragment whose fields were
// decoded up to used (ok 0: a decoding error): the rest waits in c->hb
// (from_data: the fragment data[:dn] was decoded in place, not c->hb),
// and a complete block is a request or trailers.
static void h2_block_end(h2c *c, int ok, int from_data, const uint8_t *data, int64_t dn, int64_t used, int flags) {
	if (!ok) {
		h2_conn_err(c, H2E_COMPRESSION);
		return;
	}
	if (from_data) {
		c->hb.n = 0;
		nbuf_put(&c->hb, (const char *)data + used, dn - used);
	} else {
		if (c->hb.n > used) {
			memmove(c->hb.p, c->hb.p + used, (size_t)(c->hb.n - used));
		}
		c->hb.n = c->hb.n - used;
	}
	if (c->hb.n > 2 * ((int64_t)H2_MAX_HLIST + 8)) {
		h2_conn_err(c, H2E_COMPRESSION);
		return;
	}
	if (!(flags & H2F_END_HEADERS)) {
		c->hb_on = 1;
		return;
	}
	c->hb_on = 0;
	if (c->hb.n > 0) {
		// truncated headers
		h2_conn_err(c, H2E_COMPRESSION);
		return;
	}
	if (!c->got_settings) {
		h2_conn_err(c, H2E_PROTOCOL);
		return;
	}
	h2_on_headers(c, c->hb_sid, c->hb_flags, c->hb_prio, c->hb_dep, &c->hf);
	if (c->hb.cap > (64 << 10)) {
		free(c->hb.p);
		c->hb = (nbuf){0};
	}
}

// h2_frame_rest handles a frame other than HEADERS and CONTINUATION that
// parsed (data[:dn] its payload without padding).
static void h2_frame_rest(h2c *c, int type, int flags, uint32_t sid, uint32_t len, const uint8_t *p, const uint8_t *data, int64_t dn) {
	// after a GOAWAY: frames of new streams (or all, after an error) are
	// dropped, the connection's window kept
	if (c->goaway && (c->failed || sid > c->max_id)) {
		if (type == H2_DATA && !c->failed) {
			if (c->in.avail < len) {
				h2_stream_err(c, sid, H2E_FLOW);
				return;
			}
			c->in.avail -= len;
			int64_t u = h2_inflow_add(&c->in, len);
			if (u > 0) {
				h2_put_winup(&c->wb, 0, (uint32_t)u);
			}
		}
		return;
	}
	switch (type) {
	case H2_PUSH:
		// a client cannot push
		h2_conn_err(c, H2E_PROTOCOL);
		break;
	case H2_DATA:
		h2_on_data(c, sid, flags, len, data, dn);
		break;
	case H2_SETTINGS:
		h2_on_settings(c, flags, p, len);
		break;
	case H2_PING:
		if (!(flags & H2F_ACK)) {
			h2_fh(&c->wb, 8, H2_PING, H2F_ACK, 0);
			nbuf_put(&c->wb, (const char *)p, 8);
		}
		break;
	case H2_WINUP: {
		uint32_t inc = h2_get32(p) & 0x7fffffff;
		if (sid == 0) {
			c->win += inc;
			if (c->win > 0x7fffffff) {
				h2_conn_err(c, H2E_FLOW);
				return;
			}
			h2_enqueue_pending(c);
			break;
		}
		h2s *st = h2_find(c, sid);
		if (!st) {
			if (sid > c->max_id) {
				h2_conn_err(c, H2E_PROTOCOL);
			}
			break;
		}
		st->win += inc;
		if (st->win > 0x7fffffff) {
			h2_stream_err(c, sid, H2E_FLOW);
			break;
		}
		if (st->rh_sent && !st->end_sent && !st->dead) {
			h2_ready(c, st);
		}
		break;
	}
	case H2_RST: {
		h2s *st = h2_find(c, sid);
		if (!st) {
			if (sid > c->max_id) {
				h2_conn_err(c, H2E_PROTOCOL);
			}
			break;
		}
		// the handler, if one runs, finishes unseen
		st->dead = 1;
		h2_close_stream(c, st);
		break;
	}
	case H2_PRIORITY:
		if ((h2_get32(p) & 0x7fffffff) == sid) {
			h2_stream_err(c, sid, H2E_PROTOCOL);
		}
		break;
	case H2_GOAWAY:
		// the peer leaves: no new streams, the open ones finish
		if (!c->goaway) {
			c->goaway = 1;
			h2_put_goaway(&c->wb, c->max_id, H2E_NO, NULL);
		}
		break;
	default:
		break;
	}
}

// h2_input processes the bytes b[*off:n] as far as they make frames:
// 0 when the connection must close at once (a client preface that is
// wrong, as net/http's readPreface).
static int h2_input(h2c *c, const char *b, int64_t n, int64_t *off) {
	static const char pre[] = H2_PREFACE;
	while (c->npreface < 24 && *off < n) {
		if (b[*off] != pre[c->npreface]) {
			return 0;
		}
		c->npreface++;
		(*off)++;
		if (c->npreface == 24) {
			// the first SETTINGS must follow within 2 s (firstSettingsTimeout)
			c->deadline = deadline((int64_t)2000000000);
		}
	}
	while (c->npreface == 24 && n - *off >= 9) {
		const uint8_t *h = (const uint8_t *)b + *off;
		uint32_t len = (uint32_t)h[0] << 16 | (uint32_t)h[1] << 8 | h[2];
		if (c->failed) {
			// lingering after a GOAWAY of an error: input is dropped
			*off = n;
			return 1;
		}
		if (len > H2_MAX_FRAME) {
			h2_conn_err(c, H2E_FRAME_SIZE);
			*off = n;
			return 1;
		}
		if (n - *off < 9 + (int64_t)len) {
			break;
		}
		h2_frame(c, h, h + 9);
		c->cur_sid = 0;
		*off += 9 + len;
		if (c->dec_pending) {
			return 1; // Xo decodes the fragment first (h2_next returns 2)
		}
	}
	if (c->failed) {
		*off = n;
	}
	return 1;
}

// ---------------------------------------------------------------------------
// Responses (responseWriterState.writeChunk with std/rt's serve)

// h2_head writes the response head of status from the handler's fields
// h (canonical names) as writeChunk sends it with p, the first bytes of
// the body (all of it when done), in the form of the encoder's job
// (xo_h2_enc_job): std/native/llrt/h2w.xo's head. *close is set when the
// handler asked for Connection: close.
static void h2_head(hlist *h, int64_t status, int head_req, const char *p, int64_t pn, int done, nbuf *out, int *close) {
	nbuf_grow(out, xo_x_h2_head_room(XO_ADDR(h->a), h->n));
	int64_t res = 0;
	out->n += xo_x_h2_head(XO_ADDR(out->p + out->n), XO_ADDR(h->a), h->n, status, head_req, XO_ADDR(p), pn, done, XO_ADDR(&res));
	if (res) {
		*close = 1;
	}
}

// h2_queue puts a response header (when head is not NULL, taken over),
// body bytes, and the end of the body on st, for the connection's fiber
// (c->mu held).
static void h2_queue_locked(h2c *c, h2s *st, nbuf *head, int nobody, const char *p, int64_t n, int end) {
	if (st->dead || st->end) {
		if (head) {
			free(head->p);
			*head = (nbuf){0};
		}
		return;
	}
	if (head) {
		free(st->rh.p);
		st->rh = *head;
		*head = (nbuf){0};
		st->rh_ready = 1;
		st->nobody = nobody;
		if (nobody) {
			end = 1;
		}
	}
	if (n > 0 && !st->nobody) {
		nbuf_put(&st->out, p, n);
	}
	if (end) {
		st->end = 1;
	}
	h2_ready(c, st);
}

static int h2_is_head(h2s *st) { return hs_eq(st->req.method, "HEAD"); }

// h2_respond_internal answers st from the connection itself (431, 400,
// OPTIONS *): a whole response, as a handler of net/http would write it.
static int h2_respond_internal(h2c *c, h2s *st, int64_t status, hlist *h, const char *body, int64_t bn) {
	hlist empty = {0};
	nbuf out = {0};
	int close = 0;
	int head = h2_is_head(st);
	h2_head(h ? h : &empty, status, head, body, bn, 1, &out, &close);
	h2_queue_locked(c, st, &out, head || !h_body_allowed(status), body, bn, 1);
	return 0;
}

// h2_respond is xo_hc_respond on a stream: the whole response at once.
static int64_t h2_respond(xo_hconn *hc, int64_t status, const char *body, int64_t bn) {
	h2s *st = hc->h2s;
	h2c *c = st->c;
	if (bn > 2048 && h_body_allowed(status) && hl_find(&hc->resp, "Content-Length") < 0) {
		// std/rt: Content-Length on a body over net/http's 2048 bytes
		char cl[32];
		int cn = fmt_int(cl, bn);
		hl_add(&hc->resp, hs_dup("Content-Length", 14), hs_dup(cl, cn));
	}
	int head = hs_eq(hc->req.method, "HEAD");
	nbuf out = {0};
	int close = 0;
	// a body over the 4 KiB buffer is written before the handler ends
	h2_head(&hc->resp, status, head, body, bn, bn <= H2_CHUNK, &out, &close);
	hl_clear(&hc->resp);
	pthread_mutex_lock(&c->mu);
	h2_queue_locked(c, st, &out, head || !h_body_allowed(status), body, bn, 1);
	if (close && !c->goaway) {
		c->stop = 1;
		h2_wake(c);
	}
	pthread_mutex_unlock(&c->mu);
	return 0;
}

// h2_wait_backlog waits while more than H2_BACKLOG bytes of st's body
// are unsent: 0 when the stream is gone (reset, the connection failed).
static int h2_wait_backlog(h2s *st) {
	h2c *c = st->c;
	xo_ctx *x = NULL;
	pthread_mutex_lock(&c->mu);
	while (!st->dead && st->out.n - st->off > H2_BACKLOG && !ctx_canceled(x ? x : (x = ctx()))) {
		park_reset(&x->park);
		st->waiter = &x->park;
		pthread_mutex_unlock(&c->mu);
		park_wait_n(x, 1, 0);
		pthread_mutex_lock(&c->mu);
		if (st->waiter == &x->park) {
			st->waiter = NULL;
		}
	}
	int ok = !st->dead;
	pthread_mutex_unlock(&c->mu);
	return ok;
}

// h2_sink_emit sends p for a streamed body (the handler is not done):
// the header with the first bytes, then data.
static int h2_sink_emit(xo_hconn *hc, int64_t status, int *started, const char *p, int64_t n) {
	h2s *st = hc->h2s;
	h2c *c = st->c;
	nbuf out = {0};
	int head = hs_eq(hc->req.method, "HEAD");
	int close = 0;
	if (!*started) {
		*started = 1;
		h2_head(&hc->resp, status, head, p, n, 0, &out, &close);
		hl_clear(&hc->resp);
	}
	pthread_mutex_lock(&c->mu);
	h2_queue_locked(c, st, out.n ? &out : NULL, head || !h_body_allowed(status), p, n, 0);
	if (close && !c->goaway) {
		c->stop = 1;
		h2_wake(c);
	}
	pthread_mutex_unlock(&c->mu);
	free(out.p);
	return h2_wait_backlog(st);
}

// h2_sink_finish ends a streamed body whose header went out.
static int h2_sink_finish(xo_hconn *hc, const char *p, int64_t n) {
	h2s *st = hc->h2s;
	h2c *c = st->c;
	pthread_mutex_lock(&c->mu);
	int ok = !st->dead;
	h2_queue_locked(c, st, NULL, 0, p, n, 1);
	pthread_mutex_unlock(&c->mu);
	return ok;
}

// ---------------------------------------------------------------------------
// Sending (the connection's fiber, c->mu held)

static void h2_after_end(h2c *c, h2s *st) {
	if (st->state == H2ST_OPEN) {
		// the request is still open: RST_STREAM NO_ERROR (section 8.1)
		if (!st->rst) {
			st->rst = H2E_NO + 1;
			h2_ready(c, st);
		}
	} else {
		h2_close_stream(c, st);
	}
}

// h2_schedule moves queued output into c->wb: reset streams, headers,
// and data within the peer's windows, at most 64 KiB per stream before
// the next stream's turn.
static void h2_schedule(h2c *c) {
	h2s *list = c->rfirst;
	c->rfirst = c->rlast = NULL;
	if (c->failed) {
		// after a GOAWAY of an error nothing more is sent on a stream
		while (list) {
			h2s *st = list;
			list = st->rnext;
			st->rnext = NULL;
			st->queued = 0;
			h2s_unref(st);
		}
		return;
	}
	h2s *again = NULL, *again_last = NULL;
	uint32_t fmax = c->peer_frame < 16384 ? c->peer_frame : 16384;
	while (list) {
		h2s *st = list;
		list = st->rnext;
		st->rnext = NULL;
		st->queued = 0;
		// a handler's fault resets its stream after what it queued
		int late = st->rst && st->rst_late && !st->dead && ((st->rh_ready && !st->rh_sent) || st->off < st->out.n);
		if (st->rst && !st->rst_sent && !late) {
			st->rst_sent = 1;
			if (st->in_table || !st->rst_late) {
				// (a stream the client reset needs no reset of ours)
				h2_put_rst(&c->wb, st->id, (uint32_t)(st->rst - 1));
			}
			h2_close_stream(c, st);
			h2s_unref(st);
			continue;
		}
		if (st->dead || st->end_sent) {
			h2s_unref(st);
			continue;
		}
		if (st->rh_ready && !st->rh_sent) {
			// encoded by Xo under this lock (h2_enc_job, h2_encoded)
			nbuf blk = st->hblk;
			st->hblk = (nbuf){0};
			st->hblk_ok = 0;
			int es = st->end && st->off == st->out.n;
			h2_put_block(&c->wb, st->id, blk.p, blk.n, es, c->peer_frame);
			free(blk.p);
			st->rh_sent = 1;
			if (es) {
				st->end_sent = 1;
				h2_after_end(c, st);
				h2s_unref(st);
				continue;
			}
		}
		if (!st->rh_sent) {
			h2s_unref(st);
			continue;
		}
		int64_t budget = 64 << 10;
		int more = 0;
		for (;;) {
			int64_t pend = st->out.n - st->off;
			if (pend == 0) {
				if (st->end) {
					h2_fh(&c->wb, 0, H2_DATA, H2F_END_STREAM, st->id);
					st->end_sent = 1;
				}
				break;
			}
			int64_t k = pend;
			if (k > fmax) {
				k = fmax;
			}
			if (k > st->win) {
				k = st->win;
			}
			if (k > c->win) {
				k = c->win;
			}
			if (k > budget) {
				k = budget;
			}
			if (k <= 0) {
				more = budget <= 0;
				break; // the peer's windows are closed, or the turn is over
			}
			int es = st->end && k == pend;
			h2_fh(&c->wb, (uint32_t)k, H2_DATA, es ? H2F_END_STREAM : 0, st->id);
			nbuf_put(&c->wb, st->out.p + st->off, k);
			st->off += k;
			st->win -= k;
			c->win -= k;
			budget -= k;
			if (es) {
				st->end_sent = 1;
				break;
			}
		}
		if (st->off == st->out.n) {
			st->off = st->out.n = 0;
			if (st->out.cap > (64 << 10)) {
				free(st->out.p);
				st->out = (nbuf){0};
			}
		} else if (st->off > (1 << 20)) {
			memmove(st->out.p, st->out.p + st->off, (size_t)(st->out.n - st->off));
			st->out.n -= st->off;
			st->off = 0;
		}
		if (st->out.n - st->off <= H2_BACKLOG) {
			h2_signal_stream(st);
		}
		if (late && st->rh_sent && st->off == st->out.n) {
			st->rst_sent = 1;
			h2_put_rst(&c->wb, st->id, (uint32_t)(st->rst - 1));
			h2_close_stream(c, st);
			h2s_unref(st);
			continue;
		}
		if (st->end_sent) {
			h2_after_end(c, st);
			h2s_unref(st);
			continue;
		}
		if (more) {
			// its turn is over: after the others (keeps the reference)
			st->queued = 1;
			if (again_last) {
				again_last->rnext = st;
			} else {
				again = st;
			}
			again_last = st;
			continue;
		}
		h2s_unref(st);
	}
	if (again) {
		// streams queued meanwhile (resets) go first
		if (c->rlast) {
			c->rlast->rnext = again;
		} else {
			c->rfirst = again;
		}
		c->rlast = again_last;
	}
}

// h2_shutdown marks the connection dead: every stream is dropped and
// the tasks waiting on one return.
static void h2_shutdown(h2c *c) {
	c->dead = 1;
	while (c->ntab > 0) {
		h2_close_stream(c, c->tab[c->ntab - 1]);
	}
	for (int64_t i = c->dqoff; i < c->dqn; i++) {
		c->dq[i]->dead = 1;
		h2s_unref(c->dq[i]);
	}
	c->dqoff = c->dqn = 0;
	while (c->rfirst) {
		h2s *st = c->rfirst;
		c->rfirst = st->rnext;
		st->queued = 0;
		h2s_unref(st);
	}
	c->rlast = NULL;
}

// ---------------------------------------------------------------------------
// The connection and its fiber

static h2c *h2c_new(xo_hconn *hc) {
	h2c *c = calloc(1, sizeof(h2c));
	if (!c) {
		xo_oom();
	}
	mu_init(&c->mu);
	c->hc = hc;
	c->peer_frame = 16384;
	c->peer_win = 65535;
	c->win = 65535;
	c->in.avail = H2_WINDOW;
	// the server's preface: SETTINGS, then the connection window to 1 MiB
	h2_fh(&c->wb, 30, H2_SETTINGS, 0, 0);
	h2_put_setting(&c->wb, H2S_MAX_FRAME, H2_MAX_FRAME);
	h2_put_setting(&c->wb, H2S_MAX_STREAMS, H2_MAX_STREAMS);
	h2_put_setting(&c->wb, H2S_MAX_HLIST, H2_MAX_HLIST);
	h2_put_setting(&c->wb, H2S_HEADER_TABLE, H2_TABLE);
	h2_put_setting(&c->wb, H2S_INITIAL_WINDOW, H2_WINDOW);
	c->unacked = 1;
	h2_put_winup(&c->wb, 0, H2_WINDOW - 65535);
	// the client preface within 10 s (prefaceTimeout)
	c->deadline = deadline((int64_t)10 * 1000000000);
	return c;
}

static void h2c_free(h2c *c) {
	if (!c) {
		return;
	}
	h2_shutdown(c);
	free(c->tab);
	free(c->dq);
	free(c->encset);
	free(c->encq);
	free(c->hb.p);
	hl_free(&c->hf.f);
	h2_tgt_clear(c);
	free(c->wb.p);
	pthread_mutex_destroy(&c->mu);
	free(c);
}

// h2_take_locked pops the next request whose stream is alive: its
// xo_hconn, or NULL.
static xo_hconn *h2_take_locked(h2c *c) {
	while (c->dqoff < c->dqn && c->nhandlers < H2_MAX_STREAMS) {
		h2s *st = c->dq[c->dqoff++];
		if (c->dqoff == c->dqn) {
			c->dqoff = c->dqn = 0;
		}
		if (st->dead) {
			h2s_unref(st);
			continue;
		}
		// the queue's reference becomes the xo_hconn's
		xo_hconn *hc = xo_alloc(sizeof(xo_hconn));
		memset(hc, 0, sizeof *hc);
		h_init(hc, hconn_free);
		h_retain(c->hc);
		hc->h2owner = c->hc;
		hc->h2s = st;
		hc->req = st->req;
		memset(&st->req, 0, sizeof st->req);
		hc->have_req = 1;
		hc->body = st->body;
		st->body = (nbuf){0};
		memcpy(hc->ip, c->hc->ip, sizeof hc->ip);
		st->dispatched = 1;
		c->nhandlers++;
		return hc;
	}
	return NULL;
}

// h2_stream_release drops the xo_hconn's reference to its stream.
static void h2_stream_release(xo_hconn *hc) {
	h2s *st = hc->h2s;
	h2c *c = st->c;
	pthread_mutex_lock(&c->mu);
	if (st->dispatched && !st->ended) {
		st->ended = 1;
		c->nhandlers--;
		if (!st->end && !st->dead && !st->rst) {
			st->rst = H2E_INTERNAL + 1;
			st->rst_late = 1;
			h2_ready(c, st);
		}
		h2_wake(c);
	}
	h2s_unref(st);
	pthread_mutex_unlock(&c->mu);
	hc->h2s = NULL;
}

// xo_h2_end ends a stream's handler: a response it did not finish (a
// fault in a streamed body) resets the stream with INTERNAL_ERROR, as
// net/http's handlerPanicRST.
void xo_h2_end(xo_hconn *hc) {
	h2s *st = hc->h2s;
	if (!st) {
		return;
	}
	h2c *c = st->c;
	pthread_mutex_lock(&c->mu);
	if (!st->ended) {
		st->ended = 1;
		c->nhandlers--;
		if (!st->end && !st->dead && !st->rst) {
			st->rst = H2E_INTERNAL + 1;
			st->rst_late = 1;
			h2_ready(c, st);
		}
		h2_wake(c);
	}
	pthread_mutex_unlock(&c->mu);
}

// h2_stop asks the connection to end (the server stops): GOAWAY, then
// close once its streams are done.
static void h2_stop(h2c *c) {
	pthread_mutex_lock(&c->mu);
	c->stop = 1;
	h2_wake(c);
	pthread_mutex_unlock(&c->mu);
}

static void h2_timer_fn(xo_timer *t) { park_signal(t->arg); }

// h2_wait parks the connection's fiber until the socket has input, a
// stream task woke it, its deadline passed, or the task was canceled:
// 0 when it must end (closed, canceled, the deadline).
static int h2_wait(h2c *c, xo_ctx *x) {
	xo_pd *pd = c->hc->conn->pd;
	park_reset(&x->park);
	pthread_mutex_lock(&c->mu);
	if (c->woken) {
		c->woken = 0;
		pthread_mutex_unlock(&c->mu);
		return 1;
	}
	c->waiter = &x->park;
	int64_t when = c->deadline;
	pthread_mutex_unlock(&c->mu);
	int res = 1;
	lk(&pd->mu);
	if (pd->closed) {
		res = 0;
	} else if (pd->rready) {
		pd->rready = 0;
	} else {
		pd->rwait = &x->park;
#ifdef XO_WINPOLL
		// the poller watches what someone waits for (pd_wait_until)
		if (ev.afd) {
			pd_arm(pd);
		}
#endif
		unlk(&pd->mu);
#ifdef XO_WINPOLL
		if (!ev.afd) {
			ev_poke();
		}
#endif
		xo_timer t = {.when = when, .idx = -1, .fn = h2_timer_fn, .arg = &x->park};
		if (when) {
			if (tnow() >= when) {
				t.fired = 1;
			} else {
				timer_add(&t);
			}
		}
		if (!t.fired) {
			park_wait_n(x, 1, 0);
		}
		if (when) {
			timer_del(&t);
			if (__atomic_load_n(&t.fired, __ATOMIC_ACQUIRE) || tnow() >= when) {
				res = 0;
			}
		}
		lk(&pd->mu);
		if (pd->rwait == &x->park) {
			pd->rwait = NULL;
		}
		if (pd->closed) {
			res = 0;
		}
	}
	unlk(&pd->mu);
	pthread_mutex_lock(&c->mu);
	if (c->waiter == &x->park) {
		c->waiter = NULL;
	}
	c->woken = 0;
	pthread_mutex_unlock(&c->mu);
	if (ctx_canceled(x)) {
		res = 0;
	}
	return res;
}

// h2_read reads what the socket has (TLS records): the bytes read, 0 at
// the end of the stream, -1 on an error, -2 when nothing is there.
static int64_t h2_read(xo_hconn *hc) {
	if (hc->off > 0 && hc->off == hc->n) {
		hc->off = hc->n = 0;
	}
	if (hc->cap - hc->n < 16384) {
		if (hc->off > 0) {
			memmove(hc->buf, hc->buf + hc->off, (size_t)(hc->n - hc->off));
			hc->n -= hc->off;
			hc->off = 0;
		}
		if (hc->cap - hc->n < 16384) {
			hc->cap = hc->cap ? hc->cap * 2 : 32768;
			hc->buf = realloc(hc->buf, (size_t)hc->cap);
			if (!hc->buf) {
				xo_oom();
			}
		}
	}
	int64_t r = tls_nb_read(hc->tls, hc->buf + hc->n, hc->cap - hc->n);
	if (r > 0) {
		hc->n += r;
	}
	return r;
}

// xo_hc_h2 finishes the TLS handshake of a server connection and tells
// whether ALPN chose HTTP/2 (then the connection's requests come from
// h2_next); false: HTTP/1.1 (or the handshake failed, which hc_read
// finds).
int64_t xo_hc_h2(xo_hconn *hc) {
	if (!hc->tls || !hc->conn) {
		return 0;
	}
	xo_srv *s = hc->srv;
	if (s) {
		pthread_mutex_lock(&s->mu);
		if (s->stopping) {
			pthread_mutex_unlock(&s->mu);
			return 0;
		}
		hc->idle = 1;
		pthread_mutex_unlock(&s->mu);
	}
	int ok = !ctx_canceled(ctx()) && tls_server_handshake(hc->tls);
	if (s) {
		pthread_mutex_lock(&s->mu);
		hc->idle = 0;
		pthread_mutex_unlock(&s->mu);
	}
	if (!ok || !tls_alpn_h2(hc->tls)) {
		return 0;
	}
	h2c *c = h2c_new(hc);
	char why[80];
	if (tls_h2_bad_cipher(hc->tls, why, sizeof why)) {
		// rejectConn: HTTP/2 needs TLS 1.2 with an AEAD suite at least
		h2_put_goaway(&c->wb, 0, H2E_INADEQUATE, why);
		c->failed = c->goaway = c->reject = 1;
#ifdef _WIN32
		// Windows resets a connection closed with unread input (the
		// client's preface, gotchas/windows-shutdown-resets-unread-input.md),
		// and the reset can overtake the GOAWAY: linger as after a
		// connection error, dropping input, until the client closes or
		// a second passes
		c->reject = 0;
		c->deadline = deadline((int64_t)1000000000);
#endif
	}
	if (s) {
		pthread_mutex_lock(&s->mu);
		hc->h2 = c;
		if (s->stopping) {
			c->stop = 1;
		}
		pthread_mutex_unlock(&s->mu);
	} else {
		hc->h2 = c;
	}
	return 1;
}

// h2_enc_needed lists in c->encq the streams whose response heads the
// next h2_schedule sends (in its order, by its tests), which Xo encodes:
// whether there is one. Under the connection's lock.
static int h2_enc_needed(h2c *c) {
	c->nencq = 0;
	if (c->failed) {
		return 0;
	}
	for (h2s *st = c->rfirst; st; st = st->rnext) {
		int late = st->rst && st->rst_late && !st->dead && ((st->rh_ready && !st->rh_sent) || st->off < st->out.n);
		if (st->rst && !st->rst_sent && !late) {
			continue;
		}
		if (st->dead || st->end_sent || !st->rh_ready || st->rh_sent || st->hblk_ok) {
			continue;
		}
		if (c->nencq == c->cencq) {
			c->cencq = c->cencq ? 2 * c->cencq : 8;
			c->encq = realloc(c->encq, sizeof(h2s *) * (size_t)c->cencq);
			if (!c->encq) {
				xo_oom();
			}
		}
		c->encq[c->nencq++] = st;
	}
	return c->nencq > 0;
}

// h2_feed processes the input read so far: 0 when the connection must
// close, 1 to go on, 2 when Xo decodes a header block fragment first.
static int h2_feed(h2c *c, xo_hconn *hc) {
	pthread_mutex_lock(&c->mu);
	int ok = h2_input(c, hc->buf, hc->n, &hc->off);
	pthread_mutex_unlock(&c->mu);
	if (ok && c->dec_pending) {
		return 2;
	}
	return ok;
}

// xo_h2_next runs the connection until a request is ready for a handler
// (1; h2_take hands it out) or the connection is done (0): it sends what
// the streams queued, reads and answers frames, and closes after a
// GOAWAY once the streams in flight are done. With HPACK in Xo it also
// stops when Xo has a header block fragment to decode (2: the
// fiber calls h2_block and h2_fields, then h2_next again) or response
// heads to encode (3: the connection's lock stays held until the fiber
// calls h2_enc_job and h2_encoded, then h2_next again, so the heads go
// out in the order and with the table they were encoded with).
int64_t xo_h2_next(xo_hconn *hc) {
	h2c *c = hc->h2;
	xo_ctx *x = ctx();
	nbuf out = {0};
	int resume = c->phase;
	c->phase = H2P_NONE;
	for (;;) {
		if (resume == H2P_INPUT) {
			// the rest of the input after the fragment Xo decoded
			resume = H2P_NONE;
			int f = h2_feed(c, hc);
			if (f == 2) {
				c->phase = H2P_INPUT;
				free(out.p);
				return 2;
			}
			if (!f) {
				break;
			}
			continue;
		}
		if (resume == H2P_ENCODED) {
			resume = H2P_NONE; // the lock is held since the 3
		} else {
			pthread_mutex_lock(&c->mu);
			if (!c->dead && !c->failed) {
				xo_hconn *t = h2_take_locked(c);
				if (t) {
					c->take = t;
					pthread_mutex_unlock(&c->mu);
					free(out.p);
					return 1;
				}
			}
			if (c->stop && !c->goaway) {
				c->goaway = 1;
				h2_put_goaway(&c->wb, c->max_id, H2E_NO, NULL);
			}
			if (h2_enc_needed(c)) {
				free(out.p);
				return 3; // with the lock held
			}
		}
		h2_schedule(c);
		int pending = c->rfirst != NULL;
		// a graceful end: every stream done and sent
		int finished = c->dead || c->reject || (c->goaway && !c->failed && c->nopen == 0 && c->dqoff == c->dqn && !pending);
		// swap the output buffer out, to write it without the lock
		nbuf w = c->wb;
		c->wb = out;
		c->wb.n = 0;
		out = w;
		pthread_mutex_unlock(&c->mu);
		if (out.n > 0) {
			int ok = hc_send(hc, out.p, out.n);
			out.n = 0;
			if (out.cap > (256 << 10)) {
				free(out.p);
				out = (nbuf){0};
			}
			if (!ok) {
				finished = 1;
			}
		}
		if (finished) {
			break;
		}
		int64_t r = h2_read(hc);
		if (r > 0) {
			int f = h2_feed(c, hc);
			if (f == 2) {
				c->phase = H2P_INPUT;
				free(out.p);
				return 2;
			}
			if (!f) {
				break;
			}
			continue;
		}
		if (r != -2) {
			break; // the end of the stream (the client left) or an error
		}
		if (pending) {
			continue; // more to send first
		}
		if (!h2_wait(c, x)) {
			break;
		}
	}
	free(out.p);
	pthread_mutex_lock(&c->mu);
	h2_shutdown(c);
	pthread_mutex_unlock(&c->mu);
	return 0;
}

// xo_h2_block is the header block fragment Xo decodes (after h2_next
// returned 2): the bytes a previous fragment left, then the fragment.
xo_rstr *xo_h2_block(xo_hconn *hc, int64_t *len) {
	h2c *c = hc->h2;
	return nstr(c->hb.p ? c->hb.p : "", c->hb.n, len);
}

// xo_h2_block_first: no representation of the block was decoded yet.
int64_t xo_h2_block_first(xo_hconn *hc) { return hc->h2->hfirst; }

static uint32_t h2_le32(const char *p) {
	const uint8_t *u = (const uint8_t *)p;
	return (uint32_t)u[0] | (uint32_t)u[1] << 8 | (uint32_t)u[2] << 16 | (uint32_t)u[3] << 24;
}

// xo_h2_fields goes on with the fragment Xo decoded: ok (0 a decoding
// error), how far it got, the fields (their names and values one after
// the other in d, their lengths in the List[Int] lens, a word a field:
// the name's in the low 32 bits, the value's above), whether the
// block's first representation is still to come, and the parse of the
// fragment's first :path field (the List[Str] tgt: empty when it has
// none, else the path, "1" when it parsed, then url_parts' scheme, host,
// path, and raw query, then the query's keys and values; h2_request
// takes the block's first). True when a 100 Continue is to be encoded
// now (h2_enc_job, h2_encoded).
int64_t xo_h2_fields(xo_hconn *hc, int64_t ok, int64_t used, const char *d, int64_t dn, xo_rlist *lens, int64_t first, xo_rlist *tgt) {
	h2c *c = hc->h2;
	pthread_mutex_lock(&c->mu);
	c->dec_pending = 0;
	c->hfirst = (int)first;
	if (tgt->len >= 6 && !c->tgt_set) {
		xo_rstr **ts = (xo_rstr **)rl_data(tgt);
		const int64_t *tl = (const int64_t *)rl_data(tgt);
		c->tgt_set = 1;
		c->tgt_raw = hs_dup(ts[0]->data, tl[1]);
		c->tgt_ok = tl[3] == 1;
		hstr *parts[] = {&c->tgt_url.scheme, &c->tgt_url.host, &c->tgt_url.path, &c->tgt_url.rawquery};
		for (int k = 0; k < 4; k++) {
			*parts[k] = hs_dup(ts[2 * (k + 2)]->data, tl[2 * (k + 2) + 1]);
		}
		for (int64_t i = 6; i + 1 < tgt->len; i += 2) {
			hl_add(&c->tgt_query, hs_dup(ts[2 * i]->data, tl[2 * i + 1]), hs_dup(ts[2 * i + 2]->data, tl[2 * i + 3]));
		}
	}
	int64_t off = 0;
	const int64_t *l = (const int64_t *)rl_data(lens);
	for (int64_t i = 0; i < lens->len; i++) {
		int64_t nn = l[i] & 0xffffffff, vn = (int64_t)((uint64_t)l[i] >> 32);
		if (off + nn + vn > dn) {
			break;
		}
		h2_collect(&c->hf, d + off, nn, d + off + nn, vn);
		off += nn + vn;
	}
	c->cur_sid = c->pend_sid;
	h2_block_end(c, (int)ok, 0, NULL, 0, used, c->pend_flags);
	c->cur_sid = 0;
	int want = c->want100 != 0;
	pthread_mutex_unlock(&c->mu);
	return want;
}

static void h2_job_u32(nbuf *b, uint32_t v) {
	char w[4] = {(char)v, (char)(v >> 8), (char)(v >> 16), (char)(v >> 24)};
	nbuf_put(b, w, 4);
}

// xo_h2_enc_job is what Xo's encoder encodes next (after h2_next
// returned 3, or h2_fields true): the peer's table size settings since
// the last job (their number, then each), then the heads (their number,
// then each head's number of fields and each field's name and value
// lengths, then the name and the value), 4 bytes little endian each
// number. The heads
// are a 100 Continue (h2_fields) or those of c->encq.
xo_rstr *xo_h2_enc_job(xo_hconn *hc, int64_t *len) {
	h2c *c = hc->h2;
	nbuf b = {0};
	h2_job_u32(&b, (uint32_t)c->nencset);
	for (int64_t i = 0; i < c->nencset; i++) {
		h2_job_u32(&b, c->encset[i]);
	}
	c->nencset = 0;
	if (c->want100) {
		h2_job_u32(&b, 1);
		h2_job_u32(&b, 1);
		h2_job_u32(&b, 7);
		h2_job_u32(&b, 3);
		nbuf_put(&b, ":status100", 10);
	} else {
		h2_job_u32(&b, (uint32_t)c->nencq);
		for (int64_t k = 0; k < c->nencq; k++) {
			nbuf *h = &c->encq[k]->rh;
			nbuf_put(&b, h->p, h->n);
		}
	}
	xo_rstr *s = nstr(b.p ? b.p : "", b.n, len);
	free(b.p);
	return s;
}

// xo_h2_encoded takes the blocks Xo encoded for the job (each after its
// length, 4 bytes): a 100 Continue goes out now; heads wait on their
// streams for h2_schedule, which the next h2_next runs under the lock
// still held.
void xo_h2_encoded(xo_hconn *hc, const char *p, int64_t n) {
	h2c *c = hc->h2;
	int64_t off = 0, k = 0;
	if (c->want100) {
		if (n >= 4 && 4 + (int64_t)h2_le32(p) <= n) {
			pthread_mutex_lock(&c->mu);
			h2_put_block(&c->wb, c->want100, p + 4, h2_le32(p), 0, c->peer_frame);
			pthread_mutex_unlock(&c->mu);
		}
		c->want100 = 0;
		return;
	}
	while (off + 4 <= n && k < c->nencq) {
		int64_t bn = h2_le32(p + off);
		if (off + 4 + bn > n) {
			break;
		}
		h2s *st = c->encq[k++];
		st->hblk.n = 0;
		nbuf_put(&st->hblk, p + off + 4, bn);
		st->hblk_ok = 1;
		off += 4 + bn;
	}
	c->nencq = 0;
	c->phase = H2P_ENCODED;
}

// h2_untake takes the request h2_next found (NULL: none).
static xo_hconn *h2_untake(h2c *c) {
	pthread_mutex_lock(&c->mu);
	xo_hconn *t = c->take;
	c->take = NULL;
	pthread_mutex_unlock(&c->mu);
	return t;
}

// xo_h2_take is the request h2_next found, as a connection of its own
// for std/native/http.xo (serve_stream).
xo_hconn *xo_h2_take(xo_hconn *hc) { return h2_untake(hc->h2); }

// ---------------------------------------------------------------------------
// The client: requests on https connections whose ALPN chose h2
//
// As net/http's Transport, a connection stays open after a request and
// serves the next request to the same address (and server name, with the
// same roots): stream ids go up on it (1, 3, 5, ...) as Go's do. The
// native client sends one request at a time on a connection; requests
// at once open more connections, and up to 100 idle ones are kept for 90
// s (IdleConnTimeout). A request that fails on a reused connection
// before any answer to it (the server closed it, a GOAWAY that excludes
// it, REFUSED_STREAM) is sent again on a new one, as Transport retries
// it.

typedef struct h2cc {
	struct h2cc *next;
	xo_troots *roots;
	char *key; // the address and the server name
	xo_conn *cn;
	ccon c;
	xo_any *hp;   // the HPACK state, Xo's (http1.xo H2State); NULL before the first request
	nbuf sizes;   // the server's SETTINGS_HEADER_TABLE_SIZE values Xo's encoder has not applied
	uint32_t peer_frame;
	int64_t peer_win, win; // the server's initial stream window, its connection window
	h2_inflow in;          // our connection window
	uint32_t next_id;
	int goaway;
	uint32_t goaway_last, goaway_code;
	char *goaway_debug;
	int broken; // not to be used again
	int64_t idle_since;
	nbuf out;
} h2cc;

// A request's stream.
typedef struct {
	uint32_t id;
	int64_t swin;
	h2_inflow sin;
	int headers; // the final response header arrived
	int ended;   // the response ended
	int seen;    // a frame of the stream arrived
	int refused; // REFUSED_STREAM, or a GOAWAY that excludes the stream
	nbuf hb;
	int hb_on, hb_flags;
	uint32_t hb_sid;
	int dec; // the block in hb waits for Xo's decoder (cl_h2_block, cl_h2_fields)
	hlist fields;
} h2rq;

static struct {
	pthread_mutex_t mu;
	h2cc *first;
	int n;
} h2pool = {.mu = PTHREAD_MUTEX_INITIALIZER};

static void h2cc_free(h2cc *pc) {
	if (!pc) {
		return;
	}
	tls_conn_free(pc->c.tls);
	pd_close(pc->c.pd);
	h_release(pc->cn);
	h_release(pc->roots);
	free(pc->c.buf);
	if (pc->hp) {
		h_release(pc->hp);
	}
	free(pc->sizes.p);
	free(pc->out.p);
	free(pc->key);
	free(pc->goaway_debug);
	free(pc);
}

// h2cc_new takes over connection cn and its TLS state c.
static h2cc *h2cc_new(xo_conn *cn, ccon *c, xo_troots *roots, const char *key) {
	h2cc *pc = calloc(1, sizeof(h2cc));
	if (!pc) {
		xo_oom();
	}
	pc->cn = cn;
	pc->c = *c;
	h_retain(roots);
	pc->roots = roots;
	pc->key = strdup(key);
	pc->peer_frame = 16384;
	pc->peer_win = 65535;
	pc->win = 65535;
	pc->in.avail = (int64_t)1 << 30;
	pc->next_id = 1;
	// the preface and settings of net/http's client
	nbuf_str(&pc->out, H2_PREFACE);
	h2_fh(&pc->out, 18, H2_SETTINGS, 0, 0);
	h2_put_setting(&pc->out, H2S_ENABLE_PUSH, 0);
	h2_put_setting(&pc->out, H2S_INITIAL_WINDOW, 4 << 20);
	h2_put_setting(&pc->out, H2S_MAX_HLIST, 10 << 20);
	h2_put_winup(&pc->out, 0, (1u << 30) - 65535);
	return pc;
}

// h2_pool_take is an idle connection for key and roots, or NULL.
static h2cc *h2_pool_take(xo_troots *roots, const char *key) {
	int64_t now = mono_ns();
	h2cc *found = NULL, *stale = NULL;
	pthread_mutex_lock(&h2pool.mu);
	for (h2cc **pp = &h2pool.first; *pp;) {
		h2cc *pc = *pp;
		if (now - pc->idle_since > (int64_t)90 * 1000000000) {
			*pp = pc->next;
			h2pool.n--;
			pc->next = stale;
			stale = pc;
			continue;
		}
		if (!found && pc->roots == roots && strcmp(pc->key, key) == 0) {
			*pp = pc->next;
			h2pool.n--;
			found = pc;
			continue;
		}
		pp = &pc->next;
	}
	pthread_mutex_unlock(&h2pool.mu);
	while (stale) {
		h2cc *n = stale->next;
		h2cc_free(stale);
		stale = n;
	}
	return found;
}

// h2_pool_put keeps pc for the next request, or closes it.
static void h2_pool_put(h2cc *pc) {
	if (pc->broken || pc->goaway || pc->next_id > 0x7ffffff0) {
		h2cc_free(pc);
		return;
	}
	pc->idle_since = mono_ns();
	h2cc *drop = NULL;
	pthread_mutex_lock(&h2pool.mu);
	pc->next = h2pool.first;
	h2pool.first = pc;
	if (++h2pool.n > 100) {
		// the oldest goes (MaxIdleConns)
		h2cc **pp = &h2pool.first;
		while ((*pp)->next) {
			pp = &(*pp)->next;
		}
		drop = *pp;
		*pp = NULL;
		h2pool.n--;
	}
	pthread_mutex_unlock(&h2pool.mu);
	h2cc_free(drop);
}

// h2_pool_free closes the idle connections at exit (leak check).
static void h2_pool_free(void) {
	pthread_mutex_lock(&h2pool.mu);
	h2cc *pc = h2pool.first;
	h2pool.first = NULL;
	h2pool.n = 0;
	pthread_mutex_unlock(&h2pool.mu);
	while (pc) {
		h2cc *n = pc->next;
		h2cc_free(pc);
		pc = n;
	}
}

// h2cc_frame waits for a whole frame in the connection's buffer: CC_OK
// with its header at *h (the payload follows), or why it failed.
static int h2cc_frame(ccon *c, const uint8_t **h) {
	for (;;) {
		int64_t have = c->n - c->off;
		if (have >= 9) {
			const uint8_t *p = (const uint8_t *)c->buf + c->off;
			uint32_t len = (uint32_t)p[0] << 16 | (uint32_t)p[1] << 8 | p[2];
			if (len > 16384) {
				return CC_ERR + 100; // FRAME_SIZE_ERROR
			}
			if (have >= 9 + (int64_t)len) {
				*h = p;
				c->off += 9 + len;
				return CC_OK;
			}
		}
		if (c->eof) {
			return CC_EOF;
		}
		int f = cc_fill(c);
		if (f != CC_OK && f != CC_EOF) {
			return f;
		}
	}
}

// h2rq_headers takes a header block of the response into r, the fields
// Xo decoded from it (ok: 0 a decoding error; the names and values one
// after the other in d, their lengths in lens as xo_h2_fields'): 0 when
// it is malformed (why says how).
static int h2rq_headers(h2cc *pc, h2rq *q, xo_cresp *r, int ok, const char *d, int64_t dn, xo_rlist *lens, char *why, size_t wn) {
	int end = q->hb_flags & H2F_END_STREAM;
	hl_clear(&q->fields);
	if (!ok) {
		pc->broken = 1;
		snprintf(why, wn, "connection error: COMPRESSION_ERROR");
		return 0;
	}
	int64_t off = 0;
	const int64_t *l = (const int64_t *)rl_data(lens);
	for (int64_t i = 0; i < lens->len; i++) {
		int64_t nn = l[i] & 0xffffffff, vn = (int64_t)((uint64_t)l[i] >> 32);
		if (off + nn + vn > dn) {
			break;
		}
		hl_add(&q->fields, hs_dup(d + off, nn), hs_dup(d + off + nn, vn));
		off += nn + vn;
	}
	if (q->hb_sid != q->id) {
		return 1; // another stream's (an old one's trailers): only the table counts
	}
	if (q->headers) {
		// trailers: dropped (std/rt does not read them)
		if (end) {
			q->ended = 1;
		}
		return 1;
	}
	hstr st = h2_pseudo(&q->fields, ":status");
	if (!st.p) {
		snprintf(why, wn, "malformed response from server: missing status pseudo header");
		return 0;
	}
	int64_t code = 0;
	for (int64_t i = 0; i < st.n; i++) {
		if (st.p[i] < '0' || st.p[i] > '9' || i >= 9) {
			snprintf(why, wn, "malformed response from server: malformed non-numeric status pseudo header");
			return 0;
		}
		code = code * 10 + (st.p[i] - '0');
	}
	if (st.n == 0) {
		snprintf(why, wn, "malformed response from server: malformed non-numeric status pseudo header");
		return 0;
	}
	if (code >= 100 && code <= 199 && !end) {
		return 1; // informational: the response follows
	}
	q->headers = 1;
	r->status = code;
	hl_free(&r->hdr);
	for (int64_t i = 0; i < q->fields.n; i++) {
		hstr k = q->fields.a[i].k;
		if (k.n > 0 && k.p[0] == ':') {
			continue;
		}
		char *ck = xo_alloc(k.n + 1);
		memcpy(ck, k.p, (size_t)k.n);
		ck[k.n] = 0;
		h_canon(ck, k.n);
		h2_add_field(&r->hdr, ck, k.n, q->fields.a[i].v.p, q->fields.a[i].v.n);
		xo_free(ck);
	}
	if (end) {
		q->ended = 1;
	}
	return 1;
}

// h2cc_process handles one frame of the server: 0 when the request
// failed (why set).
static int h2cc_process(h2cc *pc, h2rq *q, const uint8_t *h, xo_cresp *r, const char *method, char *why, size_t wn) {
	uint32_t len = (uint32_t)h[0] << 16 | (uint32_t)h[1] << 8 | h[2];
	int type = h[3], flags = h[4];
	uint32_t sid = h2_get32(h + 5) & 0x7fffffff;
	const uint8_t *p = h + 9;
	if (sid == q->id && sid != 0) {
		q->seen = 1;
	}
	if (q->hb_on && (type != H2_CONT || sid != q->hb_sid)) {
		pc->broken = 1;
		snprintf(why, wn, "connection error: PROTOCOL_ERROR");
		return 0;
	}
	switch (type) {
	case H2_SETTINGS:
		if (flags & H2F_ACK) {
			break;
		}
		for (uint32_t i = 0; i + 6 <= len; i += 6) {
			int id = p[i] << 8 | p[i + 1];
			uint32_t v = h2_get32(p + i + 2);
			if (id == H2S_INITIAL_WINDOW && v <= 0x7fffffff) {
				q->swin += (int64_t)v - pc->peer_win;
				pc->peer_win = v;
			} else if (id == H2S_MAX_FRAME && v >= 16384 && v < (1 << 24)) {
				pc->peer_frame = v;
			} else if (id == H2S_HEADER_TABLE) {
				// for Xo's encoder, before the next request's block
				char w[4] = {(char)v, (char)(v >> 8), (char)(v >> 16), (char)(v >> 24)};
				nbuf_put(&pc->sizes, w, 4);
			}
		}
		h2_fh(&pc->out, 0, H2_SETTINGS, H2F_ACK, 0);
		break;
	case H2_PING:
		if (!(flags & H2F_ACK) && len == 8) {
			h2_fh(&pc->out, 8, H2_PING, H2F_ACK, 0);
			nbuf_put(&pc->out, (const char *)p, 8);
		}
		break;
	case H2_WINUP:
		if (len == 4) {
			uint32_t inc = h2_get32(p) & 0x7fffffff;
			if (sid == 0) {
				pc->win += inc;
			} else if (sid == q->id) {
				q->swin += inc;
			}
		}
		break;
	case H2_GOAWAY:
		if (len >= 8) {
			pc->goaway = 1;
			pc->goaway_last = h2_get32(p) & 0x7fffffff;
			pc->goaway_code = h2_get32(p + 4);
			free(pc->goaway_debug);
			pc->goaway_debug = cstr_of((const char *)p + 8, len - 8);
			if (pc->goaway_last < q->id && !q->seen) {
				// the server will not answer this stream: another
				// connection will
				q->refused = 1;
				snprintf(why, wn, "http2: server sent GOAWAY");
				return 0;
			}
		}
		break;
	case H2_RST:
		if (sid == q->id && len == 4) {
			uint32_t code = h2_get32(p);
			if (code == H2E_REFUSED && !q->headers) {
				q->refused = 1;
			}
			if (code == H2E_PROTOCOL) {
				pc->broken = 1;
			}
			char nb[40];
			snprintf(why, wn, "stream error: stream ID %u; %s; received from peer", (unsigned)q->id, h2_err_name(code, nb, sizeof nb));
			return 0;
		}
		break;
	case H2_HEADERS:
	case H2_CONT: {
		const uint8_t *d = p;
		int64_t dn = len;
		if (type == H2_HEADERS) {
			if (flags & H2F_PADDED) {
				if (dn < 1 || p[0] > dn - 1) {
					pc->broken = 1;
					snprintf(why, wn, "connection error: PROTOCOL_ERROR");
					return 0;
				}
				dn -= 1 + p[0];
				d++;
			}
			if (flags & H2F_PRIORITY) {
				if (dn < 5) {
					pc->broken = 1;
					snprintf(why, wn, "connection error: PROTOCOL_ERROR");
					return 0;
				}
				d += 5;
				dn -= 5;
			}
			q->hb.n = 0;
			q->hb_flags = flags;
			q->hb_sid = sid;
		}
		nbuf_put(&q->hb, (const char *)d, dn);
		if (q->hb.n > (10 << 20)) {
			pc->broken = 1;
			snprintf(why, wn, "http2: response header list larger than advertised limit");
			return 0;
		}
		if (!(flags & H2F_END_HEADERS)) {
			q->hb_on = 1;
			break;
		}
		q->hb_on = 0;
		q->dec = 1; // Xo decodes it (xo_cl_h2_next returns)
		break;
	}
	case H2_DATA: {
		const uint8_t *d = p;
		int64_t dn = len;
		if (flags & H2F_PADDED) {
			if (dn < 1 || p[0] > dn - 1) {
				pc->broken = 1;
				snprintf(why, wn, "connection error: PROTOCOL_ERROR");
				return 0;
			}
			dn -= 1 + p[0];
			d++;
		}
		if (len > 0) {
			pc->in.avail -= len;
			int64_t u = h2_inflow_add(&pc->in, len);
			if (u > 0) {
				h2_put_winup(&pc->out, 0, (uint32_t)u);
			}
		}
		if (sid != q->id) {
			break; // an old stream's
		}
		if (!q->headers) {
			pc->broken = 1;
			snprintf(why, wn, "protocol error: received DATA before a HEADERS frame");
			return 0;
		}
		if (strcmp(method, "HEAD") != 0) {
			nbuf_put(&r->body, (const char *)d, dn);
		}
		if (len > 0 && !(flags & H2F_END_STREAM)) {
			q->sin.avail -= len;
			int64_t u = h2_inflow_add(&q->sin, len);
			if (u > 0) {
				h2_put_winup(&pc->out, q->id, (uint32_t)u);
			}
		}
		if (flags & H2F_END_STREAM) {
			q->ended = 1;
		}
		break;
	}
	default:
		break;
	}
	return 1;
}

// An exchange on HTTP/2 connection pc (http2 Transport.RoundTrip), which
// Xo drives (http1.xo h2_exchange) since its header blocks are Xo's: C
// never calls compiled Xo (decision 0180), so it stops where Xo encodes
// the request's block (xo_cl_h2_send) and at each response block Xo
// decodes (xo_cl_h2_next, xo_cl_h2_fields), then xo_cl_h2_end puts the
// connection back and sets the response or the failure. A failure after
// the response header came (reading the body) has why starting with \1:
// net/http reports it from the body's Read, without the method and URL.
struct h2x {
	h2cc *pc;
	int pooled; // pc came from the pool (a failure before an answer is retried)
	h2rq q;
	char *method;
	char *body;
	int64_t bn, off; // the body and how much of it went out
	int close;       // Connection: close (pc is not reused)
	int res;         // CC_OK while the exchange goes on, else how it ended
	char why[400];
};

// h2x_start sets r up for the exchange of the request on pc (the method,
// the body, the deadline, the header: names canonical, each once).
static void h2x_start(xo_cresp *r, h2cc *pc, int pooled, const char *method, const char *body, int64_t bn, int64_t when, hlist *hdr) {
	h2x *x = calloc(1, sizeof(h2x));
	char *b = malloc(bn > 0 ? (size_t)bn : 1);
	if (!x || !b) {
		xo_oom();
	}
	memcpy(b, body, (size_t)bn);
	x->pc = pc;
	x->pooled = pooled;
	x->method = strdup(method);
	x->body = b;
	x->bn = bn;
	int64_t ci = hl_find(hdr, "Connection");
	x->close = ci >= 0 && hs_eq(hdr->a[ci].v, "close");
	x->res = CC_OK;
	r->h2 = x;
	pc->c.when = when;
	tls_set_when(pc->c.tls, when);
}

// h2x_free frees x; an exchange Xo did not end drops its connection.
static void h2x_free(h2x *x) {
	if (!x) {
		return;
	}
	h2cc_free(x->pc);
	hl_free(&x->q.fields);
	free(x->q.hb.p);
	free(x->method);
	free(x->body);
	free(x);
}

int64_t xo_cl_h2(xo_cresp *r) { return r->h2 != NULL; }

int64_t xo_cl_retry(xo_cresp *r) { return r->retry; }

// xo_cl_h2_take moves the connection's HPACK state out to Xo: a list of
// one, empty before the connection's first request.
xo_rlist *xo_cl_h2_take(xo_cresp *r) {
	h2cc *pc = r->h2->pc;
	xo_rlist *l = xo_rl_new(4, 8);
	if (pc->hp) {
		((xo_any **)rl_data(l))[0] = pc->hp;
		l->len = 1;
		pc->hp = NULL;
	}
	return l;
}

// xo_cl_h2_sizes is the server's SETTINGS_HEADER_TABLE_SIZE values since
// the connection's last request (4 bytes little endian each), for Xo's
// encoder.
xo_rstr *xo_cl_h2_sizes(xo_cresp *r, int64_t *len) {
	h2cc *pc = r->h2->pc;
	xo_rstr *s = nstr(pc->sizes.p ? pc->sizes.p : "", pc->sizes.n, len);
	pc->sizes.n = 0;
	return s;
}

// xo_cl_h2_send sends the HEADERS of the request with the block Xo
// encoded (and as much of the body as the windows allow, in
// xo_cl_h2_next), or fails the request with why (checkConnHeaders).
void xo_cl_h2_send(xo_cresp *r, const char *blk, int64_t bn, const char *why, int64_t wn) {
	h2x *x = r->h2;
	h2cc *pc = x->pc;
	h2rq *q = &x->q;
	if (wn > 0) {
		snprintf(x->why, sizeof x->why, "%.*s", (int)wn, why);
		x->res = CC_ERR;
		return;
	}
	if (x->close) {
		pc->broken = 1; // not reused after this request
	}
	q->id = pc->next_id;
	pc->next_id += 2;
	q->swin = pc->peer_win;
	q->sin.avail = (int64_t)4 << 20;
	h2_put_block(&pc->out, q->id, blk, bn, x->bn == 0, pc->peer_frame);
	x->res = cc_send(&pc->c, pc->out.p, pc->out.n);
	pc->out.n = 0;
	r->body.n = 0;
}

// xo_cl_h2_next runs the exchange: 1 when a response header block waits
// for Xo's decoder (xo_cl_h2_block, xo_cl_h2_fields), 0 when the
// exchange is over (xo_cl_h2_end).
int64_t xo_cl_h2_next(xo_cresp *r) {
	h2x *x = r->h2;
	h2cc *pc = x->pc;
	h2rq *q = &x->q;
	while (x->res == CC_OK && !q->ended) {
		// the body, as the windows allow
		while (x->off < x->bn && !q->ended) {
			int64_t k = x->bn - x->off;
			if (k > pc->peer_frame) {
				k = pc->peer_frame;
			}
			if (k > q->swin) {
				k = q->swin;
			}
			if (k > pc->win) {
				k = pc->win;
			}
			if (k <= 0) {
				break;
			}
			h2_fh(&pc->out, (uint32_t)k, H2_DATA, x->off + k == x->bn ? H2F_END_STREAM : 0, q->id);
			nbuf_put(&pc->out, x->body + x->off, k);
			x->off += k;
			q->swin -= k;
			pc->win -= k;
			if (pc->out.n >= (64 << 10)) {
				break;
			}
		}
		if (pc->out.n > 0) {
			x->res = cc_send(&pc->c, pc->out.p, pc->out.n);
			pc->out.n = 0;
			if (x->res != CC_OK) {
				break;
			}
			if (x->off < x->bn && q->swin > 0 && pc->win > 0) {
				continue;
			}
		}
		const uint8_t *h;
		int f = h2cc_frame(&pc->c, &h);
		if (f == CC_EOF) {
			pc->broken = 1;
			if (pc->goaway) {
				char nb[40];
				nbuf gq = {0};
				h_go_quote(&gq, pc->goaway_debug ? pc->goaway_debug : "", pc->goaway_debug ? (int64_t)strlen(pc->goaway_debug) : 0);
				nbuf_byte(&gq, 0);
				snprintf(x->why, sizeof x->why, "%shttp2: server sent GOAWAY and closed the connection; LastStreamID=%u, ErrCode=%s, debug=%s",
				         q->headers ? "\1" : "", (unsigned)pc->goaway_last, h2_err_name(pc->goaway_code, nb, sizeof nb), gq.p);
				free(gq.p);
			} else {
				snprintf(x->why, sizeof x->why, "%sunexpected EOF", q->headers ? "\1" : "");
			}
			x->res = CC_ERR;
			break;
		}
		if (f == CC_ERR + 100) {
			pc->broken = 1;
			snprintf(x->why, sizeof x->why, "%shttp2: frame too large", q->headers ? "\1" : "");
			x->res = CC_ERR;
			break;
		}
		if (f != CC_OK) {
			x->res = f;
			break;
		}
		int had = q->headers;
		char m[300] = "";
		if (!h2cc_process(pc, q, h, r, x->method, m, sizeof m)) {
			snprintf(x->why, sizeof x->why, "%s%s", had ? "\1" : "", m);
			x->res = CC_ERR;
			break;
		}
		if (q->dec) {
			return 1;
		}
		if (pc->out.n > 0) {
			x->res = cc_send(&pc->c, pc->out.p, pc->out.n);
			pc->out.n = 0;
		}
	}
	return 0;
}

// xo_cl_h2_block is the response header block Xo decodes.
xo_rstr *xo_cl_h2_block(xo_cresp *r, int64_t *len) {
	h2rq *q = &r->h2->q;
	return nstr(q->hb.p ? q->hb.p : "", q->hb.n, len);
}

// xo_cl_h2_fields takes the fields Xo decoded from the block (ok: 0 a
// decoding error; d and lens as xo_h2_fields'); xo_cl_h2_next goes on.
void xo_cl_h2_fields(xo_cresp *r, int64_t ok, const char *d, int64_t dn, xo_rlist *lens) {
	h2x *x = r->h2;
	h2rq *q = &x->q;
	q->dec = 0;
	int had = q->headers;
	char m[300] = "";
	if (!h2rq_headers(x->pc, q, r, (int)ok, d, dn, lens, m, sizeof m)) {
		snprintf(x->why, sizeof x->why, "%s%s", had ? "\1" : "", m);
		x->res = CC_ERR;
	}
}

// xo_cl_h2_end ends the exchange: the connection keeps Xo's HPACK state
// st and goes back to the pool (or closes), and r gets the failure. A
// request the server did not take on a pooled connection (it closed it,
// a GOAWAY that excludes it, REFUSED_STREAM) sets cl_retry instead: Xo
// sends it again on a new connection, as Transport retries it.
void xo_cl_h2_end(xo_cresp *r, xo_any *st) {
	h2x *x = r->h2;
	h2cc *pc = x->pc;
	h2rq *q = &x->q;
	h_retain(st);
	if (pc->hp) {
		h_release(pc->hp);
	}
	pc->hp = st;
	int res = x->res;
	if (res != CC_OK && res != CC_ERR) {
		pc->broken = 1; // a timeout or cancellation left the stream open
	}
	int retry = q->refused || (res != CC_OK && !q->seen && res == CC_ERR && pc->broken && !q->headers);
	if (res == CC_OK && x->off < x->bn) {
		// the response came before the whole body: the stream is reset
		pc->broken = 1;
	}
	x->pc = NULL;
	if (x->pooled && retry) {
		h2cc_free(pc);
		r->status = 0;
		hl_free(&r->hdr);
		r->body.n = 0;
		r->retry = 1;
	} else {
		char *alert = NULL;
		if (!x->pooled && res != CC_OK && res != CC_TIMEOUT && res != CC_CANCELED && !x->why[0]) {
			const char *ta = tls_alert(pc->c.tls);
			alert = ta ? strdup(ta) : NULL;
		}
		h2_pool_put(pc);
		cl_finish(r, res, x->why, alert);
		free(alert);
	}
	r->h2 = NULL;
	h2x_free(x);
}
