// xo_testing.c: the test world of native test binaries (`xo test
// --no-gc`, decision 0083), included by xo_std.c. A test binary runs one
// test (XO_TEST names its index) against fakes, as std/rt's test world
// does on the Go backend:
//
//   - The virtual clock starts at 2026-01-01T00:00:00Z. Timers (sleeps,
//     select's `after`, with_timeout, the grace period, signal_after) go
//     on the event loop's heap but no poller runs: when every context is
//     blocked (the deadlock check, xo_rt.c), tw_kick moves the clock to
//     the earliest timer and fires every timer due then. With no timer
//     left the test deadlocked (XO-F001).
//   - Rand is seeded (Go's math/rand/v2 PCG with the seeds std/rt uses),
//     the environment starts empty, standard output and error are
//     captured (FakeStdio.output, errors), the file system is empty, log
//     records are kept for testing.logs() and echoed to the harness.
//   - FakeNet.listen gives an in memory listener; http.server serves it
//     by registering its router here, and the http client of the test
//     reaches a served router in process (std/native/http.xo). Handlers
//     and failures registered with FakeNet.handle and fail are kept here
//     too.
//   - testing.run_main runs main below a node of its own: the stop state
//     of xo_rt.c (two phase shutdown) is reset for it, its root is that
//     node, and the signals of FakeProc.signal_after are virtual timers.
//
// The harness reads the outcome from the exit status and standard output
// (the real one: the program's own output is captured): XO-TEST-FAIL and
// the message for a failure (exit 1), XO-TEST-LOG lines for log records.
//
// Static helpers are prefixed tw_ (gotchas/llrt-static-names-in-tests.md).

#define TW_EPOCH ((int64_t)1767225600 * 1000000000) // 2026-01-01T00:00:00Z

typedef struct {
	int64_t level;
	char *msg;
	int nf;
	char **keys, **vals;
} tw_rec;

// an in memory listener
typedef struct tw_ml {
	xo_listener *l; // not counted: the listener frees its tw_ml
	char *alias;    // localhost:<port> for a listener on 127.0.0.1
	xo_any *router; // served: http.Router as an Any, or NULL
	int closed;
	int inflight;
	int draining;
	xo_wl waiters; // accept, clients waiting for serve, drain
	struct tw_ml *next;
} tw_ml;

// a handler or a failure of FakeNet
typedef struct {
	char *method, *prefix, *path;
	xo_any *v; // the handler closure, or the ClientErr
	int once;
} tw_fake;

static struct {
	xo_lock mu;
	char **ekeys, **evals;
	int en;
	char **args;
	int nargs;
	nbuf out, err;
	int64_t *sleeps;
	int nsleeps;
	int64_t *signals; // signal_after before run_main, from its start
	int nsignals;
	uint64_t rng_hi, rng_lo;
	tw_rec *recs;
	int nrec;
	tw_ml *mls;
	int next_port;
	tw_fake *handlers, *fails;
	int nhandlers, nfails;
	xo_any **reqs;
	int nreqs;
	xo_node *outer; // the node run_main restores
	xo_node *main_node;
	int64_t exit_code;
	int has_exit;
	int exiting; // os.proc.exit unwinds main to testing.run_main
} tws;

static void *tw_grow(void *p, int n, size_t size) {
	// room for element n (doubling at powers of two)
	if (n == 0 || (n & (n - 1)) == 0) {
		p = realloc(p, (size_t)(n == 0 ? 4 : 2 * n) * size);
		if (!p) {
			xo_oom();
		}
	}
	return p;
}

static char *tw_strdup(const char *p, int64_t n) { return cstr_of(p, n); }

__attribute__((unused)) static void tw_init(void) {
	tw.on = 1;
	tw.vnow = TW_EPOCH;
	int64_t seed = 1;
	const char *s = getenv("XO_TEST_SEED");
	if (s && *s) {
		seed = strtoll(s, NULL, 10);
	}
	tws.rng_hi = (uint64_t)seed;
	tws.rng_lo = (uint64_t)seed ^ 0x9e3779b97f4a7c15ULL;
	signal(SIGPIPE, SIG_IGN);
}

// xo_test_index is the test to run (XO_TEST).
int64_t xo_test_index(void) {
	const char *s = getenv("XO_TEST");
	return s ? strtoll(s, NULL, 10) : -1;
}

// xo_test_on reports whether the program runs in the test world.
int64_t xo_test_on(void) { return tw.on; }

// tw_report writes a line for the harness on the real standard output.
static void tw_report(const char *tag, const char *p, int64_t n) {
	write_all(1, tag, strlen(tag));
	write_all(1, p, (size_t)n);
	write_all(1, "\n", 1);
}

// xo_test_fail fails the test with msg (a failed expect, an error
// returned by the test body): it does not return.
XO_NORETURN void xo_test_fail(const char *p, int64_t n) {
	tw_report("XO-TEST-FAIL\n", p, n);
	_exit(1);
}

// tw_exit is os.proc.exit in a test (std.md 8.4): the code is recorded
// (FakeProc.exit_code). Outside testing.run_main the test ends and
// passes, as on the Go backend. Inside it, main unwinds back to run_main
// (decision 0108): its root is canceled and this context unwinds from
// here (generated code checks after the call), and xo_test_main_end
// gives the code as main's status.
__attribute__((unused)) static void tw_exit(int64_t code) {
	lk(&tws.mu);
	tws.exit_code = code;
	tws.has_exit = 1;
	unlk(&tws.mu);
	if (__atomic_load_n(&tw.in_main, __ATOMIC_ACQUIRE) && tws.main_node) {
		__atomic_store_n(&tws.exiting, 1, __ATOMIC_RELEASE);
		node_cancel(tws.main_node);
		ctx()->unwinding = 1;
		return;
	}
	_exit(0);
}

// The virtual clock

// tw_kick runs after a deadlock check found every context blocked: the
// clock moves to the earliest timer and every timer due then fires; with
// none the program is deadlocked. When the timers that fired woke no
// context (a FakeProc.signal_after signal whose stop wakes nobody), every
// context still waits and none would check again: the check runs again
// here, and the clock moves on.
static int tw_kick_once(void);

static void tw_kick(void) {
	while (tw_kick_once()) {
		lk(&pool.mu);
		dl_check_locked();
		unlk(&pool.mu);
	}
}

