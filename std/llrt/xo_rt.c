// xo_rt.c: the runtime of the LLVM backend (internal/llvm, `xo build
// --backend=llvm`). It is compiled by clang together with the generated
// LLVM IR (with -flto, so the small helpers inline into Xo code).
//
// Contents:
//   - memory: a size class allocator with free lists (16 byte classes up
//     to 1 KiB, powers of two up to 256 KiB, malloc above), and a count
//     of live allocations that the leak check reads (XO_LEAKCHECK=1
//     prints it at exit).
//   - reference counted values (decision 0046), used by code lowered from
//     internal/ssa: every heap object starts with an i64 count; strings
//     are {count, cap, bytes} (Str values are {object, len}), lists are
//     {count, len, cap} and the elements. Generated code increments
//     and decrements counts inline and calls xo_rc_free, xo_rl_free
//     (which releases the elements through a generated callback),
//     xo_rl_copy (a shared list about to be updated), and xo_rl_grow (a
//     unique full list, resized in place).
//   - counts shared between threads (decision 0058): a positive count
//     is thread local (plain increments), a negative count -n is shared
//     (atomic), XO_RC_IMMORTAL marks literals (never written). See
//     "Reference counts" below.
//   - closure environments and boxes (decision 0048): freed through
//     xo_rc_defer, a worklist, so long chains do not recurse.
//   - strings: comparison, concatenation, and number formatting.
//   - output: buffered stdout, flushed at exit and before a fault.
//   - faults: messages and exit status 70, matching the Go backend's text.
//   - tasks (decision 0068): scopes, tasks, cancellation, channels,
//     Mutex, and Atomic on a pool of worker threads (see "Tasks").
//   - main: runs the Xo entry on a thread with a 1 GiB stack, like Go's
//     maximum goroutine stack, so deep recursion behaves alike. Built
//     with -DXO_RT_TEST_THREADS=N it runs the entry on N threads at once
//     (a test of the runtime's thread safety, rt_test.go).
//
// The interface to generated code uses only scalars and pointers (no
// structs by value), so it does not depend on the C ABI of aggregates.

#include <math.h>
#include <pthread.h>
#include <sched.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#define XO_NORETURN __attribute__((noreturn, noinline, cold))

// ---------------------------------------------------------------------------
// Memory
//
// Every block has an 8 byte prefix before the pointer handed out: the
// size class (1 to XO_NCLASS) or, for a block from malloc, minus its
// size. Freed blocks of a class go on that class's free list (the next
// pointer is stored in the block); new ones are carved from 4 MiB chunks.
//
// Threads (decision 0058): the free lists, the chunk, the counters, and
// xo_rc_defer's worklist live in a heap. While the program has one
// thread every call uses solo_heap (main_heap), so single threaded code
// pays one load and a predicted branch.
// xo_rt_threads_begin, called before a second thread runs Xo code,
// switches to one heap per thread: the calling thread keeps main_heap,
// every other thread gets its own on first use. A block may be freed by
// another thread than the one that allocated it: it goes on the freeing
// thread's list (the size class is in its prefix), and the leak check
// sums the counters of all heaps. Heaps are never freed.

#define XO_CHUNK (4 << 20)
#define XO_SMALL_MAX 1024
#define XO_MID_MAX (256 << 10)
#define XO_NCLASS (64 + 8) // 16..1024 by 16, then 2 KiB..256 KiB

typedef void (*xo_drop_fn)(void *);
typedef struct {
	void *obj;
	xo_drop_fn drop;
} xo_deferred;

typedef struct xo_heap {
	// blocks allocated, and blocks freed, by this heap's thread (the
	// leak check reports the difference): one counter per call
	int64_t allocs;
	int64_t frees;
	char *bump_cur;
	char *bump_end;
	void *free_lists[XO_NCLASS + 1];
	// the worklist of xo_rc_defer
	xo_deferred *defer_q;
	int64_t defer_n, defer_cap;
	int defer_busy;
	struct xo_heap *next;  // all heaps, for the leak check
	struct xo_heap *spare; // the list of heaps of exited threads
} xo_heap;

static xo_heap main_heap;
static int xo_threaded; // set once, before a second thread runs Xo code
// solo_heap is main_heap while one thread runs, null after (one load
// both tells the case and gives the heap)
static xo_heap *solo_heap = &main_heap;
static __thread xo_heap *tls_heap;
static xo_heap *all_heaps = &main_heap;
// spare_heaps are the heaps of worker threads that exited (decision
// 0068): a new thread takes one before it makes a heap, so a program that
// starts and ends many threads does not keep a heap per thread
static xo_heap *spare_heaps;
static pthread_mutex_t heaps_mu = PTHREAD_MUTEX_INITIALIZER;

XO_NORETURN static void xo_oom(void) {
	fputs("fault: out of memory\n", stderr);
	exit(70);
}

__attribute__((noinline)) static xo_heap *heap_slow(void) {
	xo_heap *h = tls_heap;
	if (h) {
		return h;
	}
	pthread_mutex_lock(&heaps_mu);
	h = spare_heaps;
	if (h) {
		spare_heaps = h->spare;
		h->spare = NULL;
		pthread_mutex_unlock(&heaps_mu);
		tls_heap = h;
		return h;
	}
	pthread_mutex_unlock(&heaps_mu);
	h = calloc(1, sizeof(xo_heap));
	if (!h) {
		xo_oom();
	}
	pthread_mutex_lock(&heaps_mu);
	h->next = all_heaps;
	all_heaps = h;
	pthread_mutex_unlock(&heaps_mu);
	tls_heap = h;
	return h;
}

// heap_release hands the calling thread's heap to the next thread that
// starts (a worker thread that exits; its blocks stay counted).
static void heap_release(void) {
	xo_heap *h = tls_heap;
	if (!h || h == &main_heap) {
		return;
	}
	tls_heap = NULL;
	pthread_mutex_lock(&heaps_mu);
	h->spare = spare_heaps;
	spare_heaps = h;
	pthread_mutex_unlock(&heaps_mu);
}

// heap returns the calling thread's heap.
static inline xo_heap *heap(void) {
	if (__builtin_expect(!__atomic_load_n(&xo_threaded, __ATOMIC_RELAXED), 1)) {
		return &main_heap;
	}
	return heap_slow();
}

// xo_rt_threads_begin switches the runtime to several threads. The
// thread that ran so far calls it before it starts another thread
// (thread creation orders it before everything the new thread does).
void xo_rt_threads_begin(void) {
	if (__atomic_load_n(&xo_threaded, __ATOMIC_RELAXED)) {
		return;
	}
	tls_heap = &main_heap;
	__atomic_store_n(&solo_heap, NULL, __ATOMIC_RELAXED);
	__atomic_store_n(&xo_threaded, 1, __ATOMIC_RELAXED);
}

// xo_rt_live returns the blocks allocated and not freed, and the blocks
// allocated, over all heaps (for the leak check, when no other thread
// runs).
void xo_rt_live(int64_t *live, int64_t *total) {
	int64_t l = 0, t = 0;
	pthread_mutex_lock(&heaps_mu);
	for (xo_heap *h = all_heaps; h; h = h->next) {
		l += h->allocs - h->frees;
		t += h->allocs;
	}
	pthread_mutex_unlock(&heaps_mu);
	*live = l;
	*total = t;
}

static inline int64_t size_class(int64_t n, int64_t *size) {
	if (n <= XO_SMALL_MAX) {
		int64_t c = (n + 15) >> 4;
		if (c == 0) {
			c = 1;
		}
		*size = c << 4;
		return c;
	}
	int64_t c = 65, s = 2048;
	while (s < n) {
		s <<= 1;
		c++;
	}
	*size = s;
	return c;
}

__attribute__((noinline)) static void *xo_alloc_large(int64_t n) {
	int64_t *b = malloc((size_t)n + 8);
	if (!b) {
		xo_oom();
	}
	b[0] = -n;
	return b + 1;
}

__attribute__((noinline)) static void xo_refill(xo_heap *h, int64_t need) {
	char *c = malloc(XO_CHUNK);
	if (!c) {
		xo_oom();
	}
	(void)need;
	h->bump_cur = c;
	h->bump_end = c + XO_CHUNK;
}

static inline __attribute__((always_inline)) void *alloc_in(xo_heap *h, int64_t n) {
	h->allocs++;
	if (n > XO_MID_MAX) {
		return xo_alloc_large(n);
	}
	int64_t size;
	int64_t c = size_class(n, &size);
	void *p = h->free_lists[c];
	if (p) {
		h->free_lists[c] = *(void **)p;
		return p;
	}
	int64_t need = size + 8;
	if (h->bump_end - h->bump_cur < need) {
		xo_refill(h, need);
	}
	int64_t *b = (int64_t *)h->bump_cur;
	h->bump_cur += need;
	b[0] = c;
	return b + 1;
}

static inline __attribute__((always_inline)) void free_in(xo_heap *h, void *p) {
	h->frees++;
	int64_t *b = (int64_t *)p - 1;
	if (b[0] < 0) {
		free(b);
		return;
	}
	*(void **)p = h->free_lists[b[0]];
	h->free_lists[b[0]] = p;
}

// The threaded paths of xo_alloc and xo_free are tail calls, so the
// single threaded paths stay leaf functions without a frame.
__attribute__((noinline)) static void *alloc_mt(int64_t n) { return alloc_in(heap_slow(), n); }

__attribute__((noinline)) static void free_mt(void *p) { free_in(heap_slow(), p); }

void *xo_alloc(int64_t n) {
	xo_heap *h = __atomic_load_n(&solo_heap, __ATOMIC_RELAXED);
	if (__builtin_expect(!h, 0)) {
		return alloc_mt(n);
	}
	return alloc_in(h, n);
}

void xo_free(void *p) {
	xo_heap *h = __atomic_load_n(&solo_heap, __ATOMIC_RELAXED);
	if (__builtin_expect(!h, 0)) {
		free_mt(p);
		return;
	}
	free_in(h, p);
}

// xo_resize returns a block of n bytes with the first old bytes of p (a
// block of old bytes, which is freed).
static void *xo_resize(void *p, int64_t old, int64_t n) {
	int64_t *b = (int64_t *)p - 1;
	if (b[0] < 0 && n > XO_MID_MAX) {
		int64_t *nb = realloc(b, (size_t)n + 8);
		if (!nb) {
			xo_oom();
		}
		nb[0] = -n;
		return nb + 1;
	}
	if (b[0] > 0) {
		int64_t size;
		if (n <= XO_MID_MAX && size_class(n, &size) == b[0]) {
			return p;
		}
	}
	void *q = xo_alloc(n);
	memcpy(q, p, (size_t)(old < n ? old : n));
	xo_free(p);
	return q;
}

void *xo_rc_alloc(int64_t n) { return xo_alloc(n); }

void xo_rc_free(void *p) { xo_free(p); }

