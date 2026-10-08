// tasks.c: a stress test of the task runtime (decision 0068), run by
// internal/llvm's TestRuntimeTasks under ThreadSanitizer and optimized.
// It includes the runtime and plays the part of generated code: entry
// functions run a task's closure (here a job number in a list of one) and
// record its result, lists of one carry values through channels and
// mutexes, and xo_unwinding is checked after each cancellation point.
//
// At the end every allocation must be freed (the leak check counts of all
// heaps). Exit status 0 means every test passed.

#define XO_RT_NO_MAIN
#include "../xo_rt.c"

#ifndef N
#define N 2000
#endif

static int failures;

#define CHECK(c)                                                                  \
	do {                                                                          \
		if (!(c)) {                                                               \
			fprintf(stderr, "%s:%d: check failed: %s\n", __FILE__, __LINE__, #c); \
			__atomic_fetch_add(&failures, 1, __ATOMIC_RELAXED);                  \
		}                                                                         \
	} while (0)

// lists of one Int, as generated code makes them
static xo_rlist *one(int64_t v) {
	xo_rlist *l = xo_rl_new(1, 8);
	l->len = 1;
	*(int64_t *)rl_data(l) = v;
	return l;
}

static int64_t first(xo_rlist *l) { return *(int64_t *)rl_data(l); }

static void drop_int_list(xo_rlist *l) {
	if (xo_rc_dec(&l->rc)) {
		xo_rl_free(l, 8, NULL);
	}
}

static void release(void *h) {
	if (xo_rc_dec(&((xo_hdr *)h)->rc)) {
		xo_h_free(h);
	}
}

static const char loc[] = "tasks.c:1";
#define LOC loc, (int64_t)(sizeof loc - 1)

// ---------------------------------------------------------------------------
// Jobs: what a spawned closure does, chosen by the job's code

typedef struct {
	int64_t kind, arg;
	void *obj; // a handle the job uses (a channel, a mutex, an atomic)
} job;

enum { J_SQUARE, J_TREE, J_PRODUCE, J_RECV_FOREVER, J_FAIL, J_LOCK, J_READ, J_ADD, J_SEND_FOREVER, J_PING };

static job jobs[1 << 16];
static int64_t njobs;

static int64_t mkjob(int64_t kind, int64_t arg, void *obj) {
	int64_t i = __atomic_fetch_add(&njobs, 1, __ATOMIC_RELAXED);
	if (i >= (int64_t)(sizeof jobs / sizeof jobs[0])) {
		fputs("too many jobs\n", stderr);
		exit(2);
	}
	jobs[i] = (job){kind, arg, obj};
	return i;
}

static xo_task *spawn(xo_scope *s, int64_t kind, int64_t arg, void *obj, int64_t flags);

// tree spawns a binary tree of tasks in a scope of its own and returns
// the number of leaves (each level awaits its children)
static int64_t tree(int64_t depth) {
	if (depth == 0) {
		return 1;
	}
	xo_scope *s = xo_scope_open();
	xo_task *a = spawn(s, J_TREE, depth - 1, NULL, 2);
	xo_task *b = spawn(s, J_TREE, depth - 1, NULL, 2);
	xo_rlist *ra = xo_task_await(a, LOC);
	xo_rlist *rb = xo_task_await(b, LOC);
	int64_t n = first(ra) + first(rb);
	drop_int_list(ra);
	drop_int_list(rb);
	release(a);
	release(b);
	CHECK(xo_scope_close(s, 0) == 0);
	release(s);
	return n;
}

// the entry function of every task
static void run_job(xo_rlist *fl, xo_task *t) {
	job j = jobs[first(fl)];
	drop_int_list(fl);
	int64_t res = 0, failed = 0;
	switch (j.kind) {
	case J_SQUARE:
		res = j.arg * j.arg;
		break;
	case J_TREE:
		res = tree(j.arg);
		break;
	case J_PRODUCE:
		for (int64_t i = 1; i <= j.arg; i++) {
			xo_rlist *l = one(i);
			xo_rc_mark(&l->rc);
			xo_chan_send(j.obj, l, LOC);
			if (xo_unwinding()) {
				break;
			}
		}
		if (!xo_unwinding()) {
			xo_chan_close(j.obj);
		}
		break;
	case J_RECV_FOREVER:
		for (;;) {
			xo_rlist *l = xo_chan_recv(j.obj);
			if (xo_unwinding()) {
				break;
			}
			res += l->len ? first(l) : 0;
			drop_int_list(l);
		}
		break;
	case J_SEND_FOREVER:
		for (;;) {
			xo_chan_send(j.obj, one(7), LOC);
			if (xo_unwinding()) {
				break;
			}
		}
		break;
	case J_FAIL:
		failed = 1;
		res = j.arg;
		break;
	case J_LOCK:
		for (int64_t i = 0; i < j.arg; i++) {
			xo_rlist *l = xo_mutex_lock(j.obj, LOC);
			if (xo_unwinding()) {
				break;
			}
			*(int64_t *)rl_data(l) += 1;
			xo_mutex_unlock(j.obj, l);
		}
		break;
	case J_READ:
		for (int64_t i = 0; i < j.arg; i++) {
			xo_rlist *l = xo_mutex_rlock(j.obj, LOC);
			if (xo_unwinding()) {
				break;
			}
			res = first(l);
			xo_mutex_runlock(j.obj, l);
		}
		break;
	case J_ADD:
		for (int64_t i = 0; i < j.arg; i++) {
			xo_atomic_add(j.obj, 1, LOC);
		}
		break;
	case J_PING: {
		// obj is a pair of unbuffered channels: receive, send back + 1
		xo_chan **cs = j.obj;
		for (int64_t i = 0; i < j.arg; i++) {
			xo_rlist *l = xo_chan_recv(cs[0]);
			if (xo_unwinding()) {
				break;
			}
			int64_t v = first(l);
			drop_int_list(l);
			xo_chan_send(cs[1], one(v + 1), LOC);
			if (xo_unwinding()) {
				break;
			}
		}
		break;
	}
	}
	if (xo_unwinding()) {
		xo_task_unwound(t);
	} else {
		xo_rlist *r = one(res);
		xo_rc_mark(&r->rc);
		xo_task_done(t, r, failed);
	}
	release(t); // an entry function owns its parameters
}

static xo_task *spawn(xo_scope *s, int64_t kind, int64_t arg, void *obj, int64_t flags) {
	int64_t id = mkjob(kind, arg, obj);
	return xo_task_spawn(s, one(id), (int64_t)run_job, (int64_t)drop_int_list, flags, 0, LOC);
}

// ---------------------------------------------------------------------------
// Tests

static void test_fanout(void) {
	xo_scope *s = xo_scope_open();
	xo_task **ts = malloc(N * sizeof *ts);
	for (int64_t i = 0; i < N; i++) {
		ts[i] = spawn(s, J_SQUARE, i, NULL, 2);
	}
	int64_t sum = 0;
	for (int64_t i = 0; i < N; i++) {
		xo_rlist *r = xo_task_await(ts[i], LOC);
		sum += first(r);
		drop_int_list(r);
		release(ts[i]);
	}
	free(ts);
	CHECK(xo_scope_close(s, 0) == 0);
	release(s);
	int64_t want = 0;
	for (int64_t i = 0; i < N; i++) {
		want += i * i;
	}
	CHECK(sum == want);
	// handles discarded: the scope's join waits for them
	s = xo_scope_open();
	for (int64_t i = 0; i < N; i++) {
		release(spawn(s, J_SQUARE, i, NULL, 0));
	}
	CHECK(xo_scope_close(s, 0) == 0);
	release(s);
	fputs("fanout: ok\n", stderr);
}

static void test_tree(void) {
	CHECK(tree(10) == 1024);
	fputs("tree: ok\n", stderr);
}

static void test_channel(void) {
	for (int64_t cap = 0; cap <= 4; cap += 4) {
		xo_chan *c = xo_chan_new(cap, (int64_t)drop_int_list, LOC);
		xo_scope *s = xo_scope_open();
		release(spawn(s, J_PRODUCE, N, c, 0));
		int64_t sum = 0;
		for (;;) {
			xo_rlist *l = xo_chan_recv(c);
			CHECK(!xo_unwinding());
			if (l->len == 0) {
				break;
			}
			sum += first(l);
			drop_int_list(l);
		}
		CHECK(xo_scope_close(s, 0) == 0);
		release(s);
		CHECK(sum == N * (N + 1) / 2);
		release(c);
	}
	// try_send and try_recv, buffered values freed with the channel
	xo_chan *c = xo_chan_new(2, (int64_t)drop_int_list, LOC);
	CHECK(xo_chan_try_send(c, one(1)) == 1);
	CHECK(xo_chan_try_send(c, one(2)) == 1);
	CHECK(xo_chan_try_send(c, one(3)) == 0);
	CHECK(xo_chan_len(c) == 2);
	xo_rlist *l = xo_chan_try_recv(c);
	CHECK(l->len == 1 && first(l) == 1);
	drop_int_list(l);
	release(c);
	// ping pong on unbuffered channels
	xo_chan *cs[2] = {xo_chan_new(0, (int64_t)drop_int_list, LOC), xo_chan_new(0, (int64_t)drop_int_list, LOC)};
	xo_scope *s = xo_scope_open();
	release(spawn(s, J_PING, 200, cs, 0));
	int64_t v = 0;
	for (int i = 0; i < 200; i++) {
		xo_chan_send(cs[0], one(v), LOC);
		xo_rlist *r = xo_chan_recv(cs[1]);
		v = first(r) + 1;
		drop_int_list(r);
	}
	CHECK(xo_scope_close(s, 0) == 0);
	release(s);
	CHECK(v == 400);
	release(cs[0]);
	release(cs[1]);
	fputs("channel: ok\n", stderr);
}

static void test_cancel(void) {
	// background tasks blocked in recv and send are canceled at the end
	xo_chan *c = xo_chan_new(0, (int64_t)drop_int_list, LOC);
	xo_chan *full = xo_chan_new(1, (int64_t)drop_int_list, LOC);
	xo_scope *s = xo_scope_open();
	for (int i = 0; i < 8; i++) {
		release(spawn(s, J_RECV_FOREVER, 0, c, 1));
		release(spawn(s, J_SEND_FOREVER, 0, full, 1));
	}
	CHECK(xo_scope_close(s, 0) == 0);
	release(s);
	release(c);
	release(full);
	// s.cancel(): the tasks unwind, the body goes on; awaiting a canceled
	// task in a context that is not canceled would fault, so only check
	// that the scope ends
	c = xo_chan_new(0, (int64_t)drop_int_list, LOC);
	s = xo_scope_open();
	for (int i = 0; i < 8; i++) {
		release(spawn(s, J_RECV_FOREVER, 0, c, 0));
	}
	xo_scope_cancel(s);
	CHECK(!xo_is_canceled());
	CHECK(xo_scope_close(s, 0) == 0);
	release(s);
	release(c);
	fputs("cancel: ok\n", stderr);
}

static void test_failure(void) {
	// a failed task cancels the body: its receive unwinds, and the scope
	// reports the failure (which ends the unwinding)
	xo_chan *c = xo_chan_new(0, (int64_t)drop_int_list, LOC);
	xo_scope *s = xo_scope_open();
	release(spawn(s, J_RECV_FOREVER, 0, c, 0));
	release(spawn(s, J_FAIL, 42, NULL, 0));
	xo_rlist *l = xo_chan_recv(c); // never gets a value
	CHECK(xo_unwinding() && l->len == 0);
	CHECK(xo_scope_close(s, 2) == 1);
	CHECK(!xo_unwinding());
	xo_task *f = xo_scope_fail_task(s);
	xo_rlist *r = xo_task_result(f);
	CHECK(first(r) == 42);
	drop_int_list(r);
	release(f);
	release(s);
	release(c);
	// an awaited failure does not fail the scope
	s = xo_scope_open();
	xo_task *t = spawn(s, J_FAIL, 7, NULL, 2);
	r = xo_task_await(t, LOC);
	CHECK(first(r) == 7);
	drop_int_list(r);
	release(t);
	CHECK(xo_scope_close(s, 0) == 0);
	release(s);
	fputs("failure: ok\n", stderr);
}

static void test_mutex(void) {
	xo_rlist *v = one(0);
	xo_rc_mark(&v->rc);
	xo_mutex *m = xo_mutex_new(v, (int64_t)drop_int_list);
	xo_atomic *a = xo_atomic_new(0);
	xo_scope *s = xo_scope_open();
	for (int i = 0; i < 8; i++) {
		release(spawn(s, J_LOCK, N / 8, m, 0));
		release(spawn(s, J_READ, N / 8, m, 0));
		release(spawn(s, J_ADD, N / 8, a, 0));
	}
	CHECK(xo_scope_close(s, 0) == 0);
	release(s);
	xo_rlist *l = xo_mutex_rlock(m, LOC);
	CHECK(first(l) == 8 * (N / 8));
	xo_mutex_runlock(m, l);
	CHECK(xo_atomic_get(a) == 8 * (N / 8));
	CHECK(xo_atomic_max(a, 5) == 8 * (N / 8));
	CHECK(xo_atomic_swap(a, 3) == 8 * (N / 8));
	CHECK(xo_atomic_cas(a, 3, 4) == 1 && xo_atomic_cas(a, 3, 5) == 0 && xo_atomic_get(a) == 4);
	release(m);
	release(a);
	fputs("mutex: ok\n", stderr);
}

// leaks checks that every allocation of the test just run was freed.
static void leaks(const char *name) {
	int64_t live, total;
	xo_rt_live(&live, &total);
	if (live != 0) {
		fprintf(stderr, "%s: %lld live of %lld allocations\n", name, (long long)live, (long long)total);
		failures++;
	}
}

int main(void) {
	test_fanout();
	leaks("fanout");
	test_tree();
	leaks("tree");
	test_channel();
	leaks("channel");
	test_cancel();
	leaks("cancel");
	test_failure();
	leaks("failure");
	test_mutex();
	leaks("mutex");
	return failures != 0;
}