// tw_kick_once is one move of the clock; 1 when only callback timers
// fired (no park was signaled by the clock itself).
static int tw_kick_once(void) {
	if (!tw.on || !__atomic_exchange_n(&tw.vpending, 0, __ATOMIC_ACQ_REL)) {
		return 0;
	}
	int parks = 0;
	pthread_mutex_lock(&ev.mu);
	if (ev.n == 0) {
		pthread_mutex_unlock(&ev.mu);
		// std/rt's test world names the timers too
		static const char m[] = "fault: XO-F001: deadlock: every task is blocked and no timer is pending\n";
		write_all(2, m, sizeof m - 1);
		_exit(70);
	}
	int64_t at = ev.heap[0]->when;
	if (at > __atomic_load_n(&tw.vnow, __ATOMIC_ACQUIRE)) {
		__atomic_store_n(&tw.vnow, at, __ATOMIC_RELEASE);
	}
	while (ev.n > 0 && ev.heap[0]->when <= at) {
		xo_timer *t = ev.heap[0];
		th_remove(t);
		__atomic_store_n(&t->fired, 1, __ATOMIC_RELEASE);
		if (t->park) {
			park_signal(t->park);
			parks++;
		} else {
			t->fn(t);
		}
	}
	pthread_mutex_unlock(&ev.mu);
	return parks == 0;
}

__attribute__((unused)) static void tw_sleep_record(int64_t d) {
	lk(&tws.mu);
	tws.sleeps = tw_grow(tws.sleeps, tws.nsleeps, sizeof(int64_t));
	tws.sleeps[tws.nsleeps++] = d;
	unlk(&tws.mu);
}

// xo_test_sleeps is FakeClock.sleeps(): every sleep asked for, in order.
xo_rlist *xo_test_sleeps(void) {
	lk(&tws.mu);
	xo_rlist *l = xo_rl_new(tws.nsleeps < 4 ? 4 : tws.nsleeps, 8);
	memcpy(rl_data(l), tws.sleeps, (size_t)tws.nsleeps * 8);
	l->len = tws.nsleeps;
	unlk(&tws.mu);
	return l;
}

// xo_test_advance is FakeClock.advance(d): the timers due within d fire
// in order, the woken contexts running (until every one waits again)
// after each instant; then the clock reads now + d. It waits like a sleep
// that is not a cancellation point and is not listed by sleeps().
void xo_test_advance(int64_t d) {
	xo_ctx *c = ctx();
	if (d < 0) {
		d = 0;
	}
	xo_timer t = {.when = deadline(d), .idx = -1, .park = &c->park};
	park_reset(&c->park);
	timer_add(&t);
	while (!__atomic_load_n(&t.fired, __ATOMIC_ACQUIRE)) {
		park_wait_n(c, 0, 1);
	}
	timer_del(&t);
	// the contexts woken at the last instant (with this one) run until
	// every one waits again before advance returns, as std/rt's
	// World.Advance: a timer due now fires at the next move of the clock,
	// which comes only when every context waits
	xo_timer q = {.when = deadline(0), .idx = -1, .park = &c->park};
	park_reset(&c->park);
	timer_add(&q);
	while (!__atomic_load_n(&q.fired, __ATOMIC_ACQUIRE)) {
		park_wait_n(c, 0, 1);
	}
	timer_del(&q);
}

// xo_test_clock_set is FakeClock.set(t).
void xo_test_clock_set(int64_t t) { __atomic_store_n(&tw.vnow, t, __ATOMIC_RELEASE); }

// Seeded random numbers: Go's math/rand/v2 PCG (std/rt's seededRand)

__attribute__((unused)) static uint64_t tw_rand_u64(void) {
	lk(&tws.mu);
	const uint64_t mulHi = 2549297995355413924ULL, mulLo = 4865540595714422341ULL;
	const uint64_t incHi = 6364136223846793005ULL, incLo = 1442695040888963407ULL;
	unsigned __int128 m = (unsigned __int128)tws.rng_lo * mulLo;
	uint64_t hi = (uint64_t)(m >> 64), lo = (uint64_t)m;
	hi += tws.rng_hi * mulLo + tws.rng_lo * mulHi;
	uint64_t lo2 = lo + incLo;
	uint64_t c = lo2 < lo;
	hi = hi + incHi + c;
	tws.rng_lo = lo2;
	tws.rng_hi = hi;
	unlk(&tws.mu);
	const uint64_t cheapMul = 0xda942042e4dd58b5ULL;
	hi ^= hi >> 32;
	hi *= cheapMul;
	hi ^= hi >> 48;
	hi *= (lo2 | 1);
	return hi;
}

// ---------------------------------------------------------------------------
// Generated values: property tests (`test "x" for v: T`, core 9.2) and
// testing.gen (std.md 8.5), decision 0120. The same choices as std/rt's
// GenRand (gen.go): Go's math/rand/v2 PCG seeded (seed, seed ^
// 0x2545f4914f6cdd1d), every choice recorded, so a test binary makes the
// Go backend's values for a seed, and a failing case can be shrunk by
// replaying edited choices (cmd/xo nativetest.go). internal/ssa generates
// the values of compound types (build_gen.go) on these primitives.

typedef struct {
	uint64_t hi, lo;
	int depth;
	int small; // contract fuzzing's small numbers (std/rt's Small)
	uint64_t *rec;
	int64_t nrec, crec;
	uint64_t *replay;
	int64_t nreplay, pos;
	int replaying;
} xo_gen;

static uint64_t gen_pcg(xo_gen *g) {
	const uint64_t mulHi = 2549297995355413924ULL, mulLo = 4865540595714422341ULL;
	const uint64_t incHi = 6364136223846793005ULL, incLo = 1442695040888963407ULL;
	unsigned __int128 m = (unsigned __int128)g->lo * mulLo;
	uint64_t hi = (uint64_t)(m >> 64), lo = (uint64_t)m;
	hi += g->hi * mulLo + g->lo * mulHi;
	uint64_t lo2 = lo + incLo;
	uint64_t c = lo2 < lo;
	hi = hi + incHi + c;
	g->lo = lo2;
	g->hi = hi;
	const uint64_t cheapMul = 0xda942042e4dd58b5ULL;
	hi ^= hi >> 32;
	hi *= cheapMul;
	hi ^= hi >> 48;
	hi *= (lo2 | 1);
	return hi;
}

// gen_u64n is Go's (*rand.Rand).Uint64N: a mask for powers of two, else
// Lemire's multiply with rejection.
static uint64_t gen_u64n(xo_gen *g, uint64_t n) {
	if ((n & (n - 1)) == 0) {
		return gen_pcg(g) & (n - 1);
	}
	unsigned __int128 p = (unsigned __int128)gen_pcg(g) * n;
	uint64_t hi = (uint64_t)(p >> 64), lo = (uint64_t)p;
	if (lo < n) {
		uint64_t thresh = (0 - n) % n;
		while (lo < thresh) {
			p = (unsigned __int128)gen_pcg(g) * n;
			hi = (uint64_t)(p >> 64);
			lo = (uint64_t)p;
		}
	}
	return hi;
}

