// realtime.c: a test of real-time sections (decision 0133), run by
// internal/llvm's TestRuntimeRealtime under ThreadSanitizer and optimized
// without it. It includes the runtime and plays the part of generated
// code: xo_realtime_enter and xo_realtime_exit bracket a section, and
// releases go through t_rc_free, t_rl_free, and t_rc_defer as the
// generated drop paths do.
//
// Tests: frees inside a section are deferred to its end (the live count
// does not move until then), nested sections drain only at the outermost
// exit, a queue of more than XO_RTQ frees overflows into frees done at
// once, allocations inside a section are counted, and threads keep
// separate sections. Every allocation must be freed. Exit status 0 means
// every test passed.

#define XO_RT_NO_MAIN
#include "../xo_rt.c"
#include "xcalls.h"

static int failures;

#define CHECK(c)                                                                  \
	do {                                                                          \
		if (!(c)) {                                                               \
			fprintf(stderr, "%s:%d: check failed: %s\n", __FILE__, __LINE__, #c); \
			__atomic_fetch_add(&failures, 1, __ATOMIC_RELAXED);                   \
		}                                                                         \
	} while (0)

static int64_t live(void) {
	int64_t l, t;
	t_rt_live(&l, &t);
	return l;
}

// a string object as generated code holds it
static xo_rstr *mkstr(const char *s) {
	int64_t n = (int64_t)strlen(s);
	xo_rstr *o = rs_new(n);
	memcpy(o->data, s, (size_t)n);
	return o;
}

static void release_str(xo_rstr *o) {
	if (rc_dec(&o->rc)) {
		t_rc_free(o);
	}
}

static void str_drop(void *p) { release_str(*(xo_rstr **)p); }

// a List[Str] of n strings
static xo_rlist *mklist(int n) {
	xo_rlist *l = rl_new(n, sizeof(xo_rstr *));
	for (int i = 0; i < n; i++) {
		((xo_rstr **)rl_data(l))[i] = mkstr("element");
	}
	l->len = n;
	return l;
}

static void release_list(xo_rlist *l) {
	if (rc_dec(&l->rc)) {
		t_rl_free(l, sizeof(xo_rstr *), str_drop);
	}
}

// a closure environment holding one string, freed through t_rc_defer
typedef struct {
	int64_t rc;
	xo_rstr *s;
} env;

static void env_drop(void *p) {
	env *e = p;
	release_str(e->s);
	t_rc_free(e);
}

static void test_defer(void) {
	int64_t base = live();
	xo_rstr *s = mkstr("old value");
	xo_rlist *l = mklist(50);
	env *e = t_rc_alloc(sizeof(env));
	e->rc = 1;
	e->s = mkstr("captured");
	int64_t before = live();
	CHECK(before == base + 54);
	xo_realtime_enter();
	release_str(s);
	release_list(l);
	if (rc_dec(&e->rc)) {
		t_rc_defer(e, env_drop);
	}
	CHECK(live() == before); // nothing freed inside the section
	CHECK(heap()->rt_n == 3);
	xo_realtime_exit("defer", 5, "realtime.c:1", 12);
	CHECK(live() == base);
	CHECK(heap()->rt_n == 0);
}

static void test_nested(void) {
	int64_t base = live();
	xo_rstr *a = mkstr("a"), *b = mkstr("b");
	xo_realtime_enter();
	release_str(a);
	xo_realtime_enter();
	release_str(b);
	xo_realtime_exit("inner", 5, "realtime.c:2", 12);
	CHECK(live() == base + 2); // the inner exit does not drain
	xo_realtime_exit("outer", 5, "realtime.c:3", 12);
	CHECK(live() == base);
}

static void test_overflow(void) {
	int64_t base = live();
	enum { N = XO_RTQ + 44 };
	static xo_rstr *ss[N];
	for (int i = 0; i < N; i++) {
		ss[i] = mkstr("x");
	}
	xo_realtime_enter();
	for (int i = 0; i < N; i++) {
		release_str(ss[i]);
	}
	CHECK(heap()->rt_n == XO_RTQ);
	CHECK(live() == base + XO_RTQ); // the rest were freed at once
	xo_realtime_exit("overflow", 8, "realtime.c:4", 12);
	CHECK(live() == base);
}

static void test_allocs(void) {
	int64_t base = live();
	xo_realtime_enter();
	xo_rstr *s = mkstr("allocated inside");
	CHECK(heap()->rt_allocs == 1);
	release_str(s);
	heap()->rt_allocs = 0; // the exit would fault with XO-F007
	xo_realtime_exit("allocs", 6, "realtime.c:5", 12);
	CHECK(live() == base);
}

// threads: each runs sections of its own; a section on one thread does
// not defer the frees of another
static void *section_thread(void *arg) {
	(void)arg;
	for (int i = 0; i < 200; i++) {
		xo_rlist *l = mklist(8);
		xo_realtime_enter();
		release_list(l);
		CHECK(heap()->rt_n == 1);
		xo_realtime_exit("thread", 6, "realtime.c:6", 12);
		CHECK(heap()->rt_n == 0);
	}
	return NULL;
}

static void test_threads(void) {
	pthread_t ts[4];
	for (int i = 0; i < 4; i++) {
		pthread_create(&ts[i], NULL, section_thread, NULL);
	}
	section_thread(NULL);
	for (int i = 0; i < 4; i++) {
		pthread_join(ts[i], NULL);
	}
}

int main(void) {
	struct {
		const char *name;
		void (*fn)(void);
	} tests[] = {
		{"defer", test_defer}, {"nested", test_nested}, {"overflow", test_overflow}, {"allocs", test_allocs},
	};
	for (size_t i = 0; i < sizeof tests / sizeof tests[0]; i++) {
		int before = failures;
		tests[i].fn();
		printf("%s: %s\n", tests[i].name, failures == before ? "ok" : "FAIL");
	}
	xo_rt_threads_begin();
	int before = failures;
	test_threads();
	CHECK(live() == 0);
	printf("threads: %s\n", failures == before ? "ok" : "FAIL");
	return failures ? 1 : 0;
}