// xo_rc_defer runs drop(obj) for an object whose count reached zero
// (a closure environment or a box). drop releases the object's children
// and frees it; a child that reaches zero meanwhile is queued instead of
// dropped recursively, so freeing a long chain (a list built from boxes,
// closures capturing closures) uses a worklist, not the C stack. Marking
// a value shared uses the same worklist (drop is then the function that
// marks the children of a box or an environment). The worklist is the
// calling thread's.
void xo_rc_defer(void *obj, xo_drop_fn drop) {
	xo_heap *h = heap();
	if (h->defer_busy) {
		if (h->defer_n == h->defer_cap) {
			h->defer_cap = h->defer_cap < 64 ? 64 : h->defer_cap * 2;
			h->defer_q = realloc(h->defer_q, (size_t)h->defer_cap * sizeof(xo_deferred));
			if (!h->defer_q) {
				xo_oom();
			}
		}
		h->defer_q[h->defer_n].obj = obj;
		h->defer_q[h->defer_n].drop = drop;
		h->defer_n++;
		return;
	}
	h->defer_busy = 1;
	drop(obj);
	while (h->defer_n > 0) {
		h->defer_n--;
		h->defer_q[h->defer_n].drop(h->defer_q[h->defer_n].obj);
	}
	h->defer_busy = 0;
}

// ---------------------------------------------------------------------------
// Reference counts (decisions 0046 and 0058)
//
// The first word of every counted object is its count c:
//
//   c > 0           thread local, c references: plain increments and
//                   decrements (only one thread can reach the object)
//   c = -n, n > 0   shared, n references: atomic read-modify-writes
//   XO_RC_IMMORTAL  a literal: never written
//
// Bit 62 tells shared counts (every negative count of magnitude below
// 2^62 has it) from XO_RC_IMMORTAL (INT64_MIN, which does not). A value
// becomes shared when it crosses to another thread (spawn captures, a
// channel send, a Mutex or Atomic store): xo_rc_mark and the generated
// `share` helpers negate the counts of every thread local object it
// reaches, stopping at objects already shared, so a shared object only
// points to shared or immortal objects. Generated code inlines the same
// tests: a retain is `c > 0 ? c + 1 : slow`, a release is
// `c > 1 ? c - 1 : slow`, so the thread local paths cost one more
// predicted branch (retain) or nothing (release: it already compared
// with one) over plain counts. Retaining a shared object is a relaxed
// atomic decrement; releasing it is an atomic increment with acquire
// and release order, so the thread that takes it to zero sees every
// other thread's writes before it frees (Rust's Arc uses a release
// decrement and an acquire fence on the last one; ThreadSanitizer does
// not model fences, and on arm64 both are one ldaddal).
//
// In place updates (Perceus reuse) need the only reference. A thread
// local count of 1 proves it; a shared count of -1 does too once read
// with acquire order (every other thread's release happened before),
// and the object then goes back to thread local (xo_rc_claim).

#define XO_RC_IMMORTAL INT64_MIN
#define XO_RC_SHARED_BIT ((int64_t)1 << 62)

static inline int64_t rc_load(int64_t *p) { return __atomic_load_n(p, __ATOMIC_RELAXED); }

// xo_rc_inc adds a reference.
static inline __attribute__((always_inline)) void rc_inc(int64_t *p) {
	int64_t c = rc_load(p);
	if (__builtin_expect(c > 0, 1)) {
		*p = c + 1;
		return;
	}
	if (c & XO_RC_SHARED_BIT) {
		__atomic_fetch_sub(p, 1, __ATOMIC_RELAXED);
	}
}

void xo_rc_inc(int64_t *p) { rc_inc(p); }

__attribute__((noinline)) static int rc_dec_shared(int64_t *p) {
	// acquire and release in one: the thread that frees sees every other
	// thread's writes (a release decrement plus an acquire fence would do
	// too, but ThreadSanitizer does not model fences)
	if (__atomic_fetch_add(p, 1, __ATOMIC_ACQ_REL) == -1) {
		return 1;
	}
	return 0;
}

// xo_rc_dec drops a reference and reports whether it was the last one
// (the caller then releases the children and frees the object). The
// thread local and immortal cases are inline (the slow atomic path is
// not), so dropping a literal costs a few compares.
static inline __attribute__((always_inline)) int rc_dec(int64_t *p) {
	int64_t c = rc_load(p);
	if (__builtin_expect(c > 1, 1)) {
		*p = c - 1;
		return 0;
	}
	if (c == 1) {
		return 1;
	}
	if (!(c & XO_RC_SHARED_BIT)) {
		return 0; // immortal
	}
	return rc_dec_shared(p);
}

int xo_rc_dec(int64_t *p) { return rc_dec(p); }

// xo_rc_claim reports whether the caller holds the only reference, so
// the object may be updated in place; a shared object is made thread
// local again.
static inline __attribute__((always_inline)) int rc_claim(int64_t *p) {
	int64_t c = rc_load(p);
	if (c == 1) {
		return 1;
	}
	if (c == -1 && __atomic_load_n(p, __ATOMIC_ACQUIRE) == -1) {
		__atomic_store_n(p, 1, __ATOMIC_RELAXED);
		return 1;
	}
	return 0;
}

int xo_rc_claim(int64_t *p) { return rc_claim(p); }

// xo_rc_mark makes the count at p shared and reports whether it was
// thread local (the caller then marks the object's children).
int xo_rc_mark(int64_t *p) {
	int64_t c = rc_load(p);
	if (c > 0) {
		__atomic_store_n(p, -c, __ATOMIC_RELAXED);
		return 1;
	}
	return 0;
}

// ---------------------------------------------------------------------------
// Output

static char obuf[1 << 16];
static size_t olen;

static void write_all(int fd, const char *p, size_t n) {
	while (n > 0) {
		ssize_t w = write(fd, p, n);
		if (w <= 0) {
			return;
		}
		p += w;
		n -= (size_t)w;
	}
}

// With several threads (xo_rt_threads_begin) the buffer is locked; one
// print's text stays together.
static pthread_mutex_t out_mu = PTHREAD_MUTEX_INITIALIZER;

static inline int out_lock(void) {
	if (__builtin_expect(!__atomic_load_n(&xo_threaded, __ATOMIC_RELAXED), 1)) {
		return 0;
	}
	pthread_mutex_lock(&out_mu);
	return 1;
}

static inline void out_unlock(int locked) {
	if (locked) {
		pthread_mutex_unlock(&out_mu);
	}
}

static void flush_locked(void) {
	if (olen > 0) {
		write_all(1, obuf, olen);
		olen = 0;
	}
}

static void print_locked(const char *p, int64_t n) {
	if ((size_t)n > sizeof obuf / 2) {
		flush_locked();
		write_all(1, p, (size_t)n);
		return;
	}
	if (olen + (size_t)n > sizeof obuf) {
		flush_locked();
	}
	memcpy(obuf + olen, p, (size_t)n);
	olen += (size_t)n;
}

void xo_flush(void) {
	int l = out_lock();
	flush_locked();
	out_unlock(l);
}

void xo_print(const char *p, int64_t n) {
	int l = out_lock();
	print_locked(p, n);
	out_unlock(l);
}

void xo_println(const char *p, int64_t n) {
	int l = out_lock();
	print_locked(p, n);
	print_locked("\n", 1);
	out_unlock(l);
}

void xo_eprint(const char *p, int64_t n) {
	int l = out_lock();
	flush_locked();
	write_all(2, p, (size_t)n);
	out_unlock(l);
}

void xo_eprintln(const char *p, int64_t n) {
	int l = out_lock();
	flush_locked();
	write_all(2, p, (size_t)n);
	write_all(2, "\n", 1);
	out_unlock(l);
}

// ---------------------------------------------------------------------------
// Number formatting (the Go backend's text: strconv.FormatInt and
// FloatStr, the shortest 'g' form that reads back the same value, with
// ".0" added to whole numbers)

// fmt_int writes the decimal text of v (no snprintf: interpolation of
// Ints is hot).
static int fmt_int(char *buf, int64_t v) {
	char t[24];
	int n = 0;
	uint64_t u = v < 0 ? 0 - (uint64_t)v : (uint64_t)v;
	do {
		t[n++] = (char)('0' + u % 10);
		u /= 10;
	} while (u);
	int k = 0;
	if (v < 0) {
		buf[k++] = '-';
	}
	while (n) {
		buf[k++] = t[--n];
	}
	buf[k] = 0;
	return k;
}

// fmt_float writes Go's strconv.FormatFloat(f, 'g', -1, 64) to buf.
static int fmt_go_g(char *buf, double f) {
	if (isnan(f)) {
		return snprintf(buf, 40, "NaN");
	}
	if (isinf(f)) {
		return snprintf(buf, 40, f > 0 ? "+Inf" : "-Inf");
	}
	if (f == 0) {
		return snprintf(buf, 40, signbit(f) ? "-0" : "0");
	}
	// shortest digits that round trip
	char e[40];
	int p;
	for (p = 0; p < 17; p++) {
		snprintf(e, sizeof e, "%.*e", p, f);
		if (strtod(e, NULL) == f) {
			break;
		}
	}
	// e is [-]d[.ddd]e(+|-)XX
	int neg = e[0] == '-';
	char *s = e + neg;
	char digs[24];
	int nd = 0;
	char *q = s;
	for (; *q && *q != 'e'; q++) {
		if (*q >= '0' && *q <= '9') {
			digs[nd++] = *q;
		}
	}
	int x = atoi(q + 1);
	while (nd > 1 && digs[nd - 1] == '0') {
		nd--;
	}
	digs[nd] = 0;
	int dp = x + 1; // decimal point position: value = 0.digs * 10^dp
	int exp = dp - 1;
	char *o = buf;
	if (neg) {
		*o++ = '-';
	}
	if (exp < -4 || exp >= 6) {
		*o++ = digs[0];
		if (nd > 1) {
			*o++ = '.';
			for (int i = 1; i < nd; i++) {
				*o++ = digs[i];
			}
		}
		*o++ = 'e';
		int ax = exp;
		if (ax < 0) {
			*o++ = '-';
			ax = -ax;
		} else {
			*o++ = '+';
		}
		o += snprintf(o, 8, "%02d", ax);
		*o = 0;
		return (int)(o - buf);
	}
	// %f with max(nd - dp, 0) decimals
	if (dp <= 0) {
		*o++ = '0';
		*o++ = '.';
		for (int i = 0; i < -dp; i++) {
			*o++ = '0';
		}
		for (int i = 0; i < nd; i++) {
			*o++ = digs[i];
		}
	} else {
		for (int i = 0; i < dp; i++) {
			*o++ = i < nd ? digs[i] : '0';
		}
		if (nd > dp) {
			*o++ = '.';
			for (int i = dp; i < nd; i++) {
				*o++ = digs[i];
			}
		}
	}
	*o = 0;
	return (int)(o - buf);
}

static int fmt_float(char *buf, double f) {
	int n = fmt_go_g(buf, f);
	if (!strpbrk(buf, ".eIN")) {
		buf[n++] = '.';
		buf[n++] = '0';
		buf[n] = 0;
	}
	return n;
}

// ---------------------------------------------------------------------------
// Faults (core 8): the same messages as std/rt, exit status 70

XO_NORETURN static void fault_text(const char *msg, const char *loc) {
	xo_flush();
	char *buf = malloc(strlen(msg) + strlen(loc) + 32);
	int n = sprintf(buf, "fault: %s at %s\n", msg, loc);
	write_all(2, buf, (size_t)n);
	exit(70);
}

