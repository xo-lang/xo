// evloop.c: a test of the event loop and fibers (decision 0069), run by
// internal/llvm's TestRuntimeEventLoop under ThreadSanitizer and
// optimized. It includes the runtime and plays the part of generated
// code, as tasks.c does: timers with many sleepers on few workers, a
// socket echo with more data than the socket buffers hold (writes park
// until the reader drains), close and cancellation waking parked socket
// operations, a refused dial, select's `after`, and a pipe a child
// process inherited.
//
// Every allocation must be freed after each test. Exit status 0 means
// every test passed.

#define XO_RT_NO_MAIN
#include "../xo_rt.c"
#include "xcalls.h"
#ifndef _WIN32
#include <spawn.h>
#include <sys/wait.h>
#endif

static int failures;

#define CHECK(c)                                                                  \
	do {                                                                          \
		if (!(c)) {                                                               \
			fprintf(stderr, "%s:%d: check failed: %s\n", __FILE__, __LINE__, #c); \
			__atomic_fetch_add(&failures, 1, __ATOMIC_RELAXED);                  \
		}                                                                         \
	} while (0)

static xo_rlist *one(int64_t v) {
	xo_rlist *l = rl_new(1, 8);
	l->len = 1;
	*(int64_t *)rl_data(l) = v;
	return l;
}

static int64_t first(xo_rlist *l) { return *(int64_t *)rl_data(l); }

static void drop_int_list(xo_rlist *l) {
	if (rc_dec(&l->rc)) {
		t_rl_free(l, 8, NULL);
	}
}

// a List[Str] of one, as conn_read returns it
static void drop_str_list(xo_rlist *l) {
	if (rc_dec(&l->rc)) {
		for (int64_t i = 0; i < l->len; i++) {
			xo_rstr *s = ((xo_rstr **)rl_data(l))[2 * i];
			if (rc_dec(&s->rc)) {
				xo_free(s);
			}
		}
		xo_free(l);
	}
}

static void release(void *h) {
	if (rc_dec(&((xo_hdr *)h)->rc)) {
		h_free(h);
	}
}

static const char loc[] = "evloop.c:1";
#define LOC loc, (int64_t)(sizeof loc - 1)
#define MS 1000000LL

typedef struct {
	int64_t kind, arg;
	void *obj;
} job;

enum { J_SLEEP, J_ECHO_ONE, J_WRITE, J_READ_ONCE, J_ACCEPT_ONCE };

static job jobs[1 << 14];
static int64_t njobs;

static int64_t mkjob(int64_t kind, int64_t arg, void *obj) {
	int64_t i = __atomic_fetch_add(&njobs, 1, __ATOMIC_RELAXED);
	jobs[i] = (job){kind, arg, obj};
	return i;
}

static unsigned char pattern(int64_t i) { return (unsigned char)(i * 7 + (i >> 9)); }

// echo copies everything conn c sends back to it, until the end of the
// stream
static void echo(xo_conn *c) {
	for (;;) {
		xo_rlist *l = xo_conn_read(c, 65536);
		if (xo_net_status() != 0 || l->len == 0) {
			if (l != &empty_list) {
				drop_str_list(l);
			}
			return;
		}
		xo_rstr *s = ((xo_rstr **)rl_data(l))[0];
		int64_t n = ((int64_t *)rl_data(l))[1];
		xo_conn_write(c, s->data, n);
		drop_str_list(l);
		if (xo_net_status() != 0) {
			return;
		}
	}
}

static void run_job(xo_rlist *fl, xo_task *t) {
	job j = jobs[first(fl)];
	drop_int_list(fl);
	int64_t res = 0;
	switch (j.kind) {
	case J_SLEEP:
		xo_clock_sleep(j.arg * MS);
		res = 1;
		break;
	case J_ECHO_ONE: {
		xo_conn *c = xo_listener_accept(j.obj);
		CHECK(xo_net_status() == 0);
		echo(c);
		xo_conn_close(c);
		release(c);
		break;
	}
	case J_WRITE: {
		// writes arg bytes of the pattern, then half closes by closing
		char *buf = malloc((size_t)j.arg);
		for (int64_t i = 0; i < j.arg; i++) {
			buf[i] = (char)pattern(i);
		}
		xo_conn_write(j.obj, buf, j.arg);
		res = xo_net_status();
		free(buf);
		break;
	}
	case J_READ_ONCE: {
		xo_rlist *l = xo_conn_read(j.obj, 16);
		res = xo_net_status();
		if (l != &empty_list) {
			drop_str_list(l);
		}
		break;
	}
	case J_ACCEPT_ONCE: {
		xo_conn *c = xo_listener_accept(j.obj);
		res = xo_net_status();
		release(c);
		break;
	}
	}
	if (t_unwinding()) {
		t_task_unwound(t);
	} else {
		xo_rlist *r = one(res);
		rc_mark(&r->rc);
		t_task_done(t, r, 0);
	}
	release(t);
}

static xo_task *spawn(xo_scope *s, int64_t kind, int64_t arg, void *obj, int64_t flags) {
	int64_t id = mkjob(kind, arg, obj);
	return t_task_spawn(s, one(id), (int64_t)run_job, (int64_t)drop_int_list, (int64_t)drop_int_list, flags, 0, LOC);
}

static int64_t await_int(xo_task *t) {
	xo_rlist *r = t_task_await(t, LOC);
	int64_t v = first(r);
	drop_int_list(r);
	release(t);
	return v;
}

// ---------------------------------------------------------------------------

static void test_timers(void) {
	int64_t start = mono_ns();
	xo_clock_sleep(10 * MS);
	CHECK(mono_ns() - start >= 10 * MS);
	// many sleepers at once park; the workers stay few
	start = mono_ns();
	xo_scope *s = t_scope_open();
	enum { SLEEPERS = 2000 };
	xo_task **ts = malloc(SLEEPERS * sizeof *ts);
	for (int i = 0; i < SLEEPERS; i++) {
		ts[i] = spawn(s, J_SLEEP, 1 + i % 20, NULL, 2);
	}
	int64_t done = 0;
	for (int i = 0; i < SLEEPERS; i++) {
		done += await_int(ts[i]);
	}
	free(ts);
	CHECK(t_scope_close(s, 0) == 0);
	release(s);
	CHECK(done == SLEEPERS);
	CHECK(mono_ns() - start >= 20 * MS);
	// the pool record's alive and ncpu words (pool.xo P_ALIVE, P_NCPU)
	int64_t *pw = pool_rec();
	CHECK(LOAD(pw[4]) <= pw[1] + 1);
	// a canceled sleeper wakes and unwinds
	start = mono_ns();
	s = t_scope_open();
	release(spawn(s, J_SLEEP, 3600 * 1000, NULL, 1));
	xo_clock_sleep(5 * MS);
	CHECK(t_scope_close(s, 0) == 0);
	release(s);
	CHECK(mono_ns() - start < 10000 * MS);
	// select with only `after`
	xo_select *sel = t_select_new();
	xo_select_after(sel, 5 * MS);
	CHECK(t_select_wait(sel, LOC) == -2);
	release(sel);
	fputs("timers: ok\n", stderr);
}

static void test_echo(void) {
	static const char a[] = "127.0.0.1:0";
	xo_listener *l = xo_net_listen("", 0, a, (int64_t)sizeof a - 1);
	CHECK(xo_net_status() == 0);
	xo_rlist *al = xo_listener_addr(l);
	xo_rstr *as = ((xo_rstr **)rl_data(al))[0];
	int64_t an = ((int64_t *)rl_data(al))[1];
	CHECK(an > 10 && memcmp(as->data, "127.0.0.1:", 10) == 0);
	xo_scope *s = t_scope_open();
	release(spawn(s, J_ECHO_ONE, 0, l, 0));
	xo_conn *c = xo_net_dial("", 0, as->data, an);
	CHECK(xo_net_status() == 0);
	drop_str_list(al);
	// more than the socket buffers hold: the writer parks until the echo
	// and this reader drain them
	enum { BYTES = 8 << 20 };
	xo_task *w = spawn(s, J_WRITE, BYTES, c, 2);
	int64_t got = 0, bad = 0;
	while (got < BYTES) {
		xo_rlist *r = xo_conn_read(c, 1 << 20);
		if (xo_net_status() != 0 || r->len == 0) {
			break;
		}
		xo_rstr *rs = ((xo_rstr **)rl_data(r))[0];
		int64_t n = ((int64_t *)rl_data(r))[1];
		for (int64_t i = 0; i < n; i++) {
			bad += (unsigned char)rs->data[i] != pattern(got + i);
		}
		got += n;
		drop_str_list(r);
	}
	CHECK(await_int(w) == 0);
	CHECK(got == BYTES && bad == 0);
	xo_conn_close(c);
	CHECK(t_scope_close(s, 0) == 0);
	release(s);
	release(c);
	release(l);
	fputs("echo: ok\n", stderr);
}

static void test_wakeups(void) {
	static const char a[] = "127.0.0.1:0";
	xo_listener *l = xo_net_listen("", 0, a, (int64_t)sizeof a - 1);
	xo_rlist *al = xo_listener_addr(l);
	xo_rstr *as = ((xo_rstr **)rl_data(al))[0];
	int64_t an = ((int64_t *)rl_data(al))[1];
	xo_conn *c = xo_net_dial("", 0, as->data, an);
	CHECK(xo_net_status() == 0);
	xo_conn *peer = xo_listener_accept(l);
	CHECK(xo_net_status() == 0);
	// close wakes a parked read: Io
	xo_scope *s = t_scope_open();
	xo_task *t = spawn(s, J_READ_ONCE, 0, c, 2);
	xo_clock_sleep(5 * MS);
	xo_conn_close(c);
	CHECK(await_int(t) == 4);
	CHECK(t_scope_close(s, 0) == 0);
	release(s);
	// cancellation wakes a parked accept and read: Canceled (the read is
	// on a second connection: peer saw c close)
	xo_conn *c2 = xo_net_dial("", 0, as->data, an);
	xo_conn *peer2 = xo_listener_accept(l);
	CHECK(xo_net_status() == 0);
	s = t_scope_open();
	xo_task *ta = spawn(s, J_ACCEPT_ONCE, 0, l, 2);
	xo_task *tr = spawn(s, J_READ_ONCE, 0, peer2, 2);
	xo_clock_sleep(5 * MS);
	t_scope_cancel(s);
	CHECK(t_scope_close(s, 0) == 0);
	CHECK(__atomic_load_n(&ta->state, __ATOMIC_ACQUIRE) == T_DONE && __atomic_load_n(&tr->state, __ATOMIC_ACQUIRE) == T_DONE);
	CHECK(ta->result && first(ta->result) == 5);
	CHECK(tr->result && first(tr->result) == 5);
	release(ta);
	release(tr);
	release(s);
	release(peer);
	release(peer2);
	release(c2);
	release(c);
	// a closed listener refuses
	xo_listener_close(l);
	xo_conn *r = xo_net_dial("", 0, as->data, an);
	CHECK(xo_net_status() == 2);
	release(r);
	drop_str_list(al);
	release(l);
	fputs("wakeups: ok\n", stderr);
}

// test_inherited frees the descriptor of a pipe whose read end a child
// process also holds, then makes it readable: no event may reach the
// freed xo_pd. epoll keys a registration by the open file, so a close
// alone left it registered while the child held the file, and the
// poller's next event named freed memory (ThreadSanitizer: "use of an
// invalid mutex" in pd_ready, linux/arm64, 2026-10-12, gotcha
// linux-epoll-inherited-socket). Sockets and process pipes share the path.
static void test_inherited(void) {
#ifdef _WIN32
	// Windows (decision 0197): pipes are not polled (a pump thread joins
	// them to a socket, xo_proc.c), and the AFD poll of a socket ends when
	// the socket closes; there is no registration to outlive it
	fputs("inherited: ok (nothing to test on Windows)\n", stderr);
#else
	int p[2];
	CHECK(pipe(p) == 0);
	// the child inherits both ends: neither is close-on-exec yet
	extern char **environ;
	char *argv[] = {"sleep", "0.3", NULL};
	pid_t pid = -1;
	CHECK(posix_spawnp(&pid, "sleep", NULL, NULL, argv, environ) == 0);
	set_nonblock(p[0]);
	xo_pd *pd = pd_new(p[0], 0);
	CHECK(pd != NULL);
	pd_free(pd);
	// the poller frees it at the top of its next turn
	ev_poke();
	usleep(50000);
	// readable now, through the file the child still holds open
	CHECK(write(p[1], "x", 1) == 1);
	usleep(50000);
	ev_poke();
	usleep(20000);
	if (pid > 0) {
		waitpid(pid, NULL, 0);
	}
	close(p[1]);
	fputs("inherited: ok\n", stderr);
#endif
}

static void leaks(const char *name) {
	int64_t live, total;
	t_rt_live(&live, &total);
	if (live != 0) {
		fprintf(stderr, "%s: %lld live of %lld allocations\n", name, (long long)live, (long long)total);
		failures++;
	}
}

int main(void) {
	test_timers();
	leaks("timers");
	test_echo();
	leaks("echo");
	test_wakeups();
	leaks("wakeups");
	test_inherited();
	leaks("inherited");
	return failures != 0;
}
