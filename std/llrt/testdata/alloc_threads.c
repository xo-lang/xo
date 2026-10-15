// alloc_threads.c: a stress test of the native heap (decision 0207 stage
// 3: std/native/llrt/alloc.xo through the C ABI exports; decision 0198
// deleted the C path, -DXO_ALLOC_C), run by internal/llvm's
// TestRuntimeAlloc under ThreadSanitizer (clang -fsanitize=thread) and
// optimized without it.
// It includes the runtime and calls its entry points the way the rest of
// the C runtime does: xo_alloc, xo_free, xo_resize, rs_new.
//
// Every block is filled with a pattern derived from a tag (its thread,
// its slot, a sequence number) and checked before it is resized or
// freed, so two blocks handed out over each other, a block reused while
// still in use, or a resize that loses bytes show up as a mismatch.
//
//   solo      one thread before xo_rt_threads_begin (solo_heap): random
//             sizes over every class, the mid classes and malloc, random
//             frees and resizes, the usable size of rs_new
//   threads   NT threads with their own heaps: the same, plus blocks sent
//             to a random other thread's mailbox, which checks and frees
//             (or resizes, then frees) them, so frees cross threads and
//             the magazines and central lists move blocks back
//   waves     one thread allocates many blocks, two new threads free
//             them, round after round (the decision 0118 pattern), with
//             the chunks carved bounded
//
// At the end of each test every allocation must be freed (the leak
// check's counts over all heaps). Exit status 0 means every test passed.

#define XO_RT_NO_MAIN
#include "../xo_rt.c"
#include <pthread.h> // the test's own threads (xo_rt.c no longer includes it)
#include "xcalls.h"

#ifndef ITER
#define ITER 3000
#endif
#define NT 8
#define SLOTS 64
#define MAILBOX 4096

static int failures;