XO_NORETURN void xo_fault(const char *msg, int64_t n, const char *loc) {
	char *m = malloc((size_t)n + 1);
	memcpy(m, msg, (size_t)n);
	m[n] = 0;
	fault_text(m, loc);
}

XO_NORETURN void xo_fault_overflow(int64_t a, int64_t op, int64_t b, const char *loc) {
	char m[160];
	snprintf(m, sizeof m, "integer overflow: %lld %c %lld does not fit Int", (long long)a, (char)op, (long long)b);
	fault_text(m, loc);
}

XO_NORETURN void xo_fault_shl(int64_t a, int64_t b, const char *loc) {
	char m[160];
	snprintf(m, sizeof m, "integer overflow: %lld << %lld does not fit Int", (long long)a, (long long)b);
	fault_text(m, loc);
}

XO_NORETURN void xo_fault_shift(int64_t n, const char *loc) {
	char m[160];
	snprintf(m, sizeof m, "shift amount %lld out of range for Int", (long long)n);
	fault_text(m, loc);
}

XO_NORETURN void xo_fault_divzero(int64_t a, int64_t op, const char *loc) {
	char m[160];
	snprintf(m, sizeof m, "division by zero: %lld %c 0", (long long)a, (char)op);
	fault_text(m, loc);
}

XO_NORETURN void xo_fault_neg(int64_t a, const char *loc) {
	char m[160];
	snprintf(m, sizeof m, "integer overflow: -(%lld) does not fit Int", (long long)a);
	fault_text(m, loc);
}

XO_NORETURN void xo_fault_abs(int64_t a, const char *loc) {
	char m[160];
	snprintf(m, sizeof m, "integer overflow: abs(%lld) does not fit Int", (long long)a);
	fault_text(m, loc);
}

XO_NORETURN void xo_fault_index(int64_t i, int64_t n, const char *loc) {
	char m[160];
	snprintf(m, sizeof m, "index %lld out of range for List of length %lld", (long long)i, (long long)n);
	fault_text(m, loc);
}

XO_NORETURN void xo_fault_float_int(double f, const char *loc) {
	char m[160];
	if (isnan(f)) {
		snprintf(m, sizeof m, "cannot convert NaN to Int");
	} else {
		char b[40];
		fmt_go_g(b, f);
		snprintf(m, sizeof m, "%s is out of range for Int", b);
	}
	fault_text(m, loc);
}

// ---------------------------------------------------------------------------
// Strings

int32_t xo_str_eq(const char *a, int64_t an, const char *b, int64_t bn) {
	return an == bn && (a == b || memcmp(a, b, (size_t)an) == 0);
}

int64_t xo_str_cmp(const char *a, int64_t an, const char *b, int64_t bn) {
	int64_t n = an < bn ? an : bn;
	int c = memcmp(a, b, (size_t)n);
	if (c != 0) {
		return c < 0 ? -1 : 1;
	}
	return an < bn ? -1 : (an > bn ? 1 : 0);
}

// ---------------------------------------------------------------------------
// Reference counted strings and lists (code lowered from internal/ssa)

typedef struct {
	int64_t rc, cap;
	char data[];
} xo_rstr;

// usable returns the bytes a block requested with n bytes really has
// (its size class), so strings can grow into the slack.
static int64_t usable(int64_t n) {
	if (n > XO_MID_MAX) {
		return n;
	}
	int64_t size;
	size_class(n, &size);
	return size;
}

static xo_rstr *rs_new(int64_t n) {
	int64_t bytes = usable((int64_t)sizeof(xo_rstr) + n);
	xo_rstr *s = xo_alloc(bytes);
	s->rc = 1;
	s->cap = bytes - (int64_t)sizeof(xo_rstr);
	return s;
}

// rs_append appends n bytes at p to a (of length an), consuming a: when
// a is unique (xo_rc_claim) the bytes go in place, growing a by doubling
// when it is full (Perceus reuse of a string about to be dropped), so a
// loop of `s = s + x` is linear. Otherwise a new string is made (with
// slack to grow into) and a loses the reference the caller gave up.
static xo_rstr *rs_append(xo_rstr *a, int64_t an, const char *p, int64_t n) {
	int64_t need = an + n;
	if (rc_claim(&a->rc)) {
		if (a->cap < need) {
			int64_t want = a->cap * 2;
			if (want < need) {
				want = need;
			}
			int64_t hdr = (int64_t)sizeof(xo_rstr);
			int64_t bytes = usable(hdr + want);
			a = xo_resize(a, hdr + an, bytes);
			a->cap = bytes - hdr;
		}
		memmove(a->data + an, p, (size_t)n);
		return a;
	}
	xo_rstr *s = rs_new(need < 16 ? 16 : need);
	memcpy(s->data, a->data, (size_t)an);
	memcpy(s->data + an, p, (size_t)n);
	// a loses the reference the caller gave up: it is a literal (common:
	// `"w${i}"` appends to one; tested first, which measured faster), or
	// it has others, or it is shared and freed here when the others went
	// meanwhile
	int64_t c = rc_load(&a->rc);
	if (c == XO_RC_IMMORTAL) {
		return s;
	}
	if (c > 1) {
		a->rc = c - 1;
	} else if (c == 1 || rc_dec_shared(&a->rc)) {
		xo_free(a);
	}
	return s;
}

// xo_rs_concat is `a + b` (consumes a, borrows b).
xo_rstr *xo_rs_concat(xo_rstr *a, int64_t an, xo_rstr *b, int64_t bn) {
	return rs_append(a, an, b->data, bn);
}

// xo_rs_concat_int is `a + "${v}"` without a temporary string.
xo_rstr *xo_rs_concat_int(xo_rstr *a, int64_t an, int64_t v, int64_t *len) {
	char t[32];
	int n = fmt_int(t, v);
	*len = an + n;
	return rs_append(a, an, t, n);
}

// xo_rs_concat_float is `a + "${v}"` for a Float.
xo_rstr *xo_rs_concat_float(xo_rstr *a, int64_t an, double v, int64_t *len) {
	char t[48];
	int n = fmt_float(t, v);
	*len = an + n;
	return rs_append(a, an, t, n);
}

xo_rstr *xo_rs_of_int(int64_t v, int64_t *len) {
	char t[32];
	int n = fmt_int(t, v);
	xo_rstr *s = rs_new(n);
	memcpy(s->data, t, (size_t)n);
	*len = n;
	return s;
}

xo_rstr *xo_rs_of_float(double v, int64_t *len) {
	char t[48];
	int n = fmt_float(t, v);
	xo_rstr *s = rs_new(n);
	memcpy(s->data, t, (size_t)n);
	*len = n;
	return s;
}

typedef struct {
	int64_t rc, len, cap;
} xo_rlist;

typedef void (*xo_elem_fn)(void *);

static inline char *rl_data(xo_rlist *l) { return (char *)(l + 1); }

xo_rlist *xo_rl_new(int64_t cap, int64_t esize) {
	xo_rlist *l = xo_alloc((int64_t)sizeof(xo_rlist) + cap * esize);
	l->rc = 1;
	l->len = 0;
	l->cap = cap;
	return l;
}

void xo_rl_free(xo_rlist *l, int64_t esize, xo_elem_fn drop);

// xo_rl_copy is the slow path of `reuse`: l's count is not 1. A shared
// list held only by the caller (xo_rc_claim) is updated in place.
// Otherwise the update goes to a copy, which holds its own references to
// the elements (dup), and l loses the one the caller gave up (freed with
// drop when the other references went meanwhile).
__attribute__((noinline)) xo_rlist *xo_rl_copy(xo_rlist *l, int64_t esize, xo_elem_fn dup, xo_elem_fn drop) {
	if (xo_rc_claim(&l->rc)) {
		return l;
	}
	int64_t cap = l->len < 4 ? 4 : l->len;
	xo_rlist *n = xo_rl_new(cap, esize);
	n->len = l->len;
	memcpy(rl_data(n), rl_data(l), (size_t)(l->len * esize));
	if (dup) {
		for (int64_t i = 0; i < l->len; i++) {
			dup(rl_data(n) + i * esize);
		}
	}
	if (xo_rc_dec(&l->rc)) {
		xo_rl_free(l, esize, drop);
	}
	return n;
}

// xo_rl_share marks the elements of a list that was just marked shared
// (share is the generated element callback; boxes and environments
// below go through xo_rc_defer's worklist, so the C stack is bounded by
// the nesting of types, not of values).
void xo_rl_share(xo_rlist *l, int64_t esize, xo_elem_fn share) {
	for (int64_t i = 0; i < l->len; i++) {
		share(rl_data(l) + i * esize);
	}
}

// xo_rl_grow doubles the capacity of a unique full list.
__attribute__((noinline)) xo_rlist *xo_rl_grow(xo_rlist *l, int64_t esize) {
	int64_t cap = l->cap < 4 ? 4 : l->cap * 2;
	int64_t hdr = (int64_t)sizeof(xo_rlist);
	xo_rlist *n = xo_resize(l, hdr + l->cap * esize, hdr + cap * esize);
	n->cap = cap;
	return n;
}

// xo_rl_free frees a list whose count reached zero, releasing its
// elements first.
void xo_rl_free(xo_rlist *l, int64_t esize, xo_elem_fn drop) {
	if (drop) {
		for (int64_t i = 0; i < l->len; i++) {
			drop(rl_data(l) + i * esize);
		}
	}
	xo_free(l);
}

// ---------------------------------------------------------------------------
// Tasks (decision 0068): structured concurrency on threads
//
// Scopes, tasks, channels, Mutex, and Atomic of core 7 and std.md 2.7 for
// the native backends. Generated code calls the functions below with
// scalars and pointers only; values cross into the runtime as lists of
// one element (a `List[T]` is one pointer whatever T is), so the runtime
// never needs a value's layout. A container that may hold values when it
// is freed (a task's result, a channel's buffer, a mutex's value) gets a
// typed release function from generated code (an entry function that
// takes the list and releases it).
//
//   - Handles. Task, Scope (and task.Group, the same object), Chan,
//     Mutex, and Atomic are reference counted objects {count, free, ...}
//     created shared (count -1): they cross threads by design. Generated
//     code retains and releases them like lists and calls xo_h_free at
//     zero.
//   - Threads. A pool of worker threads, started on the first spawn, one
//     per CPU (XO_THREADS overrides). Each worker has a deque (the owner
//     pushes and pops at the bottom, others steal from the top); spawns
//     from other threads go to a shared inject queue. A worker that blocks
//     (a channel, a lock, await, a scope join) counts as blocked, and when
//     work is queued and fewer than one worker per CPU can run, another
//     worker starts, so blocked tasks never starve queued ones (the
//     managed blocking of Java's ForkJoinPool). Workers beyond one per CPU
//     exit when they find no work; their heaps go to the next thread.
//   - Helping. `t.await` on a task that has not started runs it on the
//     awaiting thread, and a scope's join runs its tasks that have not
//     started before it blocks.
//   - Cancellation. Every task and scope body is a node of a tree (under
//     one lock, tree_mu); canceling a node marks its subtree canceled
//     (sticky) and wakes every context blocked at a node in it. A
//     context's node is its innermost scope body, or its task.
//   - Unwinding. A cancellation point (send, recv, await, waiting for a
//     lock) in a canceled context sets the context's unwinding flag and
//     returns an immortal placeholder; generated code checks xo_unwinding
//     after each call that may unwind and returns, closing the scopes it
//     leaves, up to the task's entry (or to a scope whose child failed,
//     where the unwinding becomes that failure).
//   - Waiting. Every context has a park (a mutex, a condition variable,
//     and a flag); objects keep lists of the parks of their waiters and
//     signal them, and cancellation signals the park registered at a
//     canceled node.