// gen_draw is a choice in [0, n), or any 64 bit value when n is 0.
static uint64_t gen_draw(xo_gen *g, uint64_t n) {
	uint64_t v;
	if (g->replaying) {
		v = g->pos < g->nreplay ? g->replay[g->pos] : 0;
		g->pos++;
		if (n > 0) {
			v %= n;
		}
	} else if (n == 0) {
		v = gen_pcg(g);
	} else {
		v = gen_u64n(g, n);
	}
	if (g->nrec == g->crec) {
		g->crec = g->crec ? 2 * g->crec : 64;
		g->rec = realloc(g->rec, (size_t)g->crec * sizeof(uint64_t));
		if (!g->rec) {
			xo_oom();
		}
	}
	g->rec[g->nrec++] = v;
	return v;
}

static xo_gen *gen_of(int64_t h) { return (xo_gen *)(uintptr_t)h; }

static xo_gen *gen_alloc(void) {
	xo_gen *g = calloc(1, sizeof(xo_gen));
	if (!g) {
		xo_oom();
	}
	return g;
}

// xo_gen_new is a generator seeded with seed (testing.gen, std/rt's
// NewGenRand).
int64_t xo_gen_new(int64_t seed) {
	xo_gen *g = gen_alloc();
	g->hi = (uint64_t)seed;
	g->lo = (uint64_t)seed ^ 0x2545f4914f6cdd1dULL;
	return (int64_t)(uintptr_t)g;
}

// xo_gen_free frees generator h.
void xo_gen_free(int64_t h) {
	xo_gen *g = gen_of(h);
	free(g->rec);
	free(g->replay);
	free(g);
}

// gen_choices parses "c1,c2,..." (decimal) into a new array.
static uint64_t *gen_choices(const char *s, int64_t *n) {
	int64_t cap = 16, k = 0;
	uint64_t *out = malloc((size_t)cap * sizeof(uint64_t));
	if (!out) {
		xo_oom();
	}
	while (s && *s) {
		char *end;
		uint64_t v = strtoull(s, &end, 10);
		if (end == s) {
			break;
		}
		if (k == cap) {
			cap *= 2;
			out = realloc(out, (size_t)cap * sizeof(uint64_t));
			if (!out) {
				xo_oom();
			}
		}
		out[k++] = v;
		s = *end == ',' ? end + 1 : end;
	}
	*n = k;
	return out;
}

// xo_gen_prop is the generator of a property test's case: the harness
// sets XO_PROP_REPLAY (choices to replay, shrinking) or XO_PROP_SEED (the
// case's seed: std/rt's seed*1000003 + case).
int64_t xo_gen_prop(void) {
	xo_gen *g = gen_alloc();
	const char *r = getenv("XO_PROP_REPLAY");
	if (r) {
		g->replaying = 1;
		g->replay = gen_choices(r, &g->nreplay);
		return (int64_t)(uintptr_t)g;
	}
	const char *s = getenv("XO_PROP_SEED");
	uint64_t seed = s ? strtoull(s, NULL, 10) : 0;
	g->hi = seed;
	g->lo = seed ^ 0x2545f4914f6cdd1dULL;
	return (int64_t)(uintptr_t)g;
}

// gen_shorter is std/rt's shortlexLess(a, b).
static int gen_shorter(const uint64_t *a, int64_t an, const uint64_t *b, int64_t bn) {
	if (an != bn) {
		return an < bn;
	}
	for (int64_t i = 0; i < an; i++) {
		if (a[i] != b[i]) {
			return a[i] < b[i];
		}
	}
	return 0;
}

// xo_prop_report hands a case's choices and input text (`name = value`
// lines) to the harness before the body runs. While shrinking, the
// harness passes the best choices so far (XO_PROP_BEST): a case whose
// choices are not smaller is not run (exit 3), as std/rt skips it.
void xo_prop_report(int64_t h, const char *in, int64_t inn) {
	xo_gen *g = gen_of(h);
	nbuf o = {0};
	nbuf_str(&o, "XO-PROP-CHOICES ");
	for (int64_t i = 0; i < g->nrec; i++) {
		char num[32];
		snprintf(num, sizeof num, i ? ",%llu" : "%llu", (unsigned long long)g->rec[i]);
		nbuf_str(&o, num);
	}
	nbuf_byte(&o, '\n');
	int64_t start = 0;
	for (int64_t i = 0; i <= inn; i++) {
		if (i == inn || in[i] == '\n') {
			if (i > start || i < inn) {
				nbuf_str(&o, "XO-PROP-INPUT ");
				nbuf_put(&o, in + start, i - start);
				nbuf_byte(&o, '\n');
			}
			start = i + 1;
		}
	}
	write_all(1, o.p, (size_t)o.n);
	free(o.p);
	const char *best = getenv("XO_PROP_BEST");
	if (best) {
		int64_t bn;
		uint64_t *b = gen_choices(best, &bn);
		int smaller = gen_shorter(g->rec, g->nrec, b, bn);
		free(b);
		if (!smaller) {
			write_all(1, "XO-PROP-NOT-SMALLER\n", 20);
			_exit(3);
		}
	}
}

// xo_gen_intn is GenRand.Intn: a choice in [0, n), no choice for n <= 1.
int64_t xo_gen_intn(int64_t h, int64_t n) {
	if (n <= 1) {
		return 0;
	}
	return (int64_t)gen_draw(gen_of(h), (uint64_t)n);
}

// xo_gen_enter and xo_gen_leave bracket a nested value; xo_gen_deep
// tells when nested values stop growing (GenRand.Deep).
void xo_gen_enter(int64_t h) { gen_of(h)->depth++; }

void xo_gen_leave(int64_t h) { gen_of(h)->depth--; }

int64_t xo_gen_deep(int64_t h) { return gen_of(h)->depth >= 4; }

// xo_gen_size is a collection's length: small, shrinking with depth.
int64_t xo_gen_size(int64_t h) {
	xo_gen *g = gen_of(h);
	if (g->depth >= 4) {
		return 0;
	}
	return xo_gen_intn(h, 6 - g->depth);
}

static int64_t gen_zigzag(uint64_t v) { return (int64_t)(v >> 1) ^ -(int64_t)(v & 1); }

// xo_gen_int is std/rt's genInt: small values and edges preferred.
int64_t xo_gen_int(int64_t h) {
	xo_gen *g = gen_of(h);
	if (g->small) {
		if (gen_draw(g, 5) < 4) {
			return gen_zigzag(gen_draw(g, 2001));
		}
		static const int64_t e[] = {0, 1, -1};
		return e[xo_gen_intn(h, 3)];
	}
	uint64_t c = gen_draw(g, 10);
	if (c < 7) {
		return gen_zigzag(gen_draw(g, 201));
	}
	if (c == 7) {
		static const int64_t e[] = {0, 1, -1, INT64_MAX, INT64_MIN};
		return e[xo_gen_intn(h, 5)];
	}
	return gen_zigzag(gen_draw(g, 0));
}

