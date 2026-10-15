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

// an in memory listener
typedef struct tw_ml {
	xo_listener *l; // not counted: the listener frees its tw_ml
	char *alias;    // localhost:<port> for a listener on 127.0.0.1
	xo_any *router; // served: http.Router as an Any, or NULL
	int closed;
	int served; // serve started once (std/rt's memListener.served)
	int inflight;
	int draining;
	int nbegin; // clients waiting in xo_test_mem_begin (they hold no reference)
	int freed;  // the listener is gone: the last waiting client frees this
	xo_wl waiters; // accept, clients waiting for serve, drain
	struct tw_ml *next;
} tw_ml;

static struct {
	xo_lock mu;
	int64_t *sleeps;
	int nsleeps;
	int64_t *signals; // signal_after before run_main, from its start
	int nsignals;
	tw_ml *mls;
	int next_port;
	xo_node *outer; // the node run_main restores
	xo_node *main_node;
	int exiting; // os.proc.exit unwinds main to testing.run_main
} tws;

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
	tw.real = (int)xo_x_tw_init();
	tw.virt = !(tw.real & TW_REAL_CLOCK);
	tw.vnow = TW_EPOCH;
	tw.on = 1;
#ifndef _WIN32
	signal(SIGPIPE, SIG_IGN);
#endif
}

// tw_exit is os.proc.exit in a test (std.md 8.4): the code is recorded
// (FakeProc.exit_code). Outside testing.run_main the test ends and
// passes, as on the Go backend. Inside it, main unwinds back to run_main
// (decision 0108): its root is canceled and this context unwinds from
// here (generated code checks after the call), and xo_test_main_end
// gives the code as main's status.
__attribute__((unused)) static void tw_exit(int64_t code) {
	xo_x_tw_set_exit(code);
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

// In memory listeners (std/rt's memListener)

static tw_ml *tw_ml_of(xo_listener *l) { return (tw_ml *)l->mem; }

// tw_listen is FakeNet.listen(addr): ":0" (and port 0) picks the next
// port from 49152.
__attribute__((unused)) static xo_listener *tw_listen(xo_ctx *c, const char *addr) {
	if (strncmp(addr, "unix:", 5) == 0) {
		// an in memory Unix domain socket (decision 0190)
		tw_ml *ml = calloc(1, sizeof(tw_ml));
		if (!ml) {
			xo_oom();
		}
		xo_listener *l = xo_alloc(sizeof(xo_listener));
		h_init(l, listener_free);
		l->pd = NULL;
		l->addr = strdup(addr);
		l->mem = ml;
		l->unlink_path = 0;
		lk(&tws.mu);
		ml->l = l;
		ml->next = tws.mls;
		tws.mls = ml;
		unlk(&tws.mu);
		net_ok(c);
		return l;
	}
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
	l->unlink_path = 0;
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
	xo_any *r = ml->router;
	ml->router = NULL;
	ml->closed = 1;
	ml->l = NULL;
	// a client still waiting for a serve that never came wakes and
	// fails; it frees ml when it leaves (gotchas/mem-listener-backlog-after-serve.md)
	wl_wake(&ml->waiters);
	int keep = ml->nbegin > 0;
	ml->freed = keep;
	unlk(&tws.mu);
	if (r) {
		h_release(r);
	}
	if (!keep) {
		free(ml->alias);
		free(ml);
	}
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
	ml->served = 1;
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
// when nothing serves it. A listener that exists but was never served
// keeps the request waiting, as a real backlog does, until it is served
// or closed (canceled: the empty list). Once serve started, a request
// while it is not served (stopped, draining, returned) fails at once, as
// std/rt's memListener: a waiter there would be woken by nothing
// (gotchas/mem-listener-backlog-after-serve.md).
xo_rlist *xo_test_mem_begin(const char *hp, int64_t n) {
	xo_ctx *c = ctx();
	lk(&tws.mu);
	tw_ml *ml = tw_ml_find(hp, n);
	while (ml && !ml->router && !ml->served && !ml->closed) {
		ml->nbegin++;
		int ok = wl_wait(c, &tws.mu, &ml->waiters, 1);
		ml->nbegin--;
		if (ml->freed) {
			if (ml->nbegin == 0) {
				free(ml->alias);
				free(ml);
			}
			ml = NULL;
			break;
		}
		if (!ok) {
			ml = NULL;
		}
	}
	if (ml && ml->router) {
		ml->inflight++;
		xo_any *r = ml->router;
		h_retain(r);
		unlk(&tws.mu);
		xo_rlist *l = xo_rl_new(4, 8);
		((xo_any **)rl_data(l))[0] = r;
		l->len = 1;
		return l;
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

// xo_test_now is the virtual clock's now (MemFs modified times).
int64_t xo_test_now(void) { return xo_clock_now(); }

// tw_cleanup releases the values the test world keeps (testworld.xo's,
// and the routers served) when the test ends, for the leak check.
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
	lk(&tws.mu);
	xo_any *routers[64];
	int nrouters = 0;
	for (tw_ml *ml = tws.mls; ml && nrouters < 64; ml = ml->next) {
		if (ml->router) {
			routers[nrouters++] = ml->router;
			ml->router = NULL;
		}
	}
	unlk(&tws.mu);
	for (int i = 0; i < nrouters; i++) {
		h_release(routers[i]);
	}
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
	xo_x_tw_set_args((int64_t)(intptr_t)args);
	xo_ctx *c = ctx();
	xo_node *n = calloc(1, sizeof(xo_node));
	if (!n) {
		xo_oom();
	}
	node_link(n, c->node);
	tws.outer = c->node;
	tws.main_node = n;
	c->node = n;
	xo_x_stop_reset(XO_ADDR(n));
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

// tw_main_status ends testing.run_main with main's status: the test's
// node is current again; when main unwound because the stop state
// canceled its root (and not because the test is canceled), the
// unwinding ends here and the status is 0.
static int64_t tw_main_status(int64_t code, int *exited) {
	xo_ctx *c = ctx();
	__atomic_store_n(&tw.in_main, 0, __ATOMIC_RELEASE);
	xo_node *n = tws.main_node;
	c->node = tws.outer;
	*exited = (int)xo_x_stop_unroot(XO_ADDR(n));
	if (n) {
		node_unlink(n);
		free(n);
		tws.main_node = NULL;
	}
	if (__atomic_exchange_n(&tws.exiting, 0, __ATOMIC_ACQ_REL) && !node_canceled(c->node)) {
		// os.proc.exit unwound main (tw_exit): its code is the status
		c->unwinding = 0;
		return xo_x_tw_exit_value();
	}
	if (c->unwinding && xo_x_stop_canceled() && !node_canceled(c->node)) {
		c->unwinding = 0;
		return 0;
	}
	return code;
}

// xo_test_main_end is tw_main_status, except that after a third signal
// the status is 130 however main ended, and FakeProc.exit_code is
// Some(130) (std/rt's Testing_run_main).
int64_t xo_test_main_end(int64_t code) {
	int exited = 0;
	int64_t r = tw_main_status(code, &exited);
	if (exited) {
		xo_x_tw_set_exit(130);
		r = 130;
	}
	return r;
}