#include <sys/types.h>

typedef void (*xo_free_fn)(void *);
// xo_ldrop releases a list of one element of a known type (an entry
// function of generated code)
typedef void (*xo_ldrop)(xo_rlist *);

typedef struct {
	int64_t rc;
	xo_free_fn free;
} xo_hdr;

// xo_h_free frees a handle whose count reached zero.
void xo_h_free(void *p) { ((xo_hdr *)p)->free(p); }

static void h_init(void *p, xo_free_fn f) {
	xo_hdr *h = p;
	h->rc = -1;
	h->free = f;
}

static void h_retain(void *p) { rc_inc(&((xo_hdr *)p)->rc); }

static void h_release(void *p) {
	if (rc_dec(&((xo_hdr *)p)->rc)) {
		xo_h_free(p);
	}
}

// The placeholders a canceled operation returns while it unwinds (never
// written: their counts are immortal).
static xo_hdr zero_handle = {XO_RC_IMMORTAL, NULL};
static xo_rlist empty_list = {XO_RC_IMMORTAL, 0, 0};

void *xo_h_zero(void) { return &zero_handle; }

xo_rlist *xo_rl_empty(void) { return &empty_list; }

static void list_release(xo_rlist *l, xo_ldrop drop) {
	if (l && l != &empty_list) {
		drop(l);
	}
}

static char *cstr_of(const char *p, int64_t n) {
	char *s = malloc((size_t)n + 1);
	if (!s) {
		xo_oom();
	}
	memcpy(s, p, (size_t)n);
	s[n] = 0;
	return s;
}

XO_NORETURN static void fault_at(const char *msg, const char *loc, int64_t n) { fault_text(msg, cstr_of(loc, n)); }

// ---------------------------------------------------------------------------
// Parks, nodes, contexts

typedef struct {
	pthread_mutex_t mu;
	pthread_cond_t cv;
	int signaled;
	int counted; // its context counts as blocked (dl.blocked)
} xo_park;

// xo_spin is a lock for short critical sections that many threads take
// (the deadlock counters, the cancellation tree): a contended pthread
// mutex sleeps in the kernel, which dominated spawn heavy programs.
typedef struct {
	int locked;
} xo_spin;

static inline void spin_relax(void) {
#if defined(__aarch64__)
	__asm__ volatile("yield");
#elif defined(__x86_64__)
	__asm__ volatile("pause");
#endif
}

static void spin_lock(xo_spin *l) {
	for (int n = 0;; n++) {
		if (!__atomic_load_n(&l->locked, __ATOMIC_RELAXED) && !__atomic_exchange_n(&l->locked, 1, __ATOMIC_ACQUIRE)) {
			return;
		}
		if (n > 64) {
			sched_yield();
		} else {
			spin_relax();
		}
	}
}

static void spin_unlock(xo_spin *l) { __atomic_store_n(&l->locked, 0, __ATOMIC_RELEASE); }

// Deadlock detection (core 11.4, fault XO-F001): live counts the
// contexts that run Xo code (each program root, and each task a worker
// took from a queue; a task run by a waiting context stands in for it),
// blocked those waiting in park_wait. A context counts as running again
// as soon as a waker signals it. With no task queued, every live context
// blocked means none can ever be woken: the native backends have no
// timers, I/O, or signals that could.
static struct {
	xo_spin mu;
	int64_t live, blocked;
} dl;

static void dl_check_locked(void);

// mu_init initializes a mutex of the task runtime. On Darwin the
// default policy hands a contended mutex over in FIFO order through the
// kernel, which cost a spawn heavy program four times its user time in
// system time; first fit lets a running thread take it again
// (gotchas/darwin-mutex-fairshare.md).
static void mu_init(pthread_mutex_t *m) {
#ifdef __APPLE__
	pthread_mutexattr_t a;
	pthread_mutexattr_init(&a);
	pthread_mutexattr_setpolicy_np(&a, PTHREAD_MUTEX_POLICY_FIRSTFIT_NP);
	pthread_mutex_init(m, &a);
	pthread_mutexattr_destroy(&a);
#else
	pthread_mutex_init(m, NULL);
#endif
}

static void park_init(xo_park *p) {
	mu_init(&p->mu);
	pthread_cond_init(&p->cv, NULL);
	p->signaled = 0;
}

static void park_destroy(xo_park *p) {
	pthread_mutex_destroy(&p->mu);
	pthread_cond_destroy(&p->cv);
}

static void park_reset(xo_park *p) {
	pthread_mutex_lock(&p->mu);
	p->signaled = 0;
	pthread_mutex_unlock(&p->mu);
}

static void park_signal(xo_park *p) {
	pthread_mutex_lock(&p->mu);
	p->signaled = 1;
	if (p->counted) {
		p->counted = 0;
		spin_lock(&dl.mu);
		dl.blocked--;
		spin_unlock(&dl.mu);
	}
	pthread_cond_signal(&p->cv);
	pthread_mutex_unlock(&p->mu);
}

typedef struct xo_node {
	struct xo_node *parent, *child, *prev, *next;
	int canceled;    // atomic reads; written under tree_mu
	xo_park *waiter; // the park of a context blocked at this node (tree_mu)
} xo_node;

static xo_spin tree_mu;

static int node_canceled(xo_node *n) { return __atomic_load_n(&n->canceled, __ATOMIC_ACQUIRE); }

// node_link adds n below parent (canceled at once when parent is).
static void node_link(xo_node *n, xo_node *parent) {
	spin_lock(&tree_mu);
	n->parent = parent;
	n->prev = NULL;
	n->next = parent->child;
	if (parent->child) {
		parent->child->prev = n;
	}
	parent->child = n;
	if (parent->canceled) {
		__atomic_store_n(&n->canceled, 1, __ATOMIC_RELEASE);
	}
	spin_unlock(&tree_mu);
}

static void node_unlink_locked(xo_node *n) {
	xo_node *p = n->parent;
	if (!p) {
		return;
	}
	if (n->prev) {
		n->prev->next = n->next;
	} else {
		p->child = n->next;
	}
	if (n->next) {
		n->next->prev = n->prev;
	}
	n->parent = n->prev = n->next = NULL;
	// children still linked (they end first: a scope's nodes go before
	// the body that holds them) keep a stale parent; nothing walks up
}

static void node_unlink(xo_node *n) {
	spin_lock(&tree_mu);
	node_unlink_locked(n);
	spin_unlock(&tree_mu);
}

static void cancel_locked(xo_node *n) {
	if (!n->canceled) {
		__atomic_store_n(&n->canceled, 1, __ATOMIC_RELEASE);
	}
	// a canceled node's subtree may have gained children since: walk it
	if (n->waiter) {
		park_signal(n->waiter);
	}
	for (xo_node *c = n->child; c; c = c->next) {
		cancel_locked(c);
	}
}

// node_cancel cancels n and its subtree (core 7.2) and wakes every
// context blocked in it.
static void node_cancel(xo_node *n) {
	spin_lock(&tree_mu);
	cancel_locked(n);
	spin_unlock(&tree_mu);
}

typedef struct {
	void *m;
	const char *loc;
	int64_t n;
} xo_held;

struct xo_task;

// xo_ctx is a running context: the main program, or a task.
typedef struct xo_ctx {
	xo_node *node; // innermost: a scope body, or the task's node
	int unwinding;
	xo_park park;
	// the mutexes this context holds (re-entrant use faults, XO-F004)
	xo_held *held;
	int nheld, capheld;
} xo_ctx;

static __thread xo_ctx *cur;

static pthread_once_t rt_once = PTHREAD_ONCE_INIT;

// rt_init gives the runtime's global mutexes their policy (mu_init)
// before the first context uses them.
static void rt_init(void);

// ctx returns the current context, making a root for a thread that runs
// Xo code outside any task (the program's main thread).
static xo_ctx *ctx(void) {
	xo_ctx *c = cur;
	if (__builtin_expect(c != NULL, 1)) {
		return c;
	}
	pthread_once(&rt_once, rt_init);
	c = calloc(1, sizeof(xo_ctx));
	xo_node *root = calloc(1, sizeof(xo_node));
	if (!c || !root) {
		xo_oom();
	}
	c->node = root;
	park_init(&c->park);
	cur = c;
	spin_lock(&dl.mu);
	dl.live++;
	spin_unlock(&dl.mu);
	return c;
}

int64_t xo_unwinding(void) {
	xo_ctx *c = cur;
	return c ? c->unwinding : 0;
}

int64_t xo_is_canceled(void) { return node_canceled(ctx()->node); }

// canceled reports whether a cancellation point in c reports
// cancellation (and marks c unwinding when it does).
static int canceled(xo_ctx *c) {
	if (node_canceled(c->node)) {
		c->unwinding = 1;
		return 1;
	}
	return 0;
}

static void held_check(xo_ctx *c, void *m, const char *what, const char *loc, int64_t n) {
	for (int i = 0; i < c->nheld; i++) {
		if (c->held[i].m == m) {
			char *prev = cstr_of(c->held[i].loc, c->held[i].n);
			char *msg = malloc(strlen(prev) + 128);
			sprintf(msg, "XO-F004: Mutex.%s called re-entrantly in one task; the mutex is already held from %s", what, prev);
			fault_at(msg, loc, n);
		}
	}
	if (c->nheld == c->capheld) {
		c->capheld = c->capheld ? c->capheld * 2 : 4;
		c->held = realloc(c->held, (size_t)c->capheld * sizeof(xo_held));
		if (!c->held) {
			xo_oom();
		}
	}
	c->held[c->nheld++] = (xo_held){m, loc, n};
}

static void held_del(xo_ctx *c, void *m) {
	for (int i = 0; i < c->nheld; i++) {
		if (c->held[i].m == m) {
			c->held[i] = c->held[--c->nheld];
			return;
		}
	}
}

// ---------------------------------------------------------------------------
// The worker pool

typedef struct {
	xo_spin mu;
	struct xo_task **buf;
	// a ring: head is the top, head+len-1 the bottom; len is read
	// without the lock to skip empty deques
	int64_t head, len, cap;
} xo_deque;

static void dq_push(xo_deque *d, struct xo_task *t) {
	spin_lock(&d->mu);
	if (d->len == d->cap) {
		int64_t nc = d->cap ? d->cap * 2 : 64;
		struct xo_task **nb = malloc((size_t)nc * sizeof(*nb));
		if (!nb) {
			xo_oom();
		}
		for (int64_t i = 0; i < d->len; i++) {
			nb[i] = d->buf[(d->head + i) % d->cap];
		}
		free(d->buf);
		d->buf = nb;
		d->head = 0;
		d->cap = nc;
	}
	d->buf[(d->head + d->len) % d->cap] = t;
	__atomic_store_n(&d->len, d->len + 1, __ATOMIC_RELEASE);
	spin_unlock(&d->mu);
}

