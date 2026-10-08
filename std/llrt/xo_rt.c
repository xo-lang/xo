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
//   - main: runs the Xo entry on a thread with a 1 GiB stack, like Go's
//     maximum goroutine stack, so deep recursion behaves alike. Built
//     with -DXO_RT_TEST_THREADS=N it runs the entry on N threads at once
//     (a test of the runtime's thread safety, rt_test.go).
//
// The interface to generated code uses only scalars and pointers (no
// structs by value), so it does not depend on the C ABI of aggregates.

#include <math.h>
#include <pthread.h>
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
	struct xo_heap *next; // all heaps, for the leak check
} xo_heap;

static xo_heap main_heap;
static int xo_threaded; // set once, before a second thread runs Xo code
// solo_heap is main_heap while one thread runs, null after (one load
// both tells the case and gives the heap)
static xo_heap *solo_heap = &main_heap;
static __thread xo_heap *tls_heap;
static xo_heap *all_heaps = &main_heap;
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