static int64_t gen_abs(int64_t n) { return n < 0 ? (n == INT64_MIN ? INT64_MAX : -n) : n; }

// xo_gen_sized is a sized integer (std/rt's Gen): kind 1 Int32, 2 Int16,
// 3 Int8, 4 UInt64, 5 UInt32, 6 UInt16, 7 UInt8, in the Int word.
int64_t xo_gen_sized(int64_t h, int64_t kind) {
	int64_t v = xo_gen_int(h);
	switch (kind) {
	case 1:
		return (int32_t)(v % ((int64_t)1 << 31));
	case 2:
		return (int16_t)(v % ((int64_t)1 << 15));
	case 3:
		return (int8_t)(v % ((int64_t)1 << 7));
	case 4:
		return gen_abs(v);
	case 5:
		return (int64_t)(uint32_t)gen_abs(v);
	case 6:
		return (int64_t)(uint16_t)gen_abs(v);
	default:
		return (int64_t)(uint8_t)gen_abs(v);
	}
}

// xo_gen_float is std/rt's genFloat: small values preferred.
double xo_gen_float(int64_t h) {
	xo_gen *g = gen_of(h);
	uint64_t c = gen_draw(g, 8);
	if (c < 6) {
		return (double)gen_zigzag(gen_draw(g, 2001)) / 8;
	}
	if (c == 6) {
		static const double e[] = {0, -0.5, 1, 1e300, -1e-300};
		return e[xo_gen_intn(h, 5)];
	}
	return ((double)gen_draw(g, (uint64_t)1 << 53) / (double)((uint64_t)1 << 53) - 0.5) * 1000;
}

// the runes of generated strings (std/rt's alphabet)
static const int32_t gen_alpha[] = {'a', 'b', 'c', 'x', 'y', 'z', 'A', 'B', 'C', '0', '1', '9', ' ', '_', '-', 0xe9, 0x3bb, 0x1f600};
#define GEN_NALPHA ((int64_t)(sizeof gen_alpha / sizeof gen_alpha[0]))

// xo_gen_rune is a rune of the alphabet.
int64_t xo_gen_rune(int64_t h) { return gen_alpha[xo_gen_intn(h, GEN_NALPHA)]; }

// xo_gen_str is std/rt's genStr: up to 8 runes of the alphabet (Bytes
// are its UTF-8).
xo_rstr *xo_gen_str(int64_t h, int64_t *len) {
	int64_t n = xo_gen_intn(h, 9);
	nbuf o = {0};
	for (int64_t i = 0; i < n; i++) {
		char u[4];
		int k = n_utf8_encode(u, gen_alpha[xo_gen_intn(h, GEN_NALPHA)]);
		nbuf_put(&o, u, k);
	}
	xo_rstr *s = nstr(o.p ? o.p : "", o.n, len);
	free(o.p);
	return s;
}

// xo_gen_duration and xo_gen_time are std/rt's: milliseconds of genInt,
// seconds of genInt from 2026-01-01 (wrapping as Go's int64 does).
int64_t xo_gen_duration(int64_t h) { return (int64_t)((uint64_t)xo_gen_int(h) * 1000000ULL); }

int64_t xo_gen_time(int64_t h) { return (int64_t)(1767225600000000000ULL + (uint64_t)xo_gen_int(h) * 1000000000ULL); }

// Standard streams and the environment

static void tw_write(int err, const char *p, int64_t n) {
	lk(&tws.mu);
	nbuf_put(err ? &tws.err : &tws.out, p, n);
	unlk(&tws.mu);
}

// xo_test_output is FakeStdio.output() (which 1) or errors() (2).
xo_rstr *xo_test_output(int64_t which, int64_t *len) {
	lk(&tws.mu);
	nbuf *b = which == 2 ? &tws.err : &tws.out;
	xo_rstr *s = nstr(b->p ? b->p : "", b->n, len);
	unlk(&tws.mu);
	return s;
}

// xo_test_eprint writes to the fake standard error.
void xo_test_eprint(const char *p, int64_t n) { tw_write(1, p, n); }

__attribute__((unused)) static xo_rlist *tw_env_get(const char *p, int64_t n) {
	xo_rlist *l = &empty_list;
	lk(&tws.mu);
	for (int i = tws.en - 1; i >= 0; i--) {
		if ((int64_t)strlen(tws.ekeys[i]) == n && memcmp(tws.ekeys[i], p, (size_t)n) == 0) {
			l = str_list(tws.evals[i], (int64_t)strlen(tws.evals[i]));
			break;
		}
	}
	unlk(&tws.mu);
	return l;
}

// xo_test_env_set is FakeEnv.set(name, value).
void xo_test_env_set(const char *k, int64_t kn, const char *v, int64_t vn) {
	lk(&tws.mu);
	for (int i = 0; i < tws.en; i++) {
		if ((int64_t)strlen(tws.ekeys[i]) == kn && memcmp(tws.ekeys[i], k, (size_t)kn) == 0) {
			free(tws.evals[i]);
			tws.evals[i] = tw_strdup(v, vn);
			unlk(&tws.mu);
			return;
		}
	}
	tws.ekeys = tw_grow(tws.ekeys, tws.en, sizeof(char *));
	tws.evals = tw_grow(tws.evals, tws.en, sizeof(char *));
	tws.ekeys[tws.en] = tw_strdup(k, kn);
	tws.evals[tws.en] = tw_strdup(v, vn);
	tws.en++;
	unlk(&tws.mu);
}

// xo_test_set_args is FakeEnv.set_args(args) (and run_main's args).
void xo_test_set_args(xo_rlist *l) {
	lk(&tws.mu);
	for (int i = 0; i < tws.nargs; i++) {
		free(tws.args[i]);
	}
	free(tws.args);
	tws.args = calloc((size_t)(l->len + 1), sizeof(char *));
	if (!tws.args) {
		xo_oom();
	}
	for (int64_t i = 0; i < l->len; i++) {
		xo_rstr *s = ((xo_rstr **)rl_data(l))[2 * i];
		int64_t n = ((int64_t *)rl_data(l))[2 * i + 1];
		tws.args[i] = tw_strdup(s->data, n);
	}
	tws.nargs = (int)l->len;
	unlk(&tws.mu);
}

__attribute__((unused)) static xo_rlist *tw_env_args(void) {
	lk(&tws.mu);
	xo_rlist *l = n_str_list_new(tws.nargs);
	for (int i = 0; i < tws.nargs; i++) {
		l = n_str_list_add(l, tws.args[i], (int64_t)strlen(tws.args[i]));
	}
	unlk(&tws.mu);
	return l;
}