// dq_take pops from the bottom (the owner, bottom set) or the top.
static struct xo_task *dq_take(xo_deque *d, int bottom) {
	if (__atomic_load_n(&d->len, __ATOMIC_ACQUIRE) == 0) {
		return NULL;
	}
	spin_lock(&d->mu);
	struct xo_task *t = NULL;
	if (d->len > 0) {
		if (bottom) {
			t = d->buf[(d->head + d->len - 1) % d->cap];
		} else {
			t = d->buf[d->head];
			d->head = (d->head + 1) % d->cap;
		}
		__atomic_store_n(&d->len, d->len - 1, __ATOMIC_RELEASE);
	}
	spin_unlock(&d->mu);
	return t;
}

#define XO_MAXW 4096
#define LOAD(x) __atomic_load_n(&(x), __ATOMIC_SEQ_CST)
#define STORE(x, v) __atomic_store_n(&(x), (v), __ATOMIC_SEQ_CST)

typedef struct {
	xo_deque dq;
	int live;
} xo_worker;

static struct {
	pthread_once_t once;
	pthread_mutex_t mu;
	pthread_cond_t cv;
	int ncpu;
	xo_worker *ws[XO_MAXW];
	// written under mu, read without it too (atomic)
	int nw;          // worker slots used
	int alive;       // live workers
	int idle;        // workers waiting for work
	int spinning;    // workers looking for work before they sleep
	int blocked;     // workers blocked in a task
	int64_t nqueued; // tasks in the queues
	xo_deque inject;
} pool = {.once = PTHREAD_ONCE_INIT, .mu = PTHREAD_MUTEX_INITIALIZER, .cv = PTHREAD_COND_INITIALIZER};

static __thread xo_worker *tls_w;

static void rt_init(void) {
	mu_init(&pool.mu);
}

static void pool_init(void) {
	xo_rt_threads_begin();
	long n = sysconf(_SC_NPROCESSORS_ONLN);
	const char *e = getenv("XO_THREADS");
	if (e && atoi(e) > 0) {
		n = atoi(e);
	}
	if (n < 1) {
		n = 1;
	}
	if (n > 256) {
		n = 256;
	}
	pool.ncpu = (int)n;
}

static void *worker_main(void *arg);

// start_worker starts a worker thread (pool.mu held).
static void start_worker(void) {
	if (pool.alive >= XO_MAXW - 1) {
		return;
	}
	xo_worker *w = NULL;
	for (int i = 0; i < pool.nw; i++) {
		if (!pool.ws[i]->live) {
			w = pool.ws[i];
			break;
		}
	}
	if (!w) {
		w = calloc(1, sizeof(xo_worker));
		if (!w) {
			xo_oom();
		}
		pool.ws[pool.nw] = w;
		STORE(pool.nw, pool.nw + 1);
	}
	w->live = 1;
	STORE(pool.alive, pool.alive + 1);
	pthread_attr_t attr;
	pthread_attr_init(&attr);
	pthread_attr_setstacksize(&attr, (size_t)64 << 20);
	pthread_attr_setdetachstate(&attr, PTHREAD_CREATE_DETACHED);
	pthread_t th;
	if (pthread_create(&th, &attr, worker_main, w) != 0) {
		w->live = 0;
		STORE(pool.alive, pool.alive - 1);
	}
	pthread_attr_destroy(&attr);
}

// wake makes sure queued work has a worker: an idle one is signaled, or
// a new one starts while fewer than one per CPU can run.
static void wake(void) {
	pthread_mutex_lock(&pool.mu);
	if (pool.idle > 0) {
		pthread_cond_signal(&pool.cv);
	} else if (pool.alive - pool.blocked < pool.ncpu) {
		start_worker();
	}
	pthread_mutex_unlock(&pool.mu);
}

static void pool_push(struct xo_task *t) {
	if (tls_w) {
		dq_push(&tls_w->dq, t);
	} else {
		dq_push(&pool.inject, t);
	}
	__atomic_fetch_add(&pool.nqueued, 1, __ATOMIC_SEQ_CST);
	// a spinning worker will take it (it checks nqueued after it stops
	// spinning); a worker that goes idle checks nqueued after it counts
	// itself idle, so it sees this task or is seen here
	if (LOAD(pool.spinning) > 0) {
		return;
	}
	if (LOAD(pool.idle) > 0 || LOAD(pool.alive) - LOAD(pool.blocked) < pool.ncpu) {
		wake();
	}
}

static struct xo_task *pool_pop(xo_worker *w);

static inline void cpu_relax(void) { spin_relax(); }

// spin looks for work for a while before the worker sleeps (at most two
// workers at once): a task spawned meanwhile costs no wake up.
static struct xo_task *spin(xo_worker *w) {
	if (LOAD(pool.spinning) >= 2) {
		return NULL;
	}
	__atomic_fetch_add(&pool.spinning, 1, __ATOMIC_SEQ_CST);
	struct xo_task *t = NULL;
	for (int i = 0; i < 4000 && !t; i++) {
		if (LOAD(pool.nqueued) > 0) {
			t = pool_pop(w);
		} else {
			cpu_relax();
		}
	}
	__atomic_fetch_sub(&pool.spinning, 1, __ATOMIC_SEQ_CST);
	if (t && LOAD(pool.nqueued) > 0 && LOAD(pool.spinning) == 0) {
		wake(); // more work: another worker looks for it
	}
	return t;
}

static struct xo_task *pool_pop(xo_worker *w) {
	struct xo_task *t = NULL;
	if (w) {
		t = dq_take(&w->dq, 1);
	}
	if (!t) {
		t = dq_take(&pool.inject, 0);
	}
	if (!t) {
		int n = LOAD(pool.nw);
		for (int i = 0; i < n && !t; i++) {
			xo_worker *v = pool.ws[i];
			if (v != w) {
				t = dq_take(&v->dq, 0);
			}
		}
	}
	if (t) {
		// live before it leaves the queue, so it is never missing from both
		spin_lock(&dl.mu);
		dl.live++;
		__atomic_fetch_sub(&pool.nqueued, 1, __ATOMIC_SEQ_CST);
		spin_unlock(&dl.mu);
	}
	return t;
}

XO_NORETURN static void deadlock(void) {
	xo_flush();
	static const char m[] = "fault: XO-F001: deadlock: every task is blocked\n";
	write_all(2, m, sizeof m - 1);
	exit(70);
}

static void dl_check_locked(void) {
	if (dl.live > 0 && dl.blocked == dl.live && LOAD(pool.nqueued) == 0) {
		deadlock();
	}
}

// block_begin and block_end bracket a wait of a worker's task: while it
// is blocked, queued work gets another worker.
static void block_begin(void) {
	if (!tls_w) {
		return;
	}
	pthread_mutex_lock(&pool.mu);
	STORE(pool.blocked, pool.blocked + 1);
	if (LOAD(pool.nqueued) > 0 && pool.idle == 0 && pool.alive - pool.blocked < pool.ncpu) {
		start_worker();
	}
	pthread_mutex_unlock(&pool.mu);
}

static void block_end(void) {
	if (!tls_w) {
		return;
	}
	pthread_mutex_lock(&pool.mu);
	STORE(pool.blocked, pool.blocked - 1);
	pthread_mutex_unlock(&pool.mu);
}

// park_wait blocks the current context until its park is signaled or,
// when cancelable, its node is canceled.
static void park_wait(xo_ctx *c, int cancelable) {
	if (cancelable) {
		spin_lock(&tree_mu);
		c->node->waiter = &c->park;
		spin_unlock(&tree_mu);
	}
	block_begin();
	pthread_mutex_lock(&c->park.mu);
	if (!c->park.signaled && !(cancelable && node_canceled(c->node))) {
		c->park.counted = 1;
		spin_lock(&dl.mu);
		dl.blocked++;
		dl_check_locked();
		spin_unlock(&dl.mu);
		while (!c->park.signaled && !(cancelable && node_canceled(c->node))) {
			pthread_cond_wait(&c->park.cv, &c->park.mu);
		}
		if (c->park.counted) {
			// woken by cancellation
			c->park.counted = 0;
			spin_lock(&dl.mu);
			dl.blocked--;
			spin_unlock(&dl.mu);
		}
	}
	c->park.signaled = 0;
	pthread_mutex_unlock(&c->park.mu);
	block_end();
	if (cancelable) {
		spin_lock(&tree_mu);
		c->node->waiter = NULL;
		spin_unlock(&tree_mu);
	}
}

// A wait list: the parks of the contexts waiting on an object, signaled
// all at once when its state changes (they check again).
typedef struct xo_went {
	xo_park *park;
	struct xo_went *prev, *next;
} xo_went;

typedef struct {
	xo_went *first;
} xo_wl;

static void wl_add(xo_wl *l, xo_went *e) {
	e->prev = NULL;
	e->next = l->first;
	if (l->first) {
		l->first->prev = e;
	}
	l->first = e;
}

static void wl_del(xo_wl *l, xo_went *e) {
	if (e->prev) {
		e->prev->next = e->next;
	} else {
		l->first = e->next;
	}
	if (e->next) {
		e->next->prev = e->prev;
	}
}

static void wl_wake(xo_wl *l) {
	for (xo_went *e = l->first; e; e = e->next) {
		park_signal(e->park);
	}
}

// wl_wait waits on list l of an object whose mutex mu is held; mu is
// held again on return. It reports 0 when the context was canceled.
static int wl_wait(xo_ctx *c, pthread_mutex_t *mu, xo_wl *l, int cancelable) {
	xo_went e = {.park = &c->park};
	park_reset(&c->park);
	wl_add(l, &e);
	pthread_mutex_unlock(mu);
	park_wait(c, cancelable);
	pthread_mutex_lock(mu);
	wl_del(l, &e);
	return !(cancelable && node_canceled(c->node));
}

// ---------------------------------------------------------------------------
// Scopes and tasks

typedef struct xo_scope {
	xo_hdr h;
	xo_node body;
	xo_node *tasks;       // the node of the scope's tasks (replaced by s.cancel())
	xo_node **old;        // earlier task nodes, freed with the scope
	int nold;
	xo_node *parent_node; // the opening context's node before the scope
	pthread_mutex_t mu;
	int64_t running; // tasks not finished
	int64_t queued;  // queue entries of its tasks not taken yet
	xo_wl idle;
	struct xo_task *list; // every task, each holding a reference, until the scope closes
	struct xo_task *fail; // the first task that failed (a reference)
	int closed;
} xo_scope;

typedef void (*xo_run_fn)(xo_rlist *, struct xo_task *);

typedef struct xo_task {
	xo_hdr h;
	xo_node node;
	// the scope (not counted: the scope's join waits for its tasks, and
	// only a running task or its worker uses it)
	xo_scope *scope;
	int64_t state; // atomic: 0 queued, 1 running, 2 done
	xo_run_fn run;
	xo_ldrop drop;
	xo_rlist *fl;     // the closure (a list of one) until the task runs
	xo_rlist *result; // the result (a shared list of one) when it ran to the end
	int unwound;      // the task ended by unwinding (canceled)
	int background, consumed;
	int64_t site;
	pthread_mutex_t mu;
	xo_wl waiters;
	struct xo_task *snext;
} xo_task;

