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
//   - Rand is seeded, the environment starts empty, standard output and
//     error are captured, the file system is empty, log records are kept
//     for testing.logs() and echoed to the harness, FakeNet's handlers
//     and failures are kept: that is std/native/llrt/testworld.xo (the
//     zero-C audit's B14), which the C reaches through cabi_tw.xo's
//     exports (xo_x_tw_*).
//   - FakeNet.listen gives an in memory listener; http.server serves it
//     by registering its router here, and the http client of the test
//     reaches a served router in process (std/native/http.xo).
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

// The test world's part 2 is std/native/llrt/twrun.xo (the zero-C audit's
// B15): it reads tw through its address (twrun_init) at these offsets,
// and a listener's in memory record (an ML record) through mem.
_Static_assert(offsetof(__typeof__(tw), vnow) == 8, "twrun.xo TW_VNOW");
_Static_assert(offsetof(__typeof__(tw), in_main) == 32, "twrun.xo TW_IN_MAIN");
_Static_assert(offsetof(__typeof__(tw), virt) == 40, "twrun.xo TW_VIRT");

// The exports of std/native/llrt/cabi_tw.xo, which test binaries carry;
// the weak definitions keep other programs linking (they never reach
// them: the C calls them only in the test world).
#define TW_WEAK __attribute__((weak))
TW_WEAK void xo_x_tw_setenv(int64_t k, int64_t p, int64_t n) { (void)k, (void)p, (void)n; }
TW_WEAK int64_t xo_x_tw_init(void) { return 0; }
TW_WEAK void xo_x_tw_set_args(int64_t l) { (void)l; }
TW_WEAK void xo_x_tw_set_exit(int64_t code) { (void)code; }
TW_WEAK int64_t xo_x_tw_exit_value(void) { return 0; }
TW_WEAK int64_t xo_x_tw_cleanup(void) { return 0; }
TW_WEAK void xo_x_twrun_init(int64_t tw) { (void)tw; }
TW_WEAK void xo_x_tw_sleep(int64_t d) { (void)d; }
TW_WEAK int64_t xo_x_tw_exit(int64_t code) { return (void)code, 0; }
TW_WEAK int64_t xo_x_tw_listen(int64_t p, int64_t n, int64_t l) { return (void)p, (void)n, (void)l, 0; }
TW_WEAK int64_t xo_x_tw_ml_caddr(int64_t ml) { return (void)ml, 0; }
TW_WEAK void xo_x_tw_ml_free(int64_t ml) { (void)ml; }
TW_WEAK void xo_x_tw_ml_close(int64_t ml) { (void)ml; }
TW_WEAK int64_t xo_x_tw_ml_accept(int64_t ml) { return (void)ml, 0; }
TW_WEAK void xo_x_twrun_cleanup(void) {}

__attribute__((unused)) static void *tw_grow(void *p, int n, size_t size) {
	// room for element n (doubling at powers of two)
	if (n == 0 || (n & (n - 1)) == 0) {
		p = realloc(p, (size_t)(n == 0 ? 4 : 2 * n) * size);
		if (!p) {
			xo_oom();
		}
	}
	return p;
}

__attribute__((unused)) static void tw_init(void) {
	// the variables testworld.xo reads (its V_*)
	static const char *const vars[] = {"XO_TEST_REAL", "XO_TEST_SEED", "XO_TEST", "XO_PROP_REPLAY", "XO_PROP_SEED", "XO_PROP_BEST"};
	for (int k = 0; k < (int)(sizeof vars / sizeof vars[0]); k++) {
		const char *v = getenv(vars[k]);
		xo_x_tw_setenv(k, (int64_t)(intptr_t)v, v ? (int64_t)strlen(v) : -1);
	}
	// `xo test --real` (decision 0120): the effects the test declares are
	// real; with `clock`, `net`, or `proc` the clock is the real one (a
	// socket or child process wait is not a virtual wait, and its
	// deadlines are real time)
	xo_x_twrun_init(XO_ADDR(&tw));
	tw.real = (int)xo_x_tw_init();
	tw.virt = !(tw.real & TW_REAL_CLOCK);
	tw.vnow = TW_EPOCH;
	tw.on = 1;
#ifndef _WIN32
	signal(SIGPIPE, SIG_IGN);
#endif
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
		xo_x_dl_check(XO_ADDR(&pool));
		unlk(&pool.mu);
	}
}