// xo_test_exit_code is FakeProc.exit_code(): the code os.proc.exit
// recorded, in a list of one, or the empty list.
xo_rlist *xo_test_exit_code(void) {
	xo_rlist *l = xo_rl_new(4, 8);
	lk(&tws.mu);
	if (tws.has_exit) {
		((int64_t *)rl_data(l))[0] = tws.exit_code;
		l->len = 1;
	}
	unlk(&tws.mu);
	return l;
}

// Log records (testing.logs, std.md 8.6)

// xo_test_log keeps a record and echoes its text line to the harness.
void xo_test_log(int64_t level, const char *m, int64_t mn, xo_rlist *keys, xo_rlist *vals, const char *line, int64_t ln) {
	tw_rec r = {.level = level, .msg = tw_strdup(m, mn), .nf = (int)keys->len};
	r.keys = calloc((size_t)r.nf + 1, sizeof(char *));
	r.vals = calloc((size_t)r.nf + 1, sizeof(char *));
	if (!r.keys || !r.vals) {
		xo_oom();
	}
	for (int i = 0; i < r.nf; i++) {
		xo_rstr *k = ((xo_rstr **)rl_data(keys))[2 * i];
		xo_rstr *v = ((xo_rstr **)rl_data(vals))[2 * i];
		r.keys[i] = tw_strdup(k->data, ((int64_t *)rl_data(keys))[2 * i + 1]);
		r.vals[i] = tw_strdup(v->data, ((int64_t *)rl_data(vals))[2 * i + 1]);
	}
	lk(&tws.mu);
	tws.recs = tw_grow(tws.recs, tws.nrec, sizeof(tw_rec));
	tws.recs[tws.nrec++] = r;
	tw_report("XO-TEST-LOG ", line, ln);
	unlk(&tws.mu);
}

int64_t xo_test_log_count(void) {
	lk(&tws.mu);
	int64_t n = tws.nrec;
	unlk(&tws.mu);
	return n;
}

static tw_rec *tw_rec_at(int64_t i) {
	lk(&tws.mu);
	tw_rec *r = i >= 0 && i < tws.nrec ? &tws.recs[i] : NULL;
	unlk(&tws.mu);
	return r;
}

int64_t xo_test_log_level(int64_t i) {
	tw_rec *r = tw_rec_at(i);
	return r ? r->level : 0;
}

xo_rstr *xo_test_log_msg(int64_t i, int64_t *len) {
	tw_rec *r = tw_rec_at(i);
	const char *s = r ? r->msg : "";
	return nstr(s, (int64_t)strlen(s), len);
}

int64_t xo_test_log_nfields(int64_t i) {
	tw_rec *r = tw_rec_at(i);
	return r ? r->nf : 0;
}

xo_rstr *xo_test_log_key(int64_t i, int64_t j, int64_t *len) {
	tw_rec *r = tw_rec_at(i);
	const char *s = r && j >= 0 && j < r->nf ? r->keys[j] : "";
	return nstr(s, (int64_t)strlen(s), len);
}

xo_rstr *xo_test_log_val(int64_t i, int64_t j, int64_t *len) {
	tw_rec *r = tw_rec_at(i);
	const char *s = r && j >= 0 && j < r->nf ? r->vals[j] : "";
	return nstr(s, (int64_t)strlen(s), len);
}

// In memory listeners (std/rt's memListener)

static tw_ml *tw_ml_of(xo_listener *l) { return (tw_ml *)l->mem; }

// tw_listen is FakeNet.listen(addr): ":0" (and port 0) picks the next
// port from 49152.
__attribute__((unused)) static xo_listener *tw_listen(xo_ctx *c, const char *addr) {
	const char *colon = strrchr(addr, ':');
	char host[256] = "127.0.0.1";
	const char *port = colon ? colon + 1 : addr;
	if (colon && colon > addr) {
		size_t hn = (size_t)(colon - addr);
		if (hn >= sizeof host) {
			hn = sizeof host - 1;
		}
		memcpy(host, addr, hn);
		host[hn] = 0;
	}
	char pbuf[32];
	lk(&tws.mu);
	if (*port == 0 || strcmp(port, "0") == 0) {
		tws.next_port++;
		snprintf(pbuf, sizeof pbuf, "%d", 49151 + tws.next_port);
		port = pbuf;
	}
	char full[300];
	snprintf(full, sizeof full, "%s:%s", host, port);
	tw_ml *ml = calloc(1, sizeof(tw_ml));
	if (!ml) {
		xo_oom();
	}
	if (strcmp(host, "127.0.0.1") == 0) {
		char al[300];
		snprintf(al, sizeof al, "localhost:%s", port);
		ml->alias = strdup(al);
	}
	xo_listener *l = xo_alloc(sizeof(xo_listener));
	h_init(l, listener_free);
	l->pd = NULL;
	l->addr = strdup(full);
	l->mem = ml;
	ml->l = l;
	ml->next = tws.mls;
	tws.mls = ml;
	unlk(&tws.mu);
	net_ok(c);
	return l;
}

__attribute__((unused)) static void tw_listener_free(xo_listener *l) {
	tw_ml *ml = tw_ml_of(l);
	lk(&tws.mu);
	for (tw_ml **pp = &tws.mls; *pp; pp = &(*pp)->next) {
		if (*pp == ml) {
			*pp = ml->next;
			break;
		}
	}
	unlk(&tws.mu);
	if (ml->router) {
		h_release(ml->router);
	}
	free(ml->alias);
	free(ml);
}

__attribute__((unused)) static void tw_listener_close(xo_listener *l) {
	tw_ml *ml = tw_ml_of(l);
	lk(&tws.mu);
	ml->closed = 1;
	wl_wake(&ml->waiters);
	unlk(&tws.mu);
}

// tw_listener_accept waits until the listener closes or the context is
// canceled: requests reach a served router in process, never as
// connections.
__attribute__((unused)) static xo_conn *tw_listener_accept(xo_ctx *c, xo_listener *l) {
	tw_ml *ml = tw_ml_of(l);
	lk(&tws.mu);
	while (!ml->closed) {
		if (!wl_wait(c, &tws.mu, &ml->waiters, 1)) {
			unlk(&tws.mu);
			net_fail(c, 5, "");
			return xo_h_zero();
		}
	}
	unlk(&tws.mu);
	net_fail(c, 4, "accept tcp %s: %s", l->addr, closed_text);
	return xo_h_zero();
}

// xo_test_mem_listener reports whether l is in memory.
int64_t xo_test_mem_listener(xo_listener *l) { return l->mem != NULL; }