enum { T_QUEUED, T_RUNNING, T_DONE };

static void scope_free(void *p) {
	xo_scope *s = p;
	if (s->fail) {
		h_release(s->fail);
	}
	free(s->tasks);
	for (int i = 0; i < s->nold; i++) {
		free(s->old[i]);
	}
	free(s->old);
	pthread_mutex_destroy(&s->mu);
	xo_free(s);
}

xo_scope *xo_scope_open(void) {
	xo_ctx *c = ctx();
	xo_scope *s = xo_alloc(sizeof(xo_scope));
	memset(s, 0, sizeof *s);
	h_init(s, scope_free);
	mu_init(&s->mu);
	s->parent_node = c->node;
	node_link(&s->body, c->node);
	s->tasks = calloc(1, sizeof(xo_node));
	if (!s->tasks) {
		xo_oom();
	}
	node_link(s->tasks, &s->body);
	c->node = &s->body;
	return s;
}

// xo_scope_group is `s.group()`: the same scope behind a Group handle.
xo_scope *xo_scope_group(xo_scope *s) {
	h_retain(s);
	return s;
}

// xo_scope_cancel is `s.cancel()`: cancels the scope's tasks, not the
// body; tasks spawned later start under a fresh node.
void xo_scope_cancel(xo_scope *s) {
	xo_node *fresh = calloc(1, sizeof(xo_node));
	if (!fresh) {
		xo_oom();
	}
	pthread_mutex_lock(&s->mu);
	xo_node *old = s->tasks;
	s->old = realloc(s->old, (size_t)(s->nold + 1) * sizeof(xo_node *));
	if (!s->old) {
		xo_oom();
	}
	s->old[s->nold++] = old;
	node_link(fresh, &s->body);
	s->tasks = fresh;
	pthread_mutex_unlock(&s->mu);
	node_cancel(old);
}

static void task_free(void *p) {
	xo_task *t = p;
	if (t->result) {
		list_release(t->result, t->drop);
	}
	pthread_mutex_destroy(&t->mu);
	xo_free(t);
}

// run_task runs t on this thread if nobody has started it.
static void run_task(xo_task *t) {
	int64_t q = T_QUEUED;
	if (!__atomic_compare_exchange_n(&t->state, &q, T_RUNNING, 0, __ATOMIC_ACQ_REL, __ATOMIC_ACQUIRE)) {
		return;
	}
	xo_ctx c = {.node = &t->node};
	park_init(&c.park);
	xo_ctx *saved = cur;
	cur = &c;
	xo_rlist *fl = t->fl;
	t->fl = NULL;
	h_retain(t); // the entry function's own reference
	t->run(fl, t);
	cur = saved;
	park_destroy(&c.park);
	free(c.held);
	xo_scope *s = t->scope;
	node_unlink(&t->node);
	pthread_mutex_lock(&t->mu);
	__atomic_store_n(&t->state, T_DONE, __ATOMIC_RELEASE);
	wl_wake(&t->waiters);
	pthread_mutex_unlock(&t->mu);
	// the last touch of t and s: the scope's join may free them after
	pthread_mutex_lock(&s->mu);
	if (--s->running == 0 && s->queued == 0) {
		wl_wake(&s->idle);
	}
	pthread_mutex_unlock(&s->mu);
}

// dequeued records that a worker took t's queue entry (and ran it, or
// found it run by a helper). The scope's join waits for this too, so no
// queue holds a task the scope frees.
static void dequeued(xo_task *t) {
	xo_scope *s = t->scope;
	pthread_mutex_lock(&s->mu);
	if (--s->queued == 0 && s->running == 0) {
		wl_wake(&s->idle);
	}
	pthread_mutex_unlock(&s->mu);
}

static void *worker_main(void *arg) {
	xo_worker *w = arg;
	tls_w = w;
	for (;;) {
		xo_task *t = pool_pop(w);
		if (!t) {
			t = spin(w);
		}
		if (t) {
			run_task(t);
			dequeued(t);
			spin_lock(&dl.mu);
			dl.live--;
			dl_check_locked();
			spin_unlock(&dl.mu);
			continue;
		}
		pthread_mutex_lock(&pool.mu);
		if (LOAD(pool.nqueued) > 0) {
			pthread_mutex_unlock(&pool.mu);
			continue;
		}
		if (pool.alive - pool.blocked > pool.ncpu) {
			// more workers than CPUs since others came back from waiting
			w->live = 0;
			STORE(pool.alive, pool.alive - 1);
			pthread_mutex_unlock(&pool.mu);
			break;
		}
		STORE(pool.idle, pool.idle + 1);
		while (LOAD(pool.nqueued) == 0) {
			pthread_cond_wait(&pool.cv, &pool.mu);
		}
		STORE(pool.idle, pool.idle - 1);
		pthread_mutex_unlock(&pool.mu);
	}
	heap_release();
	return NULL;
}

// xo_task_spawn starts a task in scope s (`s.spawn(f)`, flags bit 0:
// background, bit 1: its handle is bound and awaited, so its error does
// not fail the scope). fl is the closure in a list of one (consumed);
// run is the entry function that calls it, drop the one that releases
// its result list. site tells the scope which spawn failed.
xo_task *xo_task_spawn(xo_scope *s, xo_rlist *fl, int64_t run, int64_t drop, int64_t flags, int64_t site, const char *loc, int64_t n) {
	pthread_once(&pool.once, pool_init);
	xo_task *t = xo_alloc(sizeof(xo_task));
	memset(t, 0, sizeof *t);
	h_init(t, task_free);
	// references: the handle returned, and the scope's list (which keeps
	// t alive while a queue holds it)
	t->h.rc = -2;
	mu_init(&t->mu);
	t->run = (xo_run_fn)run;
	t->drop = (xo_ldrop)drop;
	t->fl = fl;
	t->background = (int)(flags & 1);
	t->consumed = (int)((flags >> 1) & 1);
	t->site = site;
	pthread_mutex_lock(&s->mu);
	if (s->closed) {
		pthread_mutex_unlock(&s->mu);
		fault_at("a task was started on a scope that has ended", loc, n);
	}
	t->scope = s;
	node_link(&t->node, s->tasks);
	t->snext = s->list;
	s->list = t;
	s->running++;
	s->queued++;
	pthread_mutex_unlock(&s->mu);
	pool_push(t);
	return t;
}

// xo_task_done records the result of the current task (a shared list of
// one, consumed); failed tells that it is an error, which fails the
// scope unless the handle is awaited or the task was canceled.
void xo_task_done(xo_task *t, xo_rlist *rl, int64_t failed) {
	t->result = rl;
	if (!failed || t->consumed || node_canceled(&t->node)) {
		return;
	}
	xo_scope *s = t->scope;
	pthread_mutex_lock(&s->mu);
	int first = s->fail == NULL;
	if (first) {
		h_retain(t);
		s->fail = t;
	}
	pthread_mutex_unlock(&s->mu);
	if (first) {
		// the body and every task (core 7.1, child failure)
		node_cancel(&s->body);
	}
}

// xo_task_unwound ends the current task that unwound (canceled).
void xo_task_unwound(xo_task *t) {
	cur->unwinding = 0;
	t->unwound = 1;
}

// xo_task_await is `t.await` (core 7.1): waits for t (a cancellation
// point that unwinds) and returns its result list (a reference).
xo_rlist *xo_task_await(xo_task *t, const char *loc, int64_t n) {
	xo_ctx *c = ctx();
	if (__atomic_load_n(&t->state, __ATOMIC_ACQUIRE) != T_DONE) {
		if (canceled(c)) {
			return &empty_list;
		}
		run_task(t); // not started yet: run it here
		pthread_mutex_lock(&t->mu);
		while (__atomic_load_n(&t->state, __ATOMIC_ACQUIRE) != T_DONE) {
			if (!wl_wait(c, &t->mu, &t->waiters, 1)) {
				pthread_mutex_unlock(&t->mu);
				c->unwinding = 1;
				return &empty_list;
			}
		}
		pthread_mutex_unlock(&t->mu);
	}
	if (t->unwound) {
		if (canceled(c)) {
			return &empty_list;
		}
		fault_at("awaited a task that was canceled before it finished", loc, n);
	}
	rc_inc(&t->result->rc);
	return t->result;
}

// xo_task_result returns the result list of a finished task (a
// reference): the error of a failed task, for its scope.
xo_rlist *xo_task_result(xo_task *t) {
	rc_inc(&t->result->rc);
	return t->result;
}

xo_task *xo_scope_fail_task(xo_scope *s) {
	h_retain(s->fail);
	return s->fail;
}

int64_t xo_scope_fail_site(xo_scope *s) { return s->fail ? s->fail->site : -1; }

// xo_scope_close ends scope s (core 7.1 exit rules). mode 0: the body
// completed (background tasks are canceled); 1: it exits early (return,
// break, an error: every task is canceled); 2: it unwinds (canceled).
// It runs the tasks nobody started, waits for every task, and returns 1
// when a child failed and its error must propagate out of the scope (an
// unwinding body then stops unwinding), else 0.
int64_t xo_scope_close(xo_scope *s, int64_t mode) {
	xo_ctx *c = ctx();
	pthread_mutex_lock(&s->mu);
	if (mode == 0) {
		for (xo_task *t = s->list; t; t = t->snext) {
			if (t->background) {
				node_cancel(&t->node);
			}
		}
	} else {
		node_cancel(s->tasks);
	}
	pthread_mutex_unlock(&s->mu);
	// help: run the tasks that have not started on this thread (tasks
	// spawned meanwhile go to the head of the list: look again until a
	// pass finds none; a task's link is fixed once it is in the list)
	for (int found = 1; found;) {
		found = 0;
		pthread_mutex_lock(&s->mu);
		xo_task *t = s->list;
		pthread_mutex_unlock(&s->mu);
		for (; t; t = t->snext) {
			if (__atomic_load_n(&t->state, __ATOMIC_ACQUIRE) == T_QUEUED) {
				run_task(t);
				found = 1;
			}
		}
	}
	// wait (not a cancellation point: a canceled body still waits)
	pthread_mutex_lock(&s->mu);
	while (s->running > 0 || s->queued > 0) {
		wl_wait(c, &s->mu, &s->idle, 0);
	}
	s->closed = 1;
	xo_task *list = s->list;
	s->list = NULL;
	pthread_mutex_unlock(&s->mu);
	while (list) {
		xo_task *next = list->snext;
		h_release(list);
		list = next;
	}
	spin_lock(&tree_mu);
	node_unlink_locked(s->tasks);
	for (int i = 0; i < s->nold; i++) {
		node_unlink_locked(s->old[i]);
	}
	node_unlink_locked(&s->body);
	spin_unlock(&tree_mu);
	c->node = s->parent_node;
	if (!s->fail) {
		return 0;
	}
	if (mode == 2) {
		if (node_canceled(s->parent_node)) {
			return 0; // canceled from outside: keep unwinding
		}
		c->unwinding = 0;
	}
	return 1;
}

