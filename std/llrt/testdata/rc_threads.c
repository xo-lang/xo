// rc_threads.c: a stress test of the runtime's reference counts shared
// between threads (decision 0058), run by internal/llvm's
// TestRuntimeThreads under ThreadSanitizer (clang -fsanitize=thread) and
// optimized without it. It includes the runtime and plays the part of
// generated code: the inline count paths of internal/llvm and
// internal/arm64 are xo_rc_inc and xo_rc_dec here, the share walkers are
// written by hand, and pthreads stand in for tasks (tasks.c tests the
// task runtime itself).
//
// Each test hands shared values to several threads, which retain,
// release, copy on write, append to, and drop them; at the end every
// allocation must be freed (the leak check counts of all heaps) and the
// values must be intact. Exit status 0 means every test passed.

#define XO_RT_NO_MAIN
#include "../xo_rt.c"

#include <assert.h>

#ifndef ITER
#define ITER 2000
#endif
#define NT 8

static int failures;

#define CHECK(c)                                                          \
	do {                                                                  \
		if (!(c)) {                                                       \
			fprintf(stderr, "%s:%d: check failed: %s\n", __FILE__, __LINE__, #c); \
			__atomic_fetch_add(&failures, 1, __ATOMIC_RELAXED);          \
		}                                                                 \
	} while (0)

// ---------------------------------------------------------------------------
// Values: strings and lists of strings, as generated code lays them out

typedef struct {
	xo_rstr *obj;
	int64_t len;
} str;

static str mkstr(const char *s) {
	int64_t n = (int64_t)strlen(s);
	xo_rstr *o = rs_new(n);
	memcpy(o->data, s, (size_t)n);
	return (str){o, n};
}

static void str_dup(void *p) { xo_rc_inc(&((str *)p)->obj->rc); }

static void str_drop(void *p) {
	xo_rstr *o = ((str *)p)->obj;
	if (xo_rc_dec(&o->rc)) {
		xo_rc_free(o);
	}
}

static void str_share(void *p) { xo_rc_mark(&((str *)p)->obj->rc); }

static str *elems(xo_rlist *l) { return (str *)rl_data(l); }

static xo_rlist *list_of(int n) {
	xo_rlist *l = xo_rl_new(n, sizeof(str));
	for (int i = 0; i < n; i++) {
		char b[32];
		snprintf(b, sizeof b, "item-%d", i);
		elems(l)[i] = mkstr(b);
	}
	l->len = n;
	return l;
}

static void list_drop(xo_rlist *l) {
	if (xo_rc_dec(&l->rc)) {
		xo_rl_free(l, sizeof(str), str_drop);
	}
}

// the share walker of List[Str] (generated code: xo.share.List[Str])
static void list_share(xo_rlist *l) {
	if (xo_rc_mark(&l->rc)) {
		xo_rl_share(l, sizeof(str), str_share);
	}
}

static int list_ok(xo_rlist *l, int n) {
	if (l->len != n) {
		return 0;
	}
	for (int i = 0; i < n; i++) {
		char b[32];
		int k = snprintf(b, sizeof b, "item-%d", i);
		if (elems(l)[i].len != k || memcmp(elems(l)[i].obj->data, b, (size_t)k) != 0) {
			return 0;
		}
	}
	return 1;
}

static void run(void *(*fn)(void *), void *arg) {
	pthread_t ts[NT];
	for (int i = 0; i < NT; i++) {
		if (pthread_create(&ts[i], NULL, fn, arg) != 0) {
			fputs("pthread_create failed\n", stderr);
			exit(1);
		}
	}
	for (int i = 0; i < NT; i++) {
		pthread_join(ts[i], NULL);
	}
}

// ---------------------------------------------------------------------------
// 1. A shared string: every thread holds one reference (given before it
// starts, as a spawn capture would) and takes and drops more; the last
// release frees it.

static str shared_s;

static void *t_string(void *arg) {
	(void)arg;
	for (int i = 0; i < ITER; i++) {
		xo_rc_inc(&shared_s.obj->rc);
		xo_rc_inc(&shared_s.obj->rc);
		CHECK(shared_s.len == 5 && memcmp(shared_s.obj->data, "hello", 5) == 0);
		str_drop(&shared_s);
		str_drop(&shared_s);
	}
	str_drop(&shared_s); // this thread's reference
	return NULL;
}

static void test_string(void) {
	shared_s = mkstr("hello");
	xo_rc_mark(&shared_s.obj->rc);
	for (int i = 1; i < NT; i++) {
		xo_rc_inc(&shared_s.obj->rc); // one per thread; the creator's goes to the last
	}
	run(t_string, NULL);
}

// ---------------------------------------------------------------------------
// 2. A shared List[Str]: threads read it, retain and drop elements, and
// update their own copy (`reuse`: xo_rl_copy copies a list others hold,
// dup retaining the shared elements), then drop everything.

static xo_rlist *shared_l;
#define NL 64

static void *t_list(void *arg) {
	(void)arg;
	for (int i = 0; i < ITER / 8; i++) {
		xo_rc_inc(&shared_l->rc); // a copy of the value, about to be updated
		xo_rlist *mine = xo_rl_copy(shared_l, sizeof(str), str_dup, str_drop);
		CHECK(mine != shared_l);
		CHECK(mine->rc == 1);
		// mine[3] = "changed": release the old element, store a new one
		str_drop(&elems(mine)[3]);
		elems(mine)[3] = mkstr("changed");
		CHECK(list_ok(shared_l, NL));
		str e = elems(shared_l)[7];
		str_dup(&e);
		CHECK(e.len == 6);
		str_drop(&e);
		list_drop(mine);
	}
	list_drop(shared_l);
	return NULL;
}

static void test_list(void) {
	shared_l = list_of(NL);
	list_share(shared_l);
	CHECK(shared_l->rc == -1 && elems(shared_l)[0].obj->rc == -1);
	for (int i = 1; i < NT; i++) {
		xo_rc_inc(&shared_l->rc);
	}
	run(t_list, NULL);
}

// ---------------------------------------------------------------------------
// 3. The claim: every thread updates the shared list through `reuse`;
// all but the last see other holders and copy, while the last one (count
// -1, read with acquire order) may update the original in place. Each
// ends with a unique list it drops.

static int in_place;

static void *t_claim(void *arg) {
	(void)arg;
	xo_rlist *mine = shared_l;
	if (rc_load(&mine->rc) != 1) { // the inline check of `reuse` (an unordered load)
		mine = xo_rl_copy(shared_l, sizeof(str), str_dup, str_drop);
	}
	if (mine == shared_l) {
		__atomic_fetch_add(&in_place, 1, __ATOMIC_RELAXED);
	}
	CHECK(mine->rc == 1);
	CHECK(list_ok(mine, NL));
	str_drop(&elems(mine)[0]);
	elems(mine)[0] = mkstr("item-0");
	list_drop(mine);
	return NULL;
}

static void test_claim(void) {
	for (int round = 0; round < ITER / 20; round++) {
		shared_l = list_of(NL);
		list_share(shared_l);
		for (int i = 1; i < NT; i++) {
			xo_rc_inc(&shared_l->rc);
		}
		in_place = 0;
		run(t_claim, NULL);
		CHECK(in_place <= 1);
	}
}

// ---------------------------------------------------------------------------
// 4. Appending to a shared string (`s + x` consumes s): the copy path
// drops the shared reference, and the last thread frees the original.

static void *t_append(void *arg) {
	(void)arg;
	for (int i = 0; i < ITER; i++) {
		str bang = mkstr("!");
		xo_rc_inc(&shared_s.obj->rc);
		xo_rstr *r = xo_rs_concat(shared_s.obj, shared_s.len, bang.obj, 1);
		CHECK(r != shared_s.obj);
		CHECK(memcmp(r->data, "hello!", 6) == 0);
		str rs = {r, 6};
		str_drop(&rs);
		str_drop(&bang);
	}
	// this thread's own reference: the last thread to get here may
	// append in place (the claim), the others copy
	xo_rstr *r = rs_append(shared_s.obj, shared_s.len, "?", 1);
	CHECK(r->rc == 1 && memcmp(r->data, "hello?", 6) == 0);
	str rs = {r, 6};
	str_drop(&rs);
	return NULL;
}

static void test_append(void) {
	shared_s = mkstr("hello");
	xo_rc_mark(&shared_s.obj->rc);
	for (int i = 1; i < NT; i++) {
		xo_rc_inc(&shared_s.obj->rc);
	}
	run(t_append, NULL);
}

// ---------------------------------------------------------------------------
// 5. Boxes: a chain of counted cells {rc, next}, freed and marked shared
// through xo_rc_defer's worklist (generated code: boxdrop and boxshare).

typedef struct box {
	int64_t rc;
	struct box *next;
	int64_t v;
} box;

static void box_free(void *p);

static void box_release(box *b) {
	if (b && xo_rc_dec(&b->rc)) {
		xo_rc_defer(b, box_free);
	}
}

static void box_free(void *p) {
	box *b = p;
	box_release(b->next);
	xo_rc_free(b);
}

static void box_share_contents(void *p);

static void box_share(box *b) {
	if (b && xo_rc_mark(&b->rc)) {
		xo_rc_defer(b, box_share_contents);
	}
}

static void box_share_contents(void *p) { box_share(((box *)p)->next); }

static box *shared_b;
#define NB 10000

static void *t_box(void *arg) {
	(void)arg;
	int64_t sum = 0;
	for (box *b = shared_b; b; b = b->next) {
		sum += b->v;
	}
	CHECK(sum == (int64_t)NB * (NB - 1) / 2);
	// hold a reference to the middle of the chain, drop the head
	box *mid = shared_b;
	for (int i = 0; i < NB / 2; i++) {
		mid = mid->next;
	}
	xo_rc_inc(&mid->rc);
	box_release(shared_b);
	CHECK(mid->v == NB / 2);
	box_release(mid);
	return NULL;
}

static void test_box(void) {
	box *head = NULL;
	for (int i = NB - 1; i >= 0; i--) {
		box *b = xo_rc_alloc(sizeof(box));
		b->rc = 1;
		b->next = head;
		b->v = i;
		head = b;
	}
	shared_b = head;
	box_share(shared_b);
	CHECK(shared_b->rc == -1 && shared_b->next->next->rc == -1);
	for (int i = 1; i < NT; i++) {
		xo_rc_inc(&shared_b->rc);
	}
	run(t_box, NULL);
}

// ---------------------------------------------------------------------------
// 6. Cross thread frees: a producer allocates thread local strings,
// marks them shared, and sends them through a mutex queue (a channel);
// consumers drop them on their own heaps.

#define QN 64
static str q[QN];
static int qh, qt, qdone;
static pthread_mutex_t qmu = PTHREAD_MUTEX_INITIALIZER;
static pthread_cond_t qcv = PTHREAD_COND_INITIALIZER;

static void *t_producer(void *arg) {
	(void)arg;
	for (int i = 0; i < ITER * 4; i++) {
		char b[32];
		snprintf(b, sizeof b, "msg-%d", i);
		str s = mkstr(b);
		xo_rc_mark(&s.obj->rc); // crossing to another thread
		pthread_mutex_lock(&qmu);
		while (qt - qh == QN) {
			pthread_cond_wait(&qcv, &qmu);
		}
		q[qt % QN] = s;
		qt++;
		pthread_cond_broadcast(&qcv);
		pthread_mutex_unlock(&qmu);
	}
	pthread_mutex_lock(&qmu);
	qdone = 1;
	pthread_cond_broadcast(&qcv);
	pthread_mutex_unlock(&qmu);
	return NULL;
}

static void *t_consumer(void *arg) {
	(void)arg;
	for (;;) {
		pthread_mutex_lock(&qmu);
		while (qh == qt && !qdone) {
			pthread_cond_wait(&qcv, &qmu);
		}
		if (qh == qt) {
			pthread_mutex_unlock(&qmu);
			return NULL;
		}
		str s = q[qh % QN];
		qh++;
		pthread_cond_broadcast(&qcv);
		pthread_mutex_unlock(&qmu);
		CHECK(s.len >= 5 && memcmp(s.obj->data, "msg-", 4) == 0);
		str_drop(&s);
	}
}

static void test_queue(void) {
	pthread_t p, c[3];
	pthread_create(&p, NULL, t_producer, NULL);
	for (int i = 0; i < 3; i++) {
		pthread_create(&c[i], NULL, t_consumer, NULL);
	}
	pthread_join(p, NULL);
	for (int i = 0; i < 3; i++) {
		pthread_join(c[i], NULL);
	}
}

// ---------------------------------------------------------------------------
// 7. Literals: immortal objects every thread retains and releases at
// once; their counts must never be written (TSan reports any write).

static struct {
	int64_t rc, cap;
	char data[8];
} lit = {XO_RC_IMMORTAL, 4, "lit!"};

static void *t_literal(void *arg) {
	(void)arg;
	for (int i = 0; i < ITER; i++) {
		xo_rc_inc(&lit.rc);
		CHECK(!xo_rc_dec(&lit.rc));
		CHECK(!xo_rc_claim(&lit.rc));
		CHECK(!xo_rc_mark(&lit.rc));
		// `"lit!" + x` copies and leaves the literal alone
		xo_rstr *r = rs_append((xo_rstr *)&lit, 4, "?", 1);
		CHECK(r != (xo_rstr *)&lit && r->rc == 1);
		xo_rc_free(r);
	}
	return NULL;
}

static void test_literal(void) {
	run(t_literal, NULL);
	CHECK(lit.rc == XO_RC_IMMORTAL);
}

// ---------------------------------------------------------------------------

int main(void) {
	xo_rt_threads_begin();
	struct {
		const char *name;
		void (*fn)(void);
	} tests[] = {
		{"string", test_string}, {"list", test_list}, {"claim", test_claim}, {"append", test_append},
		{"box", test_box}, {"queue", test_queue}, {"literal", test_literal},
	};
	for (size_t i = 0; i < sizeof tests / sizeof tests[0]; i++) {
		int before = failures;
		tests[i].fn();
		int64_t live, total;
		xo_rt_live(&live, &total);
		CHECK(live == 0);
		printf("%s: %s (%lld allocations, %lld live)\n", tests[i].name, failures == before ? "ok" : "FAIL", (long long)total,
			   (long long)live);
	}
	return failures ? 1 : 0;
}