// xo_test_mem_serve serves l with router r (an Any holding the
// http.Router): clients waiting for it go on.
void xo_test_mem_serve(xo_listener *l, xo_any *r) {
	tw_ml *ml = tw_ml_of(l);
	h_retain(r);
	lk(&tws.mu);
	xo_any *old = ml->router;
	ml->router = r;
	wl_wake(&ml->waiters);
	unlk(&tws.mu);
	if (old) {
		h_release(old);
	}
}

// xo_test_mem_unserve stops serving l.
void xo_test_mem_unserve(xo_listener *l) {
	tw_ml *ml = tw_ml_of(l);
	lk(&tws.mu);
	xo_any *old = ml->router;
	ml->router = NULL;
	wl_wake(&ml->waiters);
	unlk(&tws.mu);
	if (old) {
		h_release(old);
	}
}

// xo_test_mem_drain waits until no request runs on l (after a stop
// request; canceled: it returns).
void xo_test_mem_drain(xo_listener *l) {
	tw_ml *ml = tw_ml_of(l);
	xo_ctx *c = ctx();
	lk(&tws.mu);
	while (ml->inflight > 0) {
		if (!wl_wait(c, &tws.mu, &ml->waiters, 1)) {
			break;
		}
	}
	unlk(&tws.mu);
}

static tw_ml *tw_ml_find(const char *hp, int64_t n) {
	for (tw_ml *ml = tws.mls; ml; ml = ml->next) {
		if (ml->closed && !ml->router) {
			continue;
		}
		if (((int64_t)strlen(ml->l->addr) == n && memcmp(ml->l->addr, hp, (size_t)n) == 0) ||
		    (ml->alias && (int64_t)strlen(ml->alias) == n && memcmp(ml->alias, hp, (size_t)n) == 0)) {
			return ml;
		}
	}
	return NULL;
}

// xo_test_mem_begin starts a client request to host:port hp: the router
// serving it (an Any, a reference) in a list of one, or the empty list
// when nothing serves it. A listener that exists but is not served yet
// keeps the request waiting, as a real backlog does, until it is served
// or closed (canceled: the empty list).
xo_rlist *xo_test_mem_begin(const char *hp, int64_t n) {
	xo_ctx *c = ctx();
	lk(&tws.mu);
	for (;;) {
		tw_ml *ml = tw_ml_find(hp, n);
		if (!ml) {
			break;
		}
		if (ml->router) {
			ml->inflight++;
			xo_any *r = ml->router;
			h_retain(r);
			unlk(&tws.mu);
			xo_rlist *l = xo_rl_new(4, 8);
			((xo_any **)rl_data(l))[0] = r;
			l->len = 1;
			return l;
		}
		if (ml->closed) {
			break;
		}
		if (!wl_wait(c, &tws.mu, &ml->waiters, 1)) {
			break;
		}
	}
	unlk(&tws.mu);
	return xo_rl_new(4, 8);
}

// xo_test_mem_end ends a request started by xo_test_mem_begin.
void xo_test_mem_end(const char *hp, int64_t n) {
	lk(&tws.mu);
	tw_ml *ml = tw_ml_find(hp, n);
	if (!ml) {
		for (tw_ml *m = tws.mls; m; m = m->next) {
			if (((int64_t)strlen(m->l->addr) == n && memcmp(m->l->addr, hp, (size_t)n) == 0) ||
			    (m->alias && (int64_t)strlen(m->alias) == n && memcmp(m->alias, hp, (size_t)n) == 0)) {
				ml = m;
				break;
			}
		}
	}
	if (ml && ml->inflight > 0) {
		ml->inflight--;
		if (ml->inflight == 0) {
			wl_wake(&ml->waiters);
		}
	}
	unlk(&tws.mu);
}

// FakeNet handlers and failures, and the requests seen

static void tw_fake_add(tw_fake **list, int *n, const char *m, int64_t mn, const char *pre, int64_t pn, const char *path, int64_t an, xo_any *v, int once) {
	h_retain(v);
	lk(&tws.mu);
	*list = tw_grow(*list, *n, sizeof(tw_fake));
	(*list)[*n] = (tw_fake){.method = tw_strdup(m, mn), .prefix = tw_strdup(pre, pn), .path = tw_strdup(path, an), .v = v, .once = once};
	(*n)++;
	unlk(&tws.mu);
}

// xo_test_fake_handle is FakeNet.handle: method ("" any), the URL prefix
// (scheme://host[:port], "" any), the path pattern, and the handler.
void xo_test_fake_handle(const char *m, int64_t mn, const char *pre, int64_t pn, const char *path, int64_t an, xo_any *h) {
	tw_fake_add(&tws.handlers, &tws.nhandlers, m, mn, pre, pn, path, an, h, 0);
}

// xo_test_fake_fail is FakeNet.fail (once: fail_next) with the ClientErr.
void xo_test_fake_fail(const char *m, int64_t mn, const char *pre, int64_t pn, const char *path, int64_t an, xo_any *e, int64_t once) {
	tw_fake_add(&tws.fails, &tws.nfails, m, mn, pre, pn, path, an, e, (int)once);
}

// xo_test_fake_clear is FakeNet.clear_failures.
void xo_test_fake_clear(void) {
	lk(&tws.mu);
	int n = tws.nfails;
	tw_fake *fs = tws.fails;
	tws.fails = NULL;
	tws.nfails = 0;
	unlk(&tws.mu);
	for (int i = 0; i < n; i++) {
		free(fs[i].method);
		free(fs[i].prefix);
		free(fs[i].path);
		h_release(fs[i].v);
	}
	free(fs);
}

static tw_fake *tw_fake_at(int kind, int64_t i) {
	tw_fake *list = kind ? tws.fails : tws.handlers;
	int n = kind ? tws.nfails : tws.nhandlers;
	return i >= 0 && i < n ? &list[i] : NULL;
}

// xo_test_fake_count is the number of handlers (kind 0) or failures (1).
int64_t xo_test_fake_count(int64_t kind) {
	lk(&tws.mu);
	int64_t n = kind ? tws.nfails : tws.nhandlers;
	unlk(&tws.mu);
	return n;
}

// xo_test_fake_text is the method (what 0), prefix (1), or path pattern
// (2) of handler or failure i.
xo_rstr *xo_test_fake_text(int64_t kind, int64_t i, int64_t what, int64_t *len) {
	lk(&tws.mu);
	tw_fake *f = tw_fake_at((int)kind, i);
	const char *s = !f ? "" : what == 0 ? f->method : what == 1 ? f->prefix : f->path;
	xo_rstr *r = nstr(s, (int64_t)strlen(s), len);
	unlk(&tws.mu);
	return r;
}