// ---------------------------------------------------------------------------
// Channels (core 7.3)

typedef struct xo_cent {
	xo_park *park;
	int64_t *fired; // claimed by the first waker (atomic)
	xo_rlist *item; // a send's value, or what a receive got (NULL: closed)
	int shut;       // the channel closed while a send waited
	// a select's arm: the waker stores idx in *chosen
	int64_t idx, *chosen;
	struct xo_cent *prev, *next;
} xo_cent;

typedef struct {
	xo_cent *head, *tail;
} xo_cq;

static void cq_push(xo_cq *q, xo_cent *e) {
	e->next = NULL;
	e->prev = q->tail;
	if (q->tail) {
		q->tail->next = e;
	} else {
		q->head = e;
	}
	q->tail = e;
}

static void cq_del(xo_cq *q, xo_cent *e) {
	if (e->prev) {
		e->prev->next = e->next;
	} else if (q->head == e) {
		q->head = e->next;
	} else {
		return; // not queued
	}
	if (e->next) {
		e->next->prev = e->prev;
	} else {
		q->tail = e->prev;
	}
	e->prev = e->next = NULL;
}

static int claim(int64_t *fired) {
	int64_t z = 0;
	return __atomic_compare_exchange_n(fired, &z, 1, 0, __ATOMIC_ACQ_REL, __ATOMIC_ACQUIRE);
}

// cq_take pops the first waiter nobody has claimed, claiming it.
static xo_cent *cq_take(xo_cq *q) {
	while (q->head) {
		xo_cent *e = q->head;
		cq_del(q, e);
		if (claim(e->fired)) {
			return e;
		}
	}
	return NULL;
}

typedef struct {
	xo_hdr h;
	pthread_mutex_t mu;
	int64_t cap, head, len;
	xo_rlist **buf;
	int closed;
	xo_ldrop drop;
	xo_cq recvq, sendq;
} xo_chan;

static void chan_free(void *p) {
	xo_chan *c = p;
	for (int64_t i = 0; i < c->len; i++) {
		list_release(c->buf[(c->head + i) % c->cap], c->drop);
	}
	free(c->buf);
	pthread_mutex_destroy(&c->mu);
	xo_free(c);
}

xo_chan *xo_chan_new(int64_t cap, int64_t drop, const char *loc, int64_t n) {
	if (cap < 0) {
		char m[96];
		snprintf(m, sizeof m, "negative channel capacity %lld", (long long)cap);
		fault_at(m, loc, n);
	}
	xo_chan *c = xo_alloc(sizeof(xo_chan));
	memset(c, 0, sizeof *c);
	h_init(c, chan_free);
	mu_init(&c->mu);
	c->cap = cap;
	c->drop = (xo_ldrop)drop;
	if (cap > 0) {
		c->buf = malloc((size_t)cap * sizeof(xo_rlist *));
		if (!c->buf) {
			xo_oom();
		}
	}
	return c;
}

// hand wakes the waiter of e, which a waker claimed and gave its value
// (c->mu held).
static void hand(xo_cent *e) {
	if (e->chosen) {
		__atomic_store_n(e->chosen, e->idx, __ATOMIC_RELEASE);
	}
	park_signal(e->park);
}

// try_send_locked sends l without blocking (c->mu held, c open).
static int try_send_locked(xo_chan *c, xo_rlist *l) {
	xo_cent *r = cq_take(&c->recvq);
	if (r) {
		r->item = l;
		hand(r);
		return 1;
	}
	if (c->len < c->cap) {
		c->buf[(c->head + c->len) % c->cap] = l;
		c->len++;
		return 1;
	}
	return 0;
}

// try_recv_locked receives without blocking (c->mu held): 1 with *out
// set (NULL: closed and drained), or 0 when it would block.
static int try_recv_locked(xo_chan *c, xo_rlist **out) {
	if (c->len > 0) {
		*out = c->buf[c->head];
		c->head = (c->head + 1) % c->cap;
		c->len--;
		xo_cent *s = cq_take(&c->sendq);
		if (s) {
			c->buf[(c->head + c->len) % c->cap] = s->item;
			c->len++;
			hand(s);
		}
		return 1;
	}
	xo_cent *s = cq_take(&c->sendq);
	if (s) {
		*out = s->item;
		hand(s);
		return 1;
	}
	if (c->closed) {
		*out = NULL;
		return 1;
	}
	return 0;
}

// xo_chan_send is `ch.send(x)` with x in a list of one (consumed): blocks
// while the channel is full, a cancellation point that unwinds; sending
// on a closed channel faults.
void xo_chan_send(xo_chan *c, xo_rlist *l, const char *loc, int64_t n) {
	xo_ctx *x = ctx();
	if (canceled(x)) {
		list_release(l, c->drop);
		return;
	}
	pthread_mutex_lock(&c->mu);
	if (c->closed) {
		pthread_mutex_unlock(&c->mu);
		fault_at("send on a closed channel", loc, n);
	}
	if (try_send_locked(c, l)) {
		pthread_mutex_unlock(&c->mu);
		return;
	}
	int64_t fired = 0;
	xo_cent e = {.park = &x->park, .fired = &fired, .item = l};
	park_reset(&x->park);
	cq_push(&c->sendq, &e);
	pthread_mutex_unlock(&c->mu);
	for (;;) {
		park_wait(x, 1);
		if (__atomic_load_n(&fired, __ATOMIC_ACQUIRE)) {
			break;
		}
		if (node_canceled(x->node)) {
			pthread_mutex_lock(&c->mu);
			if (claim(&fired)) {
				cq_del(&c->sendq, &e);
				pthread_mutex_unlock(&c->mu);
				list_release(l, c->drop);
				x->unwinding = 1;
				return;
			}
			pthread_mutex_unlock(&c->mu);
			break; // taken meanwhile: sent
		}
	}
	pthread_mutex_lock(&c->mu); // the waker's writes
	int shut = e.shut;
	pthread_mutex_unlock(&c->mu);
	if (shut) {
		list_release(l, c->drop);
		fault_at("send on a closed channel", loc, n);
	}
}

// xo_chan_recv is `ch.recv()`: the value in a list of one, or the empty
// list when the channel is closed and drained (and while unwinding); a
// cancellation point that unwinds.
xo_rlist *xo_chan_recv(xo_chan *c) {
	xo_ctx *x = ctx();
	if (canceled(x)) {
		return &empty_list;
	}
	xo_rlist *out;
	pthread_mutex_lock(&c->mu);
	if (try_recv_locked(c, &out)) {
		pthread_mutex_unlock(&c->mu);
		return out ? out : &empty_list;
	}
	int64_t fired = 0;
	xo_cent e = {.park = &x->park, .fired = &fired};
	park_reset(&x->park);
	cq_push(&c->recvq, &e);
	pthread_mutex_unlock(&c->mu);
	for (;;) {
		park_wait(x, 1);
		if (__atomic_load_n(&fired, __ATOMIC_ACQUIRE)) {
			break;
		}
		if (node_canceled(x->node)) {
			pthread_mutex_lock(&c->mu);
			if (claim(&fired)) {
				cq_del(&c->recvq, &e);
				pthread_mutex_unlock(&c->mu);
				x->unwinding = 1;
				return &empty_list;
			}
			pthread_mutex_unlock(&c->mu);
			break; // a value came meanwhile: it wins
		}
	}
	pthread_mutex_lock(&c->mu);
	out = e.item;
	pthread_mutex_unlock(&c->mu);
	return out ? out : &empty_list;
}

// xo_chan_try_send is `ch.try_send(x)`: false (and x released) when the
// channel is full or closed.
int64_t xo_chan_try_send(xo_chan *c, xo_rlist *l) {
	pthread_mutex_lock(&c->mu);
	int ok = !c->closed && try_send_locked(c, l);
	pthread_mutex_unlock(&c->mu);
	if (!ok) {
		list_release(l, c->drop);
	}
	return ok;
}

// xo_chan_try_recv is `ch.try_recv()`: never blocks.
xo_rlist *xo_chan_try_recv(xo_chan *c) {
	xo_rlist *out = NULL;
	pthread_mutex_lock(&c->mu);
	if (!try_recv_locked(c, &out)) {
		out = NULL;
	}
	pthread_mutex_unlock(&c->mu);
	return out ? out : &empty_list;
}

// xo_chan_close is `ch.close()`: waiting receivers get None, waiting
// senders fault. Closing twice has no effect.
void xo_chan_close(xo_chan *c) {
	pthread_mutex_lock(&c->mu);
	if (!c->closed) {
		c->closed = 1;
		xo_cent *e;
		while ((e = cq_take(&c->recvq))) {
			e->item = NULL;
			hand(e);
		}
		while ((e = cq_take(&c->sendq))) {
			e->shut = 1;
			hand(e);
		}
	}
	pthread_mutex_unlock(&c->mu);
}

int64_t xo_chan_len(xo_chan *c) {
	pthread_mutex_lock(&c->mu);
	int64_t n = c->len;
	pthread_mutex_unlock(&c->mu);
	return n;
}

// select (core 7.3): arms are added in source order, then
// xo_select_wait performs the first arm that can proceed, or waits for
// one (a cancellation point that unwinds). While it decides, and while
// it takes its arms off the queues, it holds the locks of all its
// channels (taken in address order), so no waker sees a half registered
// select.

typedef struct {
	xo_chan *ch; // a reference
	int send;
	xo_rlist *item; // a send's value until sent; what a receive got
	xo_cent e;
} xo_sarm;

typedef struct {
	xo_hdr h;
	xo_sarm *arms;
	int n, cap;
	int64_t fired, chosen;
} xo_select;

static void select_free(void *p) {
	xo_select *s = p;
	for (int i = 0; i < s->n; i++) {
		list_release(s->arms[i].item, s->arms[i].ch->drop);
		h_release(s->arms[i].ch);
	}
	free(s->arms);
	xo_free(s);
}

xo_select *xo_select_new(void) {
	xo_select *s = xo_alloc(sizeof(xo_select));
	memset(s, 0, sizeof *s);
	h_init(s, select_free);
	return s;
}

static void select_arm(xo_select *s, xo_chan *c, int send, xo_rlist *l) {
	if (s->n == s->cap) {
		s->cap = s->cap ? s->cap * 2 : 4;
		s->arms = realloc(s->arms, (size_t)s->cap * sizeof(xo_sarm));
		if (!s->arms) {
			xo_oom();
		}
	}
	h_retain(c);
	s->arms[s->n++] = (xo_sarm){.ch = c, .send = send, .item = l};
}

// xo_select_recv adds an arm `v = ch.recv()`.
void xo_select_recv(xo_select *s, xo_chan *c) { select_arm(s, c, 0, NULL); }

// xo_select_send adds an arm `ch.send(x)`, x in a list of one (consumed).
void xo_select_send(xo_select *s, xo_chan *c, xo_rlist *l) { select_arm(s, c, 1, l); }

static int chan_addr_cmp(const void *a, const void *b) {
	uintptr_t x = (uintptr_t)*(xo_chan *const *)a, y = (uintptr_t)*(xo_chan *const *)b;
	return x < y ? -1 : x > y;
}