#define CHECK(c)                                                                     \
	do {                                                                             \
		if (!(c)) {                                                                  \
			fprintf(stderr, "%s:%d: check failed: %s\n", __FILE__, __LINE__, #c);    \
			__atomic_fetch_add(&failures, 1, __ATOMIC_RELAXED);                      \
		}                                                                            \
	} while (0)

// rnd is a xorshift64 step.
static uint64_t rnd(uint64_t *s) {
	uint64_t x = *s;
	x ^= x << 13;
	x ^= x >> 7;
	x ^= x << 17;
	*s = x;
	return x;
}

// a_size draws a size: mostly small classes, some mid classes, a few
// blocks from malloc (above 256 KiB), and sizes 0 to 7.
static int64_t a_size(uint64_t *s) {
	uint64_t r = rnd(s) % 1000;
	if (r < 20) {
		return (int64_t)(rnd(s) % 8);
	}
	if (r < 850) {
		return 8 + (int64_t)(rnd(s) % 1017);
	}
	if (r < 990) {
		return 1025 + (int64_t)(rnd(s) % (255 << 10));
	}
	return (256 << 10) + 1 + (int64_t)(rnd(s) % (768 << 10));
}

// The pattern of a block of n bytes with tag t: every byte up to 4 KiB
// and then one byte in every 4 KiB page and the last byte.
static uint8_t pat(uint64_t t, int64_t i) { return (uint8_t)((t * 0x9E3779B97F4A7C15ull + (uint64_t)i * 31) >> 56); }

static void fill(uint8_t *p, int64_t n, uint64_t t) {
	int64_t full = n < 4096 ? n : 4096;
	for (int64_t i = 0; i < full; i++) {
		p[i] = pat(t, i);
	}
	for (int64_t i = 4096; i < n; i += 4096) {
		p[i] = pat(t, i);
	}
	if (n > 0) {
		p[n - 1] = pat(t, n - 1);
	}
}

// check reports whether the first n bytes of p (a block filled for m
// bytes, n <= m) still hold the pattern of t.
static int check(const uint8_t *p, int64_t n, int64_t m, uint64_t t) {
	int64_t full = n < 4096 ? n : 4096;
	for (int64_t i = 0; i < full; i++) {
		if (p[i] != pat(t, i)) {
			return 0;
		}
	}
	for (int64_t i = 4096; i < n; i += 4096) {
		if (p[i] != pat(t, i)) {
			return 0;
		}
	}
	if (n > 0 && n == m && p[n - 1] != pat(t, n - 1)) {
		return 0;
	}
	return 1;
}

typedef struct {
	uint8_t *p;
	int64_t n;
	uint64_t tag;
} blk;

// put allocates a block of n bytes for b and fills it.
static void put(blk *b, int64_t n, uint64_t tag) {
	b->p = t_alloc(n);
	CHECK(((uintptr_t)b->p & 7) == 0);
	b->n = n;
	b->tag = tag;
	fill(b->p, n, tag);
}

// drop checks and frees b's block.
static void drop(blk *b) {
	CHECK(check(b->p, b->n, b->n, b->tag));
	t_free(b->p);
	b->p = NULL;
}

// grow resizes b's block to n bytes, checks what it kept, and refills it.
static void grow(blk *b, int64_t n, uint64_t tag) {
	CHECK(check(b->p, b->n, b->n, b->tag));
	b->p = t_resize(b->p, b->n, n);
	int64_t kept = b->n < n ? b->n : n;
	CHECK(check(b->p, kept, b->n, b->tag));
	b->n = n;
	b->tag = tag;
	fill(b->p, n, tag);
}

// churn runs iters random operations on the slots of one thread; send
// (when not null) takes some blocks away to another thread.
static void churn(blk *slots, uint64_t *s, uint64_t id, int iters, void (*send)(blk *, uint64_t *)) {
	for (int k = 0; k < iters; k++) {
		blk *b = &slots[rnd(s) % SLOTS];
		uint64_t tag = (id << 40) ^ ((uint64_t)k << 8) ^ (uint64_t)(b - slots);
		uint64_t r = rnd(s) % 100;
		if (!b->p) {
			put(b, a_size(s), tag);
		} else if (r < 40) {
			drop(b);
		} else if (r < 60) {
			grow(b, a_size(s), tag);
		} else if (r < 75 && send) {
			send(b, s);
		} else if (r < 85) {
			// a string's capacity is the usable size of its block
			int64_t want = (int64_t)(rnd(s) % 2000);
			xo_rstr *o = t_rs_new(want);
			CHECK(o->cap >= want);
			memset(o->data, 0x5a, (size_t)o->cap);
			t_free(o);
		} else {
			CHECK(check(b->p, b->n, b->n, b->tag));
		}
	}
	for (int i = 0; i < SLOTS; i++) {
		if (slots[i].p) {
			drop(&slots[i]);
		}
	}
}

static int leaks(const char *name) {
	int64_t live, total;
	t_rt_live(&live, &total);
	CHECK(live == 0);
	printf("%s: %s (%lld allocations, %lld live)\n", name, failures ? "FAIL" : "ok", (long long)total, (long long)live);
	return live == 0;
}

static void test_solo(void) {
	blk slots[SLOTS];
	memset(slots, 0, sizeof slots);
	uint64_t s = 0x1234567890abcdefull;
	churn(slots, &s, 1, ITER * 4, NULL);
}

// Mailboxes: blocks sent to a thread, which checks and frees them.
static struct {
	pthread_mutex_t mu;
	blk q[MAILBOX];
	int n;
} boxes[NT];

static void box_send(blk *b, uint64_t *s) {
	int to = (int)(rnd(s) % NT);
	pthread_mutex_lock(&boxes[to].mu);
	if (boxes[to].n < MAILBOX) {
		boxes[to].q[boxes[to].n++] = *b;
		b->p = NULL;
	}
	pthread_mutex_unlock(&boxes[to].mu);
	if (b->p) {
		drop(b); // the box is full
	}
}

// box_drain checks and frees (some after a resize) what thread id got.
static void box_drain(int id, uint64_t *s) {
	blk got[MAILBOX];
	pthread_mutex_lock(&boxes[id].mu);
	int n = boxes[id].n;
	memcpy(got, boxes[id].q, (size_t)n * sizeof(blk));
	boxes[id].n = 0;
	pthread_mutex_unlock(&boxes[id].mu);
	for (int i = 0; i < n; i++) {
		if (rnd(s) % 4 == 0) {
			grow(&got[i], a_size(s), got[i].tag ^ 0xff);
		}
		drop(&got[i]);
	}
}

static void send_and_drain(blk *b, uint64_t *s);
static __thread int my_id;

static void send_and_drain(blk *b, uint64_t *s) {
	box_send(b, s);
	if (rnd(s) % 8 == 0) {
		box_drain(my_id, s);
	}
}

static void *thread_main(void *arg) {
	int id = (int)(intptr_t)arg;
	my_id = id;
	blk slots[SLOTS];
	memset(slots, 0, sizeof slots);
	uint64_t s = 0x9e3779b97f4a7c15ull * (uint64_t)(id + 1);
	churn(slots, &s, (uint64_t)id + 2, ITER, send_and_drain);
	box_drain(id, &s);
	return NULL;
}

static void test_threads(void) {
	for (int i = 0; i < NT; i++) {
		pthread_mutex_init(&boxes[i].mu, NULL);
		boxes[i].n = 0;
	}
	pthread_t ts[NT];
	for (int i = 0; i < NT; i++) {
		CHECK(pthread_create(&ts[i], NULL, thread_main, (void *)(intptr_t)i) == 0);
	}
	for (int i = 0; i < NT; i++) {
		pthread_join(ts[i], NULL);
	}
	// what was sent after its thread's last drain
	uint64_t s = 77;
	for (int i = 0; i < NT; i++) {
		box_drain(i, &s);
	}
}

// Waves: the main thread (a heap that lives on) allocates many blocks of
// one class, two new threads free them all at once, round after round
// (the decision 0118 pattern: a server's accept loop and its connection
// tasks). The freeing threads' full lists go to the central list as
// magazines and come back to the main heap; without them each round
// would carve new chunks.
#define WAVE 7000
static void *wave_blocks[2][WAVE];

static void *wave_free(void *arg) {
	int k = (int)(intptr_t)arg;
	for (int i = 0; i < WAVE; i++) {
		CHECK(((uint8_t *)wave_blocks[k][i])[0] == (uint8_t)(k + 1));
		t_free(wave_blocks[k][i]);
	}
	return NULL;
}

static void test_waves(void) {
	int rounds = ITER / 100 + 4;
	int64_t before = 0;
	for (int r = 0; r < rounds; r++) {
		if (r == 2) {
			before = xo_chunks_now();
		}
		for (int k = 0; k < 2; k++) {
			for (int i = 0; i < WAVE; i++) {
				uint8_t *p = t_alloc(56);
				memset(p, k + 1, 56);
				wave_blocks[k][i] = p;
			}
		}
		pthread_t ts[2];
		for (int k = 0; k < 2; k++) {
			CHECK(pthread_create(&ts[k], NULL, wave_free, (void *)(intptr_t)k) == 0);
		}
		for (int k = 0; k < 2; k++) {
			pthread_join(ts[k], NULL);
		}
	}
	// each round allocates 896 KB, a chunk every 4.7 rounds without the
	// central lists; a freeing thread keeps at most two magazines (128
	// KB) in the heap it leaves behind, a chunk every 16 rounds for the
	// two (2 in 28 rounds, 19 in 300 measured)
	int n = rounds - 2;
	int64_t grew = xo_chunks_now() - before;
	CHECK(grew <= n / 10 + 3);
	fprintf(stderr, "waves: %lld chunks carved in %d rounds\n", (long long)grew, n);
}

int main(void) {
	test_solo();
	leaks("solo");
	xo_rt_threads_begin();
	test_threads();
	leaks("threads");
	test_waves();
	leaks("waves");
	return failures ? 1 : 0;
}