// xo_test_fake_value is the handler or the error of entry i (a
// reference).
xo_any *xo_test_fake_value(int64_t kind, int64_t i) {
	lk(&tws.mu);
	tw_fake *f = tw_fake_at((int)kind, i);
	xo_any *v = f ? f->v : NULL;
	if (v) {
		h_retain(v);
	}
	unlk(&tws.mu);
	return v ? v : xo_h_zero();
}

// xo_test_fake_used is called when failure i matched: one of fail_next
// is removed.
void xo_test_fake_used(int64_t i) {
	lk(&tws.mu);
	tw_fake *f = tw_fake_at(1, i);
	if (!f || !f->once) {
		unlk(&tws.mu);
		return;
	}
	tw_fake gone = *f;
	memmove(&tws.fails[i], &tws.fails[i + 1], (size_t)(tws.nfails - i - 1) * sizeof(tw_fake));
	tws.nfails--;
	unlk(&tws.mu);
	free(gone.method);
	free(gone.prefix);
	free(gone.path);
	h_release(gone.v);
}

// xo_test_record keeps a request a client made (FakeNet.requests).
void xo_test_record(xo_any *req) {
	h_retain(req);
	lk(&tws.mu);
	tws.reqs = tw_grow(tws.reqs, tws.nreqs, sizeof(xo_any *));
	tws.reqs[tws.nreqs++] = req;
	unlk(&tws.mu);
}

int64_t xo_test_req_count(void) {
	lk(&tws.mu);
	int64_t n = tws.nreqs;
	unlk(&tws.mu);
	return n;
}

xo_any *xo_test_req_get(int64_t i) {
	lk(&tws.mu);
	xo_any *v = i >= 0 && i < tws.nreqs ? tws.reqs[i] : NULL;
	if (v) {
		h_retain(v);
	}
	unlk(&tws.mu);
	return v ? v : xo_h_zero();
}

// MemFs (std.md 8.3, decision 0108): std/native/testing.xo keeps the file
// system as an Xo value (testing.MemState) here between operations. An
// operation takes it (locking the store), works on it without waiting,
// and puts it back (unlocking): operations are atomic, as the Go
// backend's under its mutex.
static struct {
	pthread_mutex_t mu;
	xo_any *st;
} tw_memfs = {.mu = PTHREAD_MUTEX_INITIALIZER};

// xo_test_memfs_take locks the store and moves the state out (a list of
// one, or empty before the first operation).
xo_rlist *xo_test_memfs_take(void) {
	pthread_mutex_lock(&tw_memfs.mu);
	xo_rlist *l = xo_rl_new(4, 8);
	if (tw_memfs.st) {
		((xo_any **)rl_data(l))[0] = tw_memfs.st;
		l->len = 1;
		tw_memfs.st = NULL;
	}
	return l;
}

// xo_test_memfs_put stores the state and unlocks the store.
void xo_test_memfs_put(xo_any *st) {
	h_retain(st);
	tw_memfs.st = st;
	pthread_mutex_unlock(&tw_memfs.mu);
}

// xo_test_now is the virtual clock's now (MemFs modified times).
int64_t xo_test_now(void) { return xo_clock_now(); }

// Go's path.Match, for MemFs.fail_next patterns: a bad pattern matches
// nothing (std/rt's globMatch).

typedef struct {
	const unsigned char *p;
	int64_t n;
} tw_sv;

static int32_t tw_rune(tw_sv *s) {
	int size;
	int32_t r = n_utf8_decode(s->p, s->n, &size);
	s->p += size;
	s->n -= size;
	return r;
}

// tw_get_esc is path.getEsc: 0 on a bad pattern.
static int tw_get_esc(tw_sv *c, int32_t *r) {
	if (c->n == 0 || c->p[0] == '-' || c->p[0] == ']') {
		return 0;
	}
	if (c->p[0] == '\\') {
		c->p++;
		c->n--;
		if (c->n == 0) {
			return 0;
		}
	}
	int size;
	*r = n_utf8_decode(c->p, c->n, &size);
	if (*r == 0xFFFD && size == 1) {
		return 0;
	}
	c->p += size;
	c->n -= size;
	return c->n != 0;
}

// tw_match_chunk is path.matchChunk: 1 matched (rest set), 0 not, -1 a
// bad pattern.
static int tw_match_chunk(tw_sv chunk, tw_sv s, tw_sv *rest) {
	int failed = 0;
	while (chunk.n > 0) {
		if (!failed && s.n == 0) {
			failed = 1;
		}
		switch (chunk.p[0]) {
		case '[': {
			int32_t r = 0;
			if (!failed) {
				r = tw_rune(&s);
			}
			chunk.p++;
			chunk.n--;
			int negated = 0;
			if (chunk.n > 0 && chunk.p[0] == '^') {
				negated = 1;
				chunk.p++;
				chunk.n--;
			}
			int match = 0, nrange = 0;
			for (;;) {
				if (chunk.n > 0 && chunk.p[0] == ']' && nrange > 0) {
					chunk.p++;
					chunk.n--;
					break;
				}
				int32_t lo, hi;
				if (!tw_get_esc(&chunk, &lo)) {
					return -1;
				}
				hi = lo;
				if (chunk.p[0] == '-') {
					chunk.p++;
					chunk.n--;
					if (!tw_get_esc(&chunk, &hi)) {
						return -1;
					}
				}
				if (lo <= r && r <= hi) {
					match = 1;
				}
				nrange++;
			}
			if (match == negated) {
				failed = 1;
			}
			break;
		}
		case '?':
			if (!failed) {
				if (s.p[0] == '/') {
					failed = 1;
				}
				tw_rune(&s);
			}
			chunk.p++;
			chunk.n--;
			break;
		case '\\':
			chunk.p++;
			chunk.n--;
			if (chunk.n == 0) {
				return -1;
			}
			// fallthrough
		default:
			if (!failed) {
				if (chunk.p[0] != s.p[0]) {
					failed = 1;
				}
				s.p++;
				s.n--;
			}
			chunk.p++;
			chunk.n--;
		}
	}
	if (failed) {
		return 0;
	}
	*rest = s;
	return 1;
}

// tw_scan_chunk is path.scanChunk.
static int tw_scan_chunk(tw_sv *pat, tw_sv *chunk) {
	int star = 0;
	while (pat->n > 0 && pat->p[0] == '*') {
		pat->p++;
		pat->n--;
		star = 1;
	}
	int inrange = 0;
	int64_t i;
	for (i = 0; i < pat->n; i++) {
		unsigned char c = pat->p[i];
		if (c == '\\') {
			if (i + 1 < pat->n) {
				i++;
			}
		} else if (c == '[') {
			inrange = 1;
		} else if (c == ']') {
			inrange = 0;
		} else if (c == '*' && !inrange) {
			break;
		}
	}
	chunk->p = pat->p;
	chunk->n = i;
	pat->p += i;
	pat->n -= i;
	return star;
}