// xo_select_wait returns the index of the arm that proceeded (-1 while
// unwinding); a send arm on a closed channel faults.
int64_t xo_select_wait(xo_select *s, const char *loc, int64_t n) {
	xo_ctx *x = ctx();
	if (canceled(x)) {
		return -1;
	}
	xo_chan **cs = malloc((size_t)(s->n ? s->n : 1) * sizeof(xo_chan *));
	if (!cs) {
		xo_oom();
	}
	int nc = 0;
	for (int i = 0; i < s->n; i++) {
		cs[nc++] = s->arms[i].ch;
	}
	qsort(cs, (size_t)nc, sizeof(xo_chan *), chan_addr_cmp);
	int u = 0;
	for (int i = 0; i < nc; i++) {
		if (u == 0 || cs[u - 1] != cs[i]) {
			cs[u++] = cs[i];
		}
	}
	nc = u;
	s->fired = 0;
	s->chosen = -1;
	park_reset(&x->park);
	for (int i = 0; i < nc; i++) {
		pthread_mutex_lock(&cs[i]->mu);
	}
	int done = -1, shut = 0;
	for (int i = 0; i < s->n && done < 0; i++) {
		xo_sarm *a = &s->arms[i];
		xo_chan *c = a->ch;
		if (a->send) {
			if (c->closed) {
				shut = 1;
				done = i;
			} else if (try_send_locked(c, a->item)) {
				a->item = NULL;
				done = i;
			}
		} else {
			xo_rlist *out;
			if (try_recv_locked(c, &out)) {
				a->item = out;
				done = i;
			}
		}
	}
	int queued = done < 0;
	if (queued) {
		for (int i = 0; i < s->n; i++) {
			xo_sarm *a = &s->arms[i];
			a->e = (xo_cent){.park = &x->park, .fired = &s->fired, .item = a->item, .idx = i, .chosen = &s->chosen};
			cq_push(a->send ? &a->ch->sendq : &a->ch->recvq, &a->e);
		}
	}
	for (int i = nc - 1; i >= 0; i--) {
		pthread_mutex_unlock(&cs[i]->mu);
	}
	int unwound = 0;
	if (queued) {
		for (;;) {
			park_wait(x, 1);
			if (__atomic_load_n(&s->fired, __ATOMIC_ACQUIRE)) {
				break;
			}
			if (node_canceled(x->node)) {
				if (claim(&s->fired)) {
					unwound = 1;
				}
				break;
			}
		}
		// take the arms off every queue; the waker's writes are visible
		// under the channel locks
		for (int i = 0; i < nc; i++) {
			pthread_mutex_lock(&cs[i]->mu);
		}
		for (int i = 0; i < s->n; i++) {
			xo_sarm *a = &s->arms[i];
			cq_del(a->send ? &a->ch->sendq : &a->ch->recvq, &a->e);
		}
		if (!unwound) {
			done = (int)__atomic_load_n(&s->chosen, __ATOMIC_ACQUIRE);
			xo_sarm *a = &s->arms[done];
			if (a->send) {
				shut = a->e.shut;
				if (!shut) {
					a->item = NULL; // the receiver has it
				}
			} else {
				a->item = a->e.item;
			}
		}
		for (int i = nc - 1; i >= 0; i--) {
			pthread_mutex_unlock(&cs[i]->mu);
		}
	}
	free(cs);
	if (unwound) {
		x->unwinding = 1;
		return -1;
	}
	if (shut) {
		fault_at("send on a closed channel", loc, n);
	}
	return done;
}

// xo_select_take returns what receive arm i got: the value in a list of
// one, or the empty list when its channel was closed and drained.
xo_rlist *xo_select_take(xo_select *s, int64_t i) {
	xo_rlist *l = s->arms[i].item;
	s->arms[i].item = NULL;
	return l ? l : &empty_list;
}

// ---------------------------------------------------------------------------
// Mutex[T] (core 5.4, std.md 2.7): a readers and writer lock with
// writers preferred, holding its value in a shared list of one.

typedef struct {
	xo_hdr h;
	pthread_mutex_t mu;
	int writer;
	int64_t readers, wwait;
	xo_rlist *slot;
	xo_ldrop drop;
	xo_wl waiters;
} xo_mutex;

static void mutex_free(void *p) {
	xo_mutex *m = p;
	list_release(m->slot, m->drop);
	pthread_mutex_destroy(&m->mu);
	xo_free(m);
}

xo_mutex *xo_mutex_new(xo_rlist *l, int64_t drop) {
	xo_mutex *m = xo_alloc(sizeof(xo_mutex));
	memset(m, 0, sizeof *m);
	h_init(m, mutex_free);
	mu_init(&m->mu);
	m->slot = l;
	m->drop = (xo_ldrop)drop;
	return m;
}

// xo_mutex_lock takes m exclusively for `with` (waiting is a
// cancellation point that unwinds) and moves its value out to the
// caller, who gives it back with xo_mutex_unlock.
xo_rlist *xo_mutex_lock(xo_mutex *m, const char *loc, int64_t n) {
	xo_ctx *c = ctx();
	held_check(c, m, "with", loc, n);
	pthread_mutex_lock(&m->mu);
	if (m->writer || m->readers > 0) {
		m->wwait++;
		while (m->writer || m->readers > 0) {
			if (!wl_wait(c, &m->mu, &m->waiters, 1)) {
				m->wwait--;
				wl_wake(&m->waiters); // readers held back by this writer
				pthread_mutex_unlock(&m->mu);
				held_del(c, m);
				c->unwinding = 1;
				return &empty_list;
			}
		}
		m->wwait--;
	}
	m->writer = 1;
	xo_rlist *l = m->slot;
	m->slot = NULL;
	pthread_mutex_unlock(&m->mu);
	return l;
}

void xo_mutex_unlock(xo_mutex *m, xo_rlist *l) {
	pthread_mutex_lock(&m->mu);
	m->slot = l;
	m->writer = 0;
	wl_wake(&m->waiters);
	pthread_mutex_unlock(&m->mu);
	held_del(ctx(), m);
}

// xo_mutex_rlock takes m shared for `with_read` and returns a reference
// to its value.
xo_rlist *xo_mutex_rlock(xo_mutex *m, const char *loc, int64_t n) {
	xo_ctx *c = ctx();
	held_check(c, m, "with_read", loc, n);
	pthread_mutex_lock(&m->mu);
	while (m->writer || m->wwait > 0) {
		if (!wl_wait(c, &m->mu, &m->waiters, 1)) {
			pthread_mutex_unlock(&m->mu);
			held_del(c, m);
			c->unwinding = 1;
			return &empty_list;
		}
	}
	m->readers++;
	xo_rlist *l = m->slot;
	rc_inc(&l->rc);
	pthread_mutex_unlock(&m->mu);
	return l;
}

// xo_mutex_runlock gives back a reader's reference and its share of m.
void xo_mutex_runlock(xo_mutex *m, xo_rlist *l) {
	if (rc_dec(&l->rc)) {
		list_release(l, m->drop); // not reached: the mutex holds it
	}
	pthread_mutex_lock(&m->mu);
	if (--m->readers == 0) {
		wl_wake(&m->waiters);
	}
	pthread_mutex_unlock(&m->mu);
	held_del(ctx(), m);
}

// ---------------------------------------------------------------------------
// Atomic[T] for Int and Bool (a Bool is 0 or 1)

typedef struct {
	xo_hdr h;
	int64_t v;
} xo_atomic;

static void atomic_free(void *p) { xo_free(p); }

xo_atomic *xo_atomic_new(int64_t v) {
	xo_atomic *a = xo_alloc(sizeof(xo_atomic));
	h_init(a, atomic_free);
	a->v = v;
	return a;
}

int64_t xo_atomic_get(xo_atomic *a) { return __atomic_load_n(&a->v, __ATOMIC_SEQ_CST); }

void xo_atomic_set(xo_atomic *a, int64_t v) { __atomic_store_n(&a->v, v, __ATOMIC_SEQ_CST); }

int64_t xo_atomic_swap(xo_atomic *a, int64_t v) { return __atomic_exchange_n(&a->v, v, __ATOMIC_SEQ_CST); }

int64_t xo_atomic_cas(xo_atomic *a, int64_t old, int64_t v) {
	return __atomic_compare_exchange_n(&a->v, &old, v, 0, __ATOMIC_SEQ_CST, __ATOMIC_SEQ_CST);
}

// xo_atomic_add is `a.add(n)`: the new value; overflow faults.
int64_t xo_atomic_add(xo_atomic *a, int64_t n, const char *loc, int64_t ln) {
	int64_t old = __atomic_load_n(&a->v, __ATOMIC_SEQ_CST);
	for (;;) {
		int64_t nv;
		if (__builtin_add_overflow(old, n, &nv)) {
			char m[160];
			snprintf(m, sizeof m, "integer overflow: %lld + %lld does not fit Int", (long long)old, (long long)n);
			fault_at(m, loc, ln);
		}
		if (__atomic_compare_exchange_n(&a->v, &old, nv, 0, __ATOMIC_SEQ_CST, __ATOMIC_SEQ_CST)) {
			return nv;
		}
	}
}

// xo_atomic_max is `a.max(n)`: stores the larger value and returns it.
int64_t xo_atomic_max(xo_atomic *a, int64_t n) {
	int64_t old = __atomic_load_n(&a->v, __ATOMIC_SEQ_CST);
	while (old < n) {
		if (__atomic_compare_exchange_n(&a->v, &old, n, 0, __ATOMIC_SEQ_CST, __ATOMIC_SEQ_CST)) {
			return n;
		}
	}
	return old;
}

// ---------------------------------------------------------------------------
// Entry (left out with -DXO_RT_NO_MAIN: the C tests of std/llrt/testdata
// include this file and bring their own main)

#ifndef XO_RT_NO_MAIN
extern void xo_entry(void);

static void *run_main(void *arg) {
	(void)arg;
	xo_entry();
	return NULL;
}

int main(int argc, char **argv) {
	(void)argc;
	(void)argv;
	pthread_attr_t attr;
	pthread_attr_init(&attr);
#ifdef XO_RT_TEST_THREADS
	// N copies of the program at once: every thread has its own heap,
	// literals are shared (and never written)
	pthread_attr_setstacksize(&attr, (size_t)1 << 30);
	xo_rt_threads_begin();
	pthread_t ts[XO_RT_TEST_THREADS];
	for (int i = 0; i < XO_RT_TEST_THREADS; i++) {
		if (pthread_create(&ts[i], &attr, run_main, NULL) != 0) {
			fputs("pthread_create failed\n", stderr);
			return 1;
		}
	}
	for (int i = 0; i < XO_RT_TEST_THREADS; i++) {
		pthread_join(ts[i], NULL);
	}
#else
	pthread_attr_setstacksize(&attr, (size_t)1 << 30);
	pthread_t t;
	if (pthread_create(&t, &attr, run_main, NULL) != 0) {
		run_main(NULL);
	} else {
		pthread_join(t, NULL);
	}
#endif
	xo_flush();
	if (getenv("XO_LEAKCHECK")) {
		int64_t live, total;
		xo_rt_live(&live, &total);
		fprintf(stderr, "xo leakcheck: %lld live of %lld allocations\n", (long long)live, (long long)total);
	}
	return 0;
}
#endif