// tw_kick_once is one move of the clock; 1 when only callback timers
// fired (no park was signaled by the clock itself).
static int tw_kick_once(void) {
	if (!tw.virt) {
		return 0;
	}
	int64_t verdict = __atomic_exchange_n(&tw.vpending, 0, __ATOMIC_SEQ_CST);
	if (!verdict) {
		return 0;
	}
	int parks = 0;
	lk(&ev.mu);
	if (((verdict - 1) & 1) || verdict - 1 != __atomic_load_n(&tw.vepoch, __ATOMIC_SEQ_CST)) {
		// stale: two threads can each find every context blocked at
		// the same instant and both take the verdict (one from the
		// other's store); the clock moved since (or was moving), and
		// what it woke may not have run yet
		// (gotchas/virtual-clock-double-kick.md)
		unlk(&ev.mu);
		return 0;
	}
	// odd while the clock moves: a check that reads the epoch now is
	// stale too (a signal's callback runs with ev.mu released)
	__atomic_fetch_add(&tw.vepoch, 1, __ATOMIC_SEQ_CST);
	if (!th_first()) {
		unlk(&ev.mu);
		// std/rt's test world names the timers too
		static const char m[] = "fault: XO-F001: deadlock: every task is blocked and no timer is pending\n";
		write_all(2, m, sizeof m - 1);
		_exit(70);
	}
	int64_t at = th_first()->when;
	if (at > __atomic_load_n(&tw.vnow, __ATOMIC_ACQUIRE)) {
		__atomic_store_n(&tw.vnow, at, __ATOMIC_RELEASE);
	}
	for (xo_timer *t = th_first(); t && t->when <= at; t = th_first()) {
		th_remove(t);
		__atomic_store_n(&t->fired, 1, __ATOMIC_RELEASE);
		if (t->park) {
			park_signal(t->park);
			parks++;
		} else {
			t->fn(t);
		}
	}
	// what the timers woke is visible (queued, or a root counted
	// awake): checks that read the epoch from here on see it
	__atomic_fetch_add(&tw.vepoch, 1, __ATOMIC_SEQ_CST);
	unlk(&ev.mu);
	return parks == 0;
}

// tw_sleep_record lists a sleep for FakeClock.sleeps (twrun.xo; sleeps,
// advance, and set are Xo too).
__attribute__((unused)) static void tw_sleep_record(int64_t d) { xo_x_tw_sleep(d); }

// In memory listeners (std/rt's memListener)

_Static_assert(offsetof(xo_listener, mem) == 32, "twrun.xo L_MEM");

// tw_listen is FakeNet.listen(addr): an in memory listener, whose
// record and address are twrun.xo's.
__attribute__((unused)) static xo_listener *tw_listen(xo_ctx *c, const char *addr) {
	xo_listener *l = xo_alloc(sizeof(xo_listener));
	h_init(l, listener_free);
	l->pd = NULL;
	l->unlink_path = 0;
	l->mem = (void *)(intptr_t)xo_x_tw_listen(XO_ADDR(addr), (int64_t)strlen(addr), XO_ADDR(l));
	l->addr = (char *)(intptr_t)xo_x_tw_ml_caddr(XO_ADDR(l->mem));
	net_ok(c);
	return l;
}

__attribute__((unused)) static void tw_listener_free(xo_listener *l) { xo_x_tw_ml_free(XO_ADDR(l->mem)); }

__attribute__((unused)) static void tw_listener_close(xo_listener *l) { xo_x_tw_ml_close(XO_ADDR(l->mem)); }

// tw_listener_accept waits until the listener closes or the context is
// canceled: requests reach a served router in process, never as
// connections.
__attribute__((unused)) static xo_conn *tw_listener_accept(xo_ctx *c, xo_listener *l) {
	if (!xo_x_tw_ml_accept(XO_ADDR(l->mem))) {
		net_fail(c, 5, "");
		return xo_h_zero();
	}
	net_fail(c, 4, "accept tcp %s: %s", l->addr, closed_text);
	return xo_h_zero();
}

// xo_test_now is the virtual clock's now (MemFs modified times).
int64_t xo_test_now(void) { return xo_clock_now(); }

// tw_cleanup releases the values the test world keeps (testworld.xo's,
// and twrun.xo's routers served) when the test ends, for the leak check.
__attribute__((unused)) static void tw_cleanup(void) {
	// testworld.xo hands over its values (a malloc'd block: the count,
	// then the objects); they are released here, on the C main thread
	int64_t *kept = (int64_t *)(intptr_t)xo_x_tw_cleanup();
	if (kept) {
		for (int64_t i = 1; i <= kept[0]; i++) {
			h_release((void *)(intptr_t)kept[i]);
		}
		free(kept - 1);
	}
	xo_x_twrun_cleanup();
}

// testing.run_main and FakeProc.signal_after (core 7.5)

static void tw_signal_fire(xo_timer *t) {
	// ev.mu is held: stop_signal takes stop.xo's lock, then may add a
	// timer (ev.mu): run it unlocked
	unlk(&ev.mu);
	stop_signal();
	lk(&ev.mu);
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


// xo_test_signal_arm arms FakeProc.signal_after's signal d from now
// (twrun.xo signal_after and main_begin; testing.run_main is Xo too).
void xo_test_signal_arm(int64_t d) { tw_signal_arm(d); }