// xo_test_glob is path.Match(pattern, name) without its error.
int64_t xo_test_glob(const char *pp, int64_t pn, const char *np, int64_t nn) {
	tw_sv pat = {(const unsigned char *)pp, pn}, name = {(const unsigned char *)np, nn};
	while (pat.n > 0) {
		tw_sv chunk, t;
		int star = tw_scan_chunk(&pat, &chunk);
		if (star && chunk.n == 0) {
			return memchr(name.p, '/', (size_t)name.n) == NULL;
		}
		int ok = tw_match_chunk(chunk, name, &t);
		if (ok == 1 && (t.n == 0 || pat.n > 0)) {
			name = t;
			continue;
		}
		if (ok < 0) {
			return 0;
		}
		int next = 0;
		if (star) {
			for (int64_t i = 0; i < name.n && name.p[i] != '/'; i++) {
				tw_sv from = {name.p + i + 1, name.n - i - 1};
				int k = tw_match_chunk(chunk, from, &t);
				if (k == 1) {
					if (pat.n == 0 && t.n > 0) {
						continue;
					}
					name = t;
					next = 1;
					break;
				}
				if (k < 0) {
					return 0;
				}
			}
		}
		if (next) {
			continue;
		}
		return 0;
	}
	return name.n == 0;
}

// tw_cleanup releases the values the test world keeps (handlers,
// failures, requests, served routers, the MemFs) when the test ends, for
// the leak check.
__attribute__((unused)) static void tw_cleanup(void) {
	xo_test_fake_clear();
	pthread_mutex_lock(&tw_memfs.mu);
	xo_any *mst = tw_memfs.st;
	tw_memfs.st = NULL;
	pthread_mutex_unlock(&tw_memfs.mu);
	if (mst) {
		h_release(mst);
	}
	lk(&tws.mu);
	int nh = tws.nhandlers, nr = tws.nreqs;
	tw_fake *hs = tws.handlers;
	xo_any **rs = tws.reqs;
	tws.handlers = NULL;
	tws.nhandlers = 0;
	tws.reqs = NULL;
	tws.nreqs = 0;
	xo_any *routers[64];
	int nrouters = 0;
	for (tw_ml *ml = tws.mls; ml && nrouters < 64; ml = ml->next) {
		if (ml->router) {
			routers[nrouters++] = ml->router;
			ml->router = NULL;
		}
	}
	unlk(&tws.mu);
	for (int i = 0; i < nh; i++) {
		free(hs[i].method);
		free(hs[i].prefix);
		free(hs[i].path);
		h_release(hs[i].v);
	}
	free(hs);
	for (int i = 0; i < nr; i++) {
		h_release(rs[i]);
	}
	free(rs);
	for (int i = 0; i < nrouters; i++) {
		h_release(routers[i]);
	}
}

// testing.run_main and FakeProc.signal_after (core 7.5)

static void tw_signal_fire(xo_timer *t) {
	// ev.mu is held: stop_signal takes stop.mu, then may add a timer
	// (ev.mu): run it unlocked
	pthread_mutex_unlock(&ev.mu);
	stop_signal();
	pthread_mutex_lock(&ev.mu);
	free(t);
}

static void tw_signal_arm(int64_t d) {
	xo_timer *t = calloc(1, sizeof(xo_timer));
	if (!t) {
		xo_oom();
	}
	t->when = deadline(d < 0 ? 0 : d);
	t->idx = -1;
	t->fn = tw_signal_fire;
	timer_add(t);
}

// xo_test_signal_after is FakeProc.signal_after(d): inside run_main a
// signal d from now, before it d from run_main's start.
void xo_test_signal_after(int64_t d) {
	if (__atomic_load_n(&tw.in_main, __ATOMIC_ACQUIRE)) {
		tw_signal_arm(d);
		return;
	}
	lk(&tws.mu);
	tws.signals = tw_grow(tws.signals, tws.nsignals, sizeof(int64_t));
	tws.signals[tws.nsignals++] = d;
	unlk(&tws.mu);
}

// xo_test_main_begin starts testing.run_main: main's root is a new node
// below the test's, the stop state is fresh, and the queued signals are
// armed.
void xo_test_main_begin(xo_rlist *args) {
	xo_test_set_args(args);
	xo_ctx *c = ctx();
	xo_node *n = calloc(1, sizeof(xo_node));
	if (!n) {
		xo_oom();
	}
	node_link(n, c->node);
	tws.outer = c->node;
	tws.main_node = n;
	c->node = n;
	pthread_mutex_lock(&stop.mu);
	stop.signals = 0;
	__atomic_store_n(&stop.requested, 0, __ATOMIC_RELEASE);
	__atomic_store_n(&stop.canceled, 0, __ATOMIC_RELEASE);
	stop.grace = XO_DEFAULT_GRACE;
	stop.root = n;
	xo_chan *old = stop.stopping;
	stop.stopping = NULL;
	pthread_mutex_unlock(&stop.mu);
	if (old) {
		h_release(old);
	}
	__atomic_store_n(&tw.in_main, 1, __ATOMIC_RELEASE);
	lk(&tws.mu);
	int64_t *sigs = tws.signals;
	int ns = tws.nsignals;
	tws.signals = NULL;
	tws.nsignals = 0;
	unlk(&tws.mu);
	for (int i = 0; i < ns; i++) {
		tw_signal_arm(sigs[i]);
	}
	free(sigs);
}

// xo_test_main_end ends testing.run_main with main's status: the test's
// node is current again; when main unwound because the stop state
// canceled its root (and not because the test is canceled), the
// unwinding ends here and the status is 0.
int64_t xo_test_main_end(int64_t code) {
	xo_ctx *c = ctx();
	__atomic_store_n(&tw.in_main, 0, __ATOMIC_RELEASE);
	xo_node *n = tws.main_node;
	c->node = tws.outer;
	pthread_mutex_lock(&stop.mu);
	if (stop.root == n) {
		stop.root = NULL;
	}
	timer_del(&stop.grace_t);
	pthread_mutex_unlock(&stop.mu);
	if (n) {
		node_unlink(n);
		free(n);
		tws.main_node = NULL;
	}
	if (__atomic_exchange_n(&tws.exiting, 0, __ATOMIC_ACQ_REL) && !node_canceled(c->node)) {
		// os.proc.exit unwound main (tw_exit): its code is the status
		c->unwinding = 0;
		lk(&tws.mu);
		int64_t ec = tws.exit_code;
		unlk(&tws.mu);
		return ec;
	}
	if (c->unwinding && __atomic_load_n(&stop.canceled, __ATOMIC_ACQUIRE) && !node_canceled(c->node)) {
		c->unwinding = 0;
		return 0;
	}
	return code;
}
