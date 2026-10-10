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
#include <time.h>
#include <unistd.h>
#include <errno.h>
#include <fcntl.h>
#include <signal.h>
#include <stdarg.h>
#include <arpa/inet.h>
#include <netdb.h>
#include <netinet/in.h>
#include <netinet/tcp.h>
#include <sys/mman.h>
#include <sys/socket.h>
#include <unwind.h>

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
//
// Central lists (decision 0118): a thread that frees what others
// allocate (a server's connection tasks free what its accept loop made)
// would keep those blocks for good while the allocating thread carves new
// chunks, so memory grew with every connection. With more than one
// thread, free lists move between threads as magazines through a central
// list per class (alloc_mt, free_mt); one thread alone never touches it.

#define XO_CHUNK (4 << 20)
#define XO_SMALL_MAX 1024
#define XO_MID_MAX (256 << 10)
#define XO_NCLASS (64 + 8) // 16..1024 by 16, then 2 KiB..256 KiB

typedef void (*xo_drop_fn)(void *);
typedef struct {
	void *obj;
	xo_drop_fn drop;
} xo_deferred;

// A free deferred by a real-time section (decision 0133, "Real-time
// sections" below): what reached a count of zero and how to free it.
#define XO_RTQ 256
typedef struct {
	int kind; // RT_STR, RT_LIST, RT_DEFER, RT_HANDLE
	void *obj;
	int64_t esize;
	void *fn;
} xo_rtdef;

typedef struct xo_heap {
	// blocks allocated, and blocks freed, by this heap's thread (the
	// leak check reports the difference): one counter per call
	int64_t allocs;
	int64_t frees;
	char *bump_cur;
	char *bump_end;
	void *free_lists[XO_NCLASS + 1];
	int64_t nfree[XO_NCLASS + 1]; // blocks on each free list (about: a magazine taken counts as full)
	void *mag[XO_NCLASS + 1];     // a full magazine per class (threaded: free_mt)
	// the worklist of xo_rc_defer
	xo_deferred *defer_q;
	int64_t defer_n, defer_cap;
	int defer_busy;
	struct xo_heap *next;  // all heaps, for the leak check
	struct xo_heap *spare; // the list of heaps of exited threads
	// real-time sections (decision 0133): calls of real-time functions
	// entered and not left on this thread, allocations since the
	// outermost one began, and the frees it deferred (a fixed queue, so
	// deferring never allocates)
	int64_t rt_depth;
	int64_t rt_allocs;
	int64_t rt_n;
	xo_rtdef rt_q[XO_RTQ];
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

// xo_chunks counts the chunks carved (std/llrt/testdata/tasks.c checks
// that blocks freed on another thread come back instead, decision 0118).
static int64_t xo_chunks;

__attribute__((noinline)) static void xo_refill(xo_heap *h, int64_t need) {
	char *c = malloc(XO_CHUNK);
	if (!c) {
		xo_oom();
	}
	__atomic_fetch_add(&xo_chunks, 1, __ATOMIC_RELAXED);
	(void)need;
	h->bump_cur = c;
	h->bump_end = c + XO_CHUNK;
}

static inline __attribute__((always_inline)) void *alloc_in(xo_heap *h, int64_t n) {
	h->allocs++;
	if (__builtin_expect(h->rt_depth != 0, 0)) {
		h->rt_allocs++; // a real-time section allocated: XO-F007 when it ends
	}
	if (n > XO_MID_MAX) {
		return xo_alloc_large(n);
	}
	int64_t size;
	int64_t c = size_class(n, &size);
	void *p = h->free_lists[c];
	if (p) {
		h->free_lists[c] = *(void **)p;
		h->nfree[c]--;
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
	h->nfree[b[0]]++;
}

// The central lists: per class, a stack of magazines (free lists of
// about central_batch(c) blocks) linked by their first block's second
// word (every block has at least 16 bytes).
static void *central[XO_NCLASS + 1];
static pthread_mutex_t central_mu = PTHREAD_MUTEX_INITIALIZER;

// central_batch is the size of a magazine of class c: 64 KiB of blocks,
// at least 8.
static inline int64_t central_batch(int64_t c) {
	int64_t size = c <= 64 ? c << 4 : (int64_t)2048 << (c - 65);
	int64_t n = (64 << 10) / size;
	return n < 8 ? 8 : n;
}

// central_put pushes magazine m of class c.
__attribute__((noinline)) static void central_put(int64_t c, void *m) {
	pthread_mutex_lock(&central_mu);
	((void **)m)[1] = central[c];
	__atomic_store_n(&central[c], m, __ATOMIC_RELAXED); // read unlocked by central_take
	pthread_mutex_unlock(&central_mu);
}

// central_take pops a magazine of class c, or returns null.
__attribute__((noinline)) static void *central_take(int64_t c) {
	if (!__atomic_load_n(&central[c], __ATOMIC_RELAXED)) {
		return NULL;
	}
	pthread_mutex_lock(&central_mu);
	void *m = central[c];
	if (m) {
		__atomic_store_n(&central[c], ((void **)m)[1], __ATOMIC_RELAXED);
	}
	pthread_mutex_unlock(&central_mu);
	return m;
}

// The threaded paths of xo_alloc and xo_free are tail calls, so the
// single threaded paths stay leaf functions without a frame. With
// threads, a class keeps its free list (nfree long) and at most one full
// spare magazine: an empty list takes the spare, else one from the
// central list; a list that reaches a magazine's size becomes the spare,
// and the spare it replaces goes to the central list. Nothing walks a
// list, so moving blocks between threads costs O(1) per magazine.
__attribute__((noinline)) static void *alloc_mt(int64_t n) {
	xo_heap *h = heap_slow();
	if (n <= XO_MID_MAX) {
		int64_t size;
		int64_t c = size_class(n, &size);
		if (!h->free_lists[c]) {
			void *m = h->mag[c];
			h->mag[c] = NULL;
			if (!m) {
				m = central_take(c);
			}
			if (m) {
				h->free_lists[c] = m;
				h->nfree[c] = central_batch(c);
			}
		}
	}
	return alloc_in(h, n);
}

__attribute__((noinline)) static void free_mt(void *p) {
	xo_heap *h = heap_slow();
	int64_t c = ((int64_t *)p)[-1]; // before free_in: a large block is gone after
	free_in(h, p);
	if (c > 0 && h->nfree[c] >= central_batch(c)) {
		if (h->mag[c]) {
			central_put(c, h->mag[c]);
		}
		h->mag[c] = h->free_lists[c];
		h->free_lists[c] = NULL;
		h->nfree[c] = 0;
	}
}

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

// rt_defer queues a free in the calling thread's real-time section; it
// returns 0 when no section runs or the queue is full (the caller frees
// at once).
static int rt_defer(xo_heap *h, int kind, void *obj, int64_t esize, void *fn);

enum { RT_STR = 1, RT_LIST, RT_DEFER, RT_HANDLE };

void xo_rc_free(void *p) {
	xo_heap *h = heap();
	if (__builtin_expect(h->rt_depth != 0, 0) && rt_defer(h, RT_STR, p, 0, NULL)) {
		return;
	}
	xo_free(p);
}

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
	if (__builtin_expect(h->rt_depth != 0, 0) && !h->defer_busy && rt_defer(h, RT_DEFER, obj, 0, (void *)drop)) {
		return;
	}
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
// The test world (decision 0083, xo_testing.c): a test binary run with
// XO_TEST set runs one test against fakes. tw.on switches the clock,
// timers, standard streams, environment, network listeners, and random
// numbers of this file to the test world; the rest of it is in
// xo_testing.c (included at the end).

static struct {
	int on;
	int64_t vnow;    // the virtual clock: wall time, ns (atomic)
	int vpending;    // every context is blocked: advance the clock (atomic)
	int in_main;     // testing.run_main is running main
	// real capabilities of `xo test --real` (decision 0120): the effects
	// the test declares (TW_REAL_*, from XO_TEST_REAL); virt: the clock
	// is virtual (the test world without `clock` or `net`)
	int real;
	int virt;
} tw;

#define TW_REAL_FS_READ 1
#define TW_REAL_FS_WRITE 2
#define TW_REAL_NET 4
#define TW_REAL_CLOCK 8
#define TW_REAL_ENV 16
#define TW_REAL_STDIO 32
#define TW_REAL_PROC 64

// tw_is_fake reports whether the effect real (TW_REAL_*) is a fake here.
static inline int tw_is_fake(int real) { return tw.on && !(tw.real & real); }

static void tw_write(int err, const char *p, int64_t n);

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

// out_dirty tells that the buffer holds text (atomic): a context about
// to wait flushes it (decision 0069), so a program's output shows before
// it waits for a signal or a connection.
static int out_dirty;

static void flush_locked(void) {
	if (olen > 0) {
		write_all(1, obuf, olen);
		olen = 0;
		__atomic_store_n(&out_dirty, 0, __ATOMIC_RELAXED);
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
	__atomic_store_n(&out_dirty, 1, __ATOMIC_RELAXED);
}

void xo_flush(void) {
	int l = out_lock();
	flush_locked();
	out_unlock(l);
}

// flush_pending writes buffered output before the caller waits.
static void flush_pending(void) {
	if (__atomic_load_n(&out_dirty, __ATOMIC_RELAXED)) {
		xo_flush();
	}
}

void xo_print(const char *p, int64_t n) {
	if (tw_is_fake(TW_REAL_STDIO)) {
		tw_write(0, p, n);
		return;
	}
	int l = out_lock();
	print_locked(p, n);
	out_unlock(l);
}

void xo_println(const char *p, int64_t n) {
	if (tw_is_fake(TW_REAL_STDIO)) {
		tw_write(0, p, n);
		tw_write(0, "\n", 1);
		return;
	}
	int l = out_lock();
	print_locked(p, n);
	print_locked("\n", 1);
	out_unlock(l);
}

void xo_eprint(const char *p, int64_t n) {
	if (tw_is_fake(TW_REAL_STDIO)) {
		tw_write(1, p, n);
		return;
	}
	int l = out_lock();
	flush_locked();
	write_all(2, p, (size_t)n);
	out_unlock(l);
}

void xo_eprintln(const char *p, int64_t n) {
	if (tw_is_fake(TW_REAL_STDIO)) {
		tw_write(1, p, n);
		tw_write(1, "\n", 1);
		return;
	}
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

// A fault unwinds the task that raised it (decision 0085, "Fault
// unwinding" below): fault_text records the message in the running
// context and raises.
XO_NORETURN static void fault_raise_msg(const char *msg, const char *loc);

XO_NORETURN static void fault_text(const char *msg, const char *loc) { fault_raise_msg(msg, loc); }

// xo_fault_detail sets the line printed under the next fault (the values
// of a failed contract, core 9.1).
void xo_fault_detail(const char *p, int64_t n);

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

// xo_rl_new_n is an empty list with room for n elements (at least 4):
// `list.new` with a size, for results whose length is known.
xo_rlist *xo_rl_new_n(int64_t n, int64_t esize) { return xo_rl_new(n < 4 ? 4 : n, esize); }

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
	xo_heap *h = heap();
	if (__builtin_expect(h->rt_depth != 0, 0) && rt_defer(h, RT_LIST, l, esize, (void *)drop)) {
		return;
	}
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
//   - Threads. A pool of worker threads, started on demand, one per CPU
//     (XO_THREADS overrides), each with a run queue; every task runs on
//     a fiber (decision 0069), so a task that waits gives its worker back.
//     A root thread (the program's main) has a run queue too once it
//     spawns, and runs queued work while it waits (decision 0077; see
//     "The worker pool").
//   - Helping. `t.await` on a task that has not started runs it on the
//     awaiting thread, and a scope's join runs its tasks that have not
//     started before it blocks.
//   - Cancellation. Every task and scope body is a node of a tree (a lock
//     per node, taken parent first); canceling a node marks its subtree
//     canceled (sticky) and wakes every context blocked at a node in it.
//     A context's node is its innermost scope body, or its task.
//   - Unwinding. A cancellation point (send, recv, await, waiting for a
//     lock) in a canceled context sets the context's unwinding flag and
//     returns an immortal placeholder; generated code checks xo_unwinding
//     after each call that may unwind and returns, closing the scopes it
//     leaves, up to the task's entry (or to a scope whose child failed,
//     where the unwinding becomes that failure).
//   - Waiting. Every context has a park (a lock and a futex word);
//     objects keep lists of the parks of their waiters and
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
void xo_h_free(void *p) {
	xo_heap *h = heap();
	if (__builtin_expect(h->rt_depth != 0, 0) && rt_defer(h, RT_HANDLE, p, 0, NULL)) {
		return;
	}
	((xo_hdr *)p)->free(p);
}

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

// Runtime statistics (decision 0077): XO_RTSTATS=1 prints counts of the
// scheduler's events to stderr at exit (and xo_rt_stats prints them on
// demand). Each count is one relaxed atomic add, done only when enabled.
static int rtstats = -1; // -1 not read yet
static struct {
	int64_t spawn, steal, inject, wake, idle_sleep, spin_round, fiber_park, fiber_new, thread_wait, thread_wake, lock_slow, spin_yield;
	// sockets: read and write system calls, those that would block
	// (EAGAIN), poller returns, readiness events, and waiters they woke
	int64_t net_read, net_write, net_again, poll_return, poll_event, poll_wake;
	// real-time sections (decision 0133): sections run, frees deferred,
	// frees done at once because the queue was full
	int64_t rt_section, rt_defer, rt_overflow;
} st;
static const char *const st_names[] = {"spawn", "steal", "inject", "wake", "idle_sleep", "spin_round", "fiber_park", "fiber_new", "thread_wait", "thread_wake", "lock_slow", "spin_yield",
                                       "net_read", "net_write", "net_again", "poll_return", "poll_event", "poll_wake",
                                       "rt_section", "rt_defer", "rt_overflow"};

static void stats_init(void) {
	const char *e = getenv("XO_RTSTATS");
	__atomic_store_n(&rtstats, e && *e && *e != '0', __ATOMIC_RELAXED);
}

#define STAT(f)                                                                     \
	do {                                                                            \
		if (__builtin_expect(__atomic_load_n(&rtstats, __ATOMIC_RELAXED) > 0, 0)) { \
			__atomic_fetch_add(&st.f, 1, __ATOMIC_RELAXED);                         \
		}                                                                           \
	} while (0)

void xo_rt_stats(void) {
	if (__atomic_load_n(&rtstats, __ATOMIC_RELAXED) <= 0) {
		return;
	}
	int64_t *v = (int64_t *)&st;
	fputs("xo rtstats:", stderr);
	for (size_t i = 0; i < sizeof st_names / sizeof st_names[0]; i++) {
		fprintf(stderr, " %s=%lld", st_names[i], (long long)__atomic_load_n(&v[i], __ATOMIC_RELAXED));
	}
	fputc('\n', stderr);
}

// ---------------------------------------------------------------------------
// Real-time sections (core 6.5, decision 0133)
//
// A call from code that is not real-time to a function declared `uses
// realtime` is bracketed by xo_realtime_enter and xo_realtime_exit. While
// the thread's depth is above zero, an object whose count reaches zero is
// queued instead of freed (with what it holds: a list's elements, a
// closure's captures), so the section does work bounded by its own code;
// the outermost exit frees the queue. A full queue frees at once and
// counts rt_overflow. An allocation inside a section is counted, and the
// outermost exit faults with XO-F007 when there was one. A fault leaves
// every section of the thread (a real-time function cannot catch it).

static int rt_defer(xo_heap *h, int kind, void *obj, int64_t esize, void *fn) {
	if (h->rt_n == XO_RTQ) {
		STAT(rt_overflow);
		return 0;
	}
	xo_rtdef *e = &h->rt_q[h->rt_n++];
	e->kind = kind;
	e->obj = obj;
	e->esize = esize;
	e->fn = fn;
	STAT(rt_defer);
	return 1;
}

// rt_drain frees what the section deferred; the depth is zero, so what
// those frees release is freed at once.
static void rt_drain(xo_heap *h) {
	for (int64_t i = 0; i < h->rt_n; i++) {
		xo_rtdef e = h->rt_q[i];
		switch (e.kind) {
		case RT_STR:
			xo_free(e.obj);
			break;
		case RT_LIST:
			xo_rl_free(e.obj, e.esize, (xo_elem_fn)e.fn);
			break;
		case RT_DEFER:
			xo_rc_defer(e.obj, (xo_drop_fn)e.fn);
			break;
		case RT_HANDLE:
			((xo_hdr *)e.obj)->free(e.obj);
			break;
		}
	}
	h->rt_n = 0;
}

void xo_realtime_enter(void) {
	xo_heap *h = heap();
	if (h->rt_depth++ == 0) {
		h->rt_allocs = 0;
		if (__builtin_expect(__atomic_load_n(&rtstats, __ATOMIC_RELAXED) < 0, 0)) {
			stats_init();
		}
		STAT(rt_section);
	}
}

void xo_realtime_exit(const char *name, int64_t n, const char *loc, int64_t ln) {
	xo_heap *h = heap();
	if (--h->rt_depth > 0) {
		return;
	}
	rt_drain(h);
	if (h->rt_allocs != 0) {
		char m[256];
		snprintf(m, sizeof m, "XO-F007: real-time function `%.*s` allocated %lld time%s", (int)(n < 128 ? n : 128), name, (long long)h->rt_allocs, h->rt_allocs == 1 ? "" : "s");
		h->rt_allocs = 0;
		fault_at(m, loc, ln);
	}
}

// rt_fault_leave ends the thread's real-time sections when a fault
// unwinds out of them.
static void rt_fault_leave(void) {
	xo_heap *h = heap();
	if (h->rt_depth != 0) {
		h->rt_depth = 0;
		h->rt_allocs = 0;
		rt_drain(h);
	}
}

static inline void spin_relax(void) {
#if defined(__aarch64__)
	__asm__ volatile("yield");
#elif defined(__x86_64__)
	__asm__ volatile("pause");
#endif
}

// Futexes (decision 0077): wait while a 32 bit word holds a value, and
// wake the threads waiting on a word. Darwin has no public futex; its
// __ulock calls are what libc++ uses for std::atomic::wait. Both are
// never inlined: they may set errno (Linux), which must not be read
// across a fiber switch (gotchas/fiber-thread-local-address.md).
#if defined(__APPLE__)
extern int __ulock_wait(uint32_t op, void *addr, uint64_t value, uint32_t timeout_us);
extern int __ulock_wake(uint32_t op, void *addr, uint64_t wake_value);
#define XO_UL_COMPARE_AND_WAIT 1
#define XO_ULF_WAKE_ALL 0x100
#define XO_ULF_NO_ERRNO 0x01000000
__attribute__((noinline)) static void futex_wait(uint32_t *a, uint32_t v) {
	__ulock_wait(XO_UL_COMPARE_AND_WAIT | XO_ULF_NO_ERRNO, a, v, 0);
}
__attribute__((noinline)) static void futex_wake(uint32_t *a) {
	__ulock_wake(XO_UL_COMPARE_AND_WAIT | XO_ULF_NO_ERRNO, a, 0);
}
#else
#include <linux/futex.h>
#include <sys/syscall.h>
__attribute__((noinline)) static void futex_wait(uint32_t *a, uint32_t v) {
	int e = errno;
	syscall(SYS_futex, a, FUTEX_WAIT_PRIVATE, v, NULL, NULL, 0);
	errno = e;
}
__attribute__((noinline)) static void futex_wake(uint32_t *a) {
	int e = errno;
	syscall(SYS_futex, a, FUTEX_WAKE_PRIVATE, 1, NULL, NULL, 0);
	errno = e;
}
#endif

// xo_lock is the lock of the task runtime's objects (decision 0077): a
// word that is 0 (free), 1 (held), or 2 (held, a thread may sleep on
// it). A contended lock spins a little, then sleeps on the word; an
// unlock makes a system call only when someone may sleep. Unlike a
// pthread mutex it needs no initialization (zero is free) and has no
// fair share hand off (gotchas/darwin-mutex-fairshare.md).
typedef struct {
	uint32_t v;
} xo_lock;

__attribute__((noinline)) static void lock_slow(xo_lock *l) {
	STAT(lock_slow);
	for (int i = 0; i < 100; i++) {
		spin_relax();
		uint32_t z = 0;
		if (__atomic_load_n(&l->v, __ATOMIC_RELAXED) == 0 && __atomic_compare_exchange_n(&l->v, &z, 1, 0, __ATOMIC_ACQUIRE, __ATOMIC_RELAXED)) {
			return;
		}
	}
	while (__atomic_exchange_n(&l->v, 2, __ATOMIC_ACQUIRE) != 0) {
		futex_wait(&l->v, 2);
	}
}

static inline void lk(xo_lock *l) {
	uint32_t z = 0;
	if (!__atomic_compare_exchange_n(&l->v, &z, 1, 0, __ATOMIC_ACQUIRE, __ATOMIC_RELAXED)) {
		lock_slow(l);
	}
}

static inline void unlk(xo_lock *l) {
	if (__atomic_exchange_n(&l->v, 0, __ATOMIC_RELEASE) == 2) {
		futex_wake(&l->v);
	}
}

// Deadlock detection (core 11.4, fault XO-F001), decision 0077: the
// program is deadlocked when nothing runs and nothing can wake anything:
// every worker thread is idle and no queue holds an item, every root
// context (a thread that runs Xo code outside tasks: the program's main)
// sleeps in a counted wait, and no context waits for a timer, a socket,
// or a signal (uncounted waits). Fibers parked in counted waits need no
// count: only a running context can wake them. The check runs under
// pool.mu when a worker goes idle and when a root goes to sleep, and
// workers count themselves idle under that lock too, so a worker that
// signaled a root before it went idle is seen with the root awake. (The
// counts used to be updated by every spawn and every task end, an atomic
// that all workers contended for.)
static struct {
	int64_t roots;     // root contexts made (never fewer: decision 0068)
	int64_t blocked;   // roots asleep in a counted wait
	int64_t uncounted; // contexts in a wait a timer, the network, or a signal can end
} dlk;

XO_NORETURN static void deadlock(void);

// mu_init initializes a pthread mutex of the runtime (the event loop's,
// the network's, the HTTP server's). On Darwin the default policy hands
// a contended mutex over in FIFO order through the kernel, which cost a
// spawn heavy program four times its user time in system time; first
// fit lets a running thread take it again
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

// A park is where one context waits. signaled is also the futex word of
// a thread (a root context) that sleeps on it; a fiber that waits parks
// instead (decision 0069). All zero is a fresh park.
typedef struct {
	xo_lock mu;
	uint32_t signaled; // written under mu, read atomically
	int sleeping;      // a thread sleeps on signaled
	int counted;       // a root asleep here counts in dlk.blocked
	// the fiber parked here (decision 0069): a signal makes it runnable
	struct xo_fiber *fiber;
} xo_park;

static void park_init(xo_park *p) { memset(p, 0, sizeof *p); }

static void park_destroy(xo_park *p) { (void)p; }

static void park_reset(xo_park *p) {
	lk(&p->mu);
	__atomic_store_n(&p->signaled, 0, __ATOMIC_RELAXED);
	unlk(&p->mu);
}

struct xo_fiber;
static void fiber_wake(struct xo_fiber *f);

// park_signal wakes the context waiting at p. The waiter takes p->mu
// before it returns, so p stays valid until the unlock here.
static void park_signal(xo_park *p) {
	lk(&p->mu);
	__atomic_store_n(&p->signaled, 1, __ATOMIC_RELEASE);
	if (p->counted) {
		p->counted = 0;
		__atomic_fetch_sub(&dlk.blocked, 1, __ATOMIC_SEQ_CST);
	}
	struct xo_fiber *f = p->fiber;
	if (f) {
		p->fiber = NULL;
		fiber_wake(f);
	} else if (p->sleeping) {
		STAT(thread_wake);
		futex_wake(&p->signaled);
	}
	unlk(&p->mu);
}

// The cancellation tree: every node has a lock for its list of children
// and its waiter. A walk that cancels a subtree holds the locks from the
// node down to the one it visits, so locks are always taken parent
// first; linking and unlinking take only the parent's (decision 0077:
// one lock for the whole tree serialized every spawn and every wait).
typedef struct xo_node {
	struct xo_node *parent, *child, *prev, *next;
	int canceled;    // atomic reads; written under the parent's and its own lock
	xo_park *waiter; // the park of a context blocked at this node (mu)
	xo_lock mu;      // child, the children's prev and next, waiter
} xo_node;

static int node_canceled(xo_node *n) { return __atomic_load_n(&n->canceled, __ATOMIC_ACQUIRE); }

// node_link adds n below parent (canceled at once when parent is).
static void node_link(xo_node *n, xo_node *parent) {
	lk(&parent->mu);
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
	unlk(&parent->mu);
}

// node_unlink takes n out of its parent's children. A task's node is not
// unlinked when the task ends: it goes with its scope's node of tasks
// (xo_scope_close), and nothing walks up from a node.
static void node_unlink(xo_node *n) {
	xo_node *p = n->parent;
	if (!p) {
		return;
	}
	lk(&p->mu);
	if (n->prev) {
		n->prev->next = n->next;
	} else {
		p->child = n->next;
	}
	if (n->next) {
		n->next->prev = n->prev;
	}
	n->parent = n->prev = n->next = NULL;
	unlk(&p->mu);
}

// cancel_locked cancels n and its subtree (n->mu held).
static void cancel_locked(xo_node *n) {
	if (!n->canceled) {
		__atomic_store_n(&n->canceled, 1, __ATOMIC_RELEASE);
	}
	// a canceled node's subtree may have gained children since: walk it
	if (n->waiter) {
		park_signal(n->waiter);
	}
	for (xo_node *c = n->child; c; c = c->next) {
		lk(&c->mu);
		cancel_locked(c);
		unlk(&c->mu);
	}
}

// node_cancel cancels n and its subtree (core 7.2) and wakes every
// context blocked in it.
static void node_cancel(xo_node *n) {
	lk(&n->mu);
	cancel_locked(n);
	unlk(&n->mu);
}

static void node_set_waiter(xo_node *n, xo_park *p) {
	lk(&n->mu);
	n->waiter = p;
	unlk(&n->mu);
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
	// shield counts the deferred code running (core 3.5, decision 0022):
	// while it is not zero, cancellation points do not report
	// cancellation
	int shield;
	xo_park park;
	// the mutexes this context holds (re-entrant use faults, XO-F004)
	xo_held *held;
	int nheld, capheld;
	// the outcome of the last network operation (decision 0069): 0, or
	// the tag of its NetErr (1 InvalidAddr, 2 Refused, 3 Timeout, 4 Io,
	// 5 Canceled) and the message of Io
	int64_t net_status;
	char *net_msg;
	// a fault unwinding this context (decision 0085): its message,
	// location, and the values line of a failed contract (malloc'd)
	int faulting;
	char *fault_msg, *fault_loc, *fault_detail;
	// the frame address the unwinding never passes: the C function that
	// runs the context's code (0: the end of the stack)
	uintptr_t boundary;
	// the exception object of the fault unwinding this context. It lives
	// here, not in fault_raise's frame: a cleanup that resumes the unwind
	// (_Unwind_Resume) reads it after the raising frames are gone, and
	// ThreadSanitizer gives every function such a cleanup (its
	// __tsan_func_exit on unwind; gotchas/tsan-resumes-fault-unwind.md)
	struct _Unwind_Exception fault_exc;
} xo_ctx;

static __thread xo_ctx *cur;
static __thread struct xo_fiber *tls_fib;

// Thread locals are read and written only through these functions, which
// are never inlined (decision 0069): a task on a fiber may go on on
// another thread after it parks, and within one function the compiler may
// keep a thread local's address (it treats it as a constant, also after
// link time inlining), which would then be the old thread's.
__attribute__((noinline)) static xo_ctx *cur_get(void) { return cur; }
__attribute__((noinline)) static void cur_set(xo_ctx *c) { cur = c; }
__attribute__((noinline)) static struct xo_fiber *fib_get(void) { return tls_fib; }
__attribute__((noinline)) static void fib_set(struct xo_fiber *f) { tls_fib = f; }

static pthread_once_t rt_once = PTHREAD_ONCE_INIT;

// rt_init gives the runtime's global mutexes their policy (mu_init)
// before the first context uses them.
static void rt_init(void);

// ctx returns the current context, making a root for a thread that runs
// Xo code outside any task (the program's main thread).
static xo_ctx *ctx(void) {
	xo_ctx *c = cur_get();
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
	cur_set(c);
	__atomic_fetch_add(&dlk.roots, 1, __ATOMIC_SEQ_CST);
	return c;
}

int64_t xo_unwinding(void) {
	xo_ctx *c = cur_get();
	return c ? c->unwinding : 0;
}

// ---------------------------------------------------------------------------
// Fault unwinding (core 8, decision 0085)
//
// A fault is raised as a forced unwind of the platform unwinder
// (_Unwind_ForcedUnwind; libunwind on macOS, libgcc on Linux): every
// frame of generated code has unwind tables, and its landing pads (the
// cleanup paths of internal/ssa's checks, landing.go) run the deferred
// code of the blocks the fault leaves, close their scopes, unlock their
// Mutexes, and release what the frame owns, then hand the fault on
// (xo_fault_continue raises it again from there). The fault is caught by
// the entry function of a task (it ends the task as faulted:
// xo_task_unwound), by `task.isolate` (xo_fault_take), or, past the
// boundary of the context (the C function running the program's main),
// reported and the process exits with status 70. The personality routine
// (xo_personality) reads the call site table of the language specific
// data area that both backends emit (the GCC format for cleanups).

#define XO_EXC_CLASS 0x584f00004641554cULL // "XO\0\0FAUL"

XO_NORETURN static void fault_exit(xo_ctx *c) {
	xo_flush();
	const char *msg = c->fault_msg ? c->fault_msg : "fault";
	const char *loc = c->fault_loc ? c->fault_loc : "?";
	const char *d = c->fault_detail ? c->fault_detail : "";
	char *buf = malloc(strlen(msg) + strlen(loc) + strlen(d) + 32);
	int n = sprintf(buf, "fault: %s at %s\n%s%s", msg, loc, d, *d ? "\n" : "");
	write_all(2, buf, (size_t)n);
	exit(70);
}

static _Unwind_Reason_Code fault_stop(int version, _Unwind_Action actions, _Unwind_Exception_Class cls,
	struct _Unwind_Exception *e, struct _Unwind_Context *uc, void *arg) {
	(void)version;
	(void)cls;
	(void)e;
	xo_ctx *c = arg;
	if (actions & _UA_END_OF_STACK) {
		fault_exit(c);
	}
	if (c->boundary && (uintptr_t)_Unwind_GetCFA(uc) > c->boundary) {
		// the frames past the boundary are the runtime's: nothing there
		// catches a fault
		fault_exit(c);
	}
	return _URC_NO_REASON;
}

static void fault_cleanup(_Unwind_Reason_Code r, struct _Unwind_Exception *e) {
	(void)r;
	(void)e;
}

// fault_raise unwinds the current context with the fault it records. The
// exception object is the context's: generated landing pads never resume
// it (xo_fault_continue raises anew, reusing it), but a sanitizer's
// cleanup in a C frame does, after the frame of this call is gone.
XO_NORETURN static void fault_raise(xo_ctx *c) {
	rt_fault_leave();
	c->faulting = 1;
	c->unwinding = 1;
	struct _Unwind_Exception *exc = &c->fault_exc;
	memset(exc, 0, sizeof *exc);
	exc->exception_class = XO_EXC_CLASS;
	exc->exception_cleanup = fault_cleanup;
	_Unwind_ForcedUnwind(exc, fault_stop, c);
	// the unwinder found no way up
	fault_exit(c);
}

static char *dup_bytes(const char *p, size_t n) {
	char *s = malloc(n + 1);
	if (!s) {
		xo_oom();
	}
	memcpy(s, p, n);
	s[n] = 0;
	return s;
}

// fault_set records a fault in c (a fault raised while one unwinds
// replaces it, as a panic in Go's deferred code does). The detail line,
// set just before by xo_fault_detail, stays.
static void fault_set(xo_ctx *c, const char *msg, const char *loc) {
	free(c->fault_msg);
	free(c->fault_loc);
	c->fault_msg = dup_bytes(msg, strlen(msg));
	c->fault_loc = dup_bytes(loc, strlen(loc));
}

static void fault_clear(xo_ctx *c) {
	c->faulting = 0;
	if (!c->fault_msg && !c->fault_loc && !c->fault_detail) {
		return;
	}
	free(c->fault_msg);
	free(c->fault_loc);
	free(c->fault_detail);
	c->fault_msg = c->fault_loc = c->fault_detail = NULL;
	c->faulting = 0;
}

XO_NORETURN static void fault_raise_msg(const char *msg, const char *loc) {
	xo_ctx *c = ctx();
	fault_set(c, msg, loc);
	fault_raise(c);
}

void xo_fault_detail(const char *p, int64_t n) {
	xo_ctx *c = ctx();
	free(c->fault_detail);
	c->fault_detail = dup_bytes(p, (size_t)n);
}

// xo_fault_raise is `fault(msg)` and a failed contract at loc.
XO_NORETURN void xo_fault_raise(const char *msg, int64_t n, const char *loc, int64_t ln) {
	xo_ctx *c = ctx();
	free(c->fault_msg);
	free(c->fault_loc);
	c->fault_msg = dup_bytes(msg, (size_t)n);
	c->fault_loc = dup_bytes(loc, (size_t)ln);
	fault_raise(c);
}

// xo_fault_continue runs at the unwind exit of a function: a fault goes
// on to the caller; otherwise (cancellation) the function returns.
void xo_fault_continue(void) {
	xo_ctx *c = cur_get();
	if (c && c->faulting) {
		fault_raise(c);
	}
}

// xo_landing is never called: generated code branches on it only to
// mark a check that only a fault takes (internal/ssa landing.go).
int64_t xo_landing(void) { return 0; }

// xo_fault_take catches the fault unwinding the current context (for
// `task.isolate`): 1 and the context goes on normally, its message
// available to xo_fault_msg and xo_fault_loc; 0 when it is not a fault
// (cancellation, which goes on unwinding).
int64_t xo_fault_take(void) {
	xo_ctx *c = ctx();
	if (!c->faulting) {
		return 0;
	}
	c->faulting = 0;
	c->unwinding = 0;
	return 1;
}

static xo_rstr *rs_of_cstr(const char *s, int64_t *len) {
	int64_t n = s ? (int64_t)strlen(s) : 0;
	xo_rstr *r = rs_new(n);
	if (n) {
		memcpy(r->data, s, (size_t)n);
	}
	*len = n;
	return r;
}

// xo_fault_msg is the message of the fault xo_fault_take caught.
xo_rstr *xo_fault_msg(int64_t *len) { return rs_of_cstr(ctx()->fault_msg, len); }

// xo_fault_loc is its location; the record is cleared after it.
xo_rstr *xo_fault_loc(int64_t *len) {
	xo_ctx *c = ctx();
	xo_rstr *r = rs_of_cstr(c->fault_loc, len);
	fault_clear(c);
	return r;
}

// xo_personality is the personality routine of generated code: every
// landing pad is a cleanup, entered on the second phase. The language
// specific data is GCC's: an LPStart and a TType encoding (omitted), the
// call site encoding, and the call sites (start, length, landing pad,
// action), offsets from the function's start.
static uintptr_t lsda_read(const uint8_t **p, uint8_t enc) {
	const uint8_t *q = *p;
	uintptr_t v = 0;
	switch (enc & 0x0f) {
	case 0x01: { // uleb128
		int shift = 0;
		uint8_t byte;
		do {
			byte = *q++;
			v |= (uintptr_t)(byte & 0x7f) << shift;
			shift += 7;
		} while (byte & 0x80);
		break;
	}
	case 0x02: {
		uint16_t x;
		memcpy(&x, q, 2);
		q += 2;
		v = x;
		break;
	}
	case 0x03:
	case 0x0b: {
		uint32_t x;
		memcpy(&x, q, 4);
		q += 4;
		v = (enc & 0x0f) == 0x0b ? (uintptr_t)(intptr_t)(int32_t)x : x;
		break;
	}
	default: { // absptr, udata8, sdata8
		uint64_t x;
		memcpy(&x, q, 8);
		q += 8;
		v = (uintptr_t)x;
		break;
	}
	}
	*p = q;
	return v;
}

static uintptr_t uleb(const uint8_t **p) { return lsda_read(p, 0x01); }

_Unwind_Reason_Code xo_personality(int version, _Unwind_Action actions, uint64_t cls,
	struct _Unwind_Exception *e, struct _Unwind_Context *uc) {
	(void)cls;
	if (version != 1) {
		return _URC_FATAL_PHASE1_ERROR;
	}
	if (!(actions & _UA_CLEANUP_PHASE)) {
		return _URC_CONTINUE_UNWIND;
	}
	const uint8_t *p = (const uint8_t *)_Unwind_GetLanguageSpecificData(uc);
	if (!p) {
		return _URC_CONTINUE_UNWIND;
	}
	uintptr_t func = _Unwind_GetRegionStart(uc);
	uintptr_t ip = _Unwind_GetIP(uc) - 1;
	uint8_t lpenc = *p++;
	if (lpenc != 0xff) {
		lsda_read(&p, lpenc);
	}
	uint8_t ttenc = *p++;
	if (ttenc != 0xff) {
		uleb(&p);
	}
	uint8_t csenc = *p++;
	uintptr_t cslen = uleb(&p);
	const uint8_t *end = p + cslen;
	while (p < end) {
		uintptr_t start = lsda_read(&p, csenc);
		uintptr_t len = lsda_read(&p, csenc);
		uintptr_t lp = lsda_read(&p, csenc);
		uleb(&p); // action
		if (ip < func + start) {
			break;
		}
		if (ip < func + start + len) {
			if (lp == 0) {
				return _URC_CONTINUE_UNWIND;
			}
			_Unwind_SetGR(uc, __builtin_eh_return_data_regno(0), (uintptr_t)e);
			_Unwind_SetGR(uc, __builtin_eh_return_data_regno(1), 0);
			_Unwind_SetIP(uc, func + lp);
			return _URC_INSTALL_CONTEXT;
		}
	}
	return _URC_CONTINUE_UNWIND;
}

int64_t xo_is_canceled(void) { return node_canceled(ctx()->node); }

static int64_t mono_ns(void) {
	struct timespec ts;
	clock_gettime(CLOCK_MONOTONIC, &ts);
	return (int64_t)ts.tv_sec * 1000000000 + ts.tv_nsec;
}

// tnow is the time of timers: monotonic ns, or the virtual clock in the
// test world.
static int64_t tnow(void) { return tw.virt ? __atomic_load_n(&tw.vnow, __ATOMIC_ACQUIRE) : mono_ns(); }

// xo_shield_begin starts deferred code (core 3.5, decision 0022): it is
// shielded from cancellation, and on the unwind path the unwinding flag
// is clear while it runs. The result is the token xo_shield_end takes:
// the start time and the flag.
int64_t xo_shield_begin(void) {
	xo_ctx *c = ctx();
	int64_t tok = (mono_ns() << 1) | (c->unwinding & 1);
	c->unwinding = 0;
	c->shield++;
	return tok;
}

// xo_shield_end ends deferred code started with token tok: the flag is
// set again, and deferred code that ran longer than 5 seconds faults
// (XO-F005, as the Go backend checks it when the code returns).
void xo_shield_end(int64_t tok, const char *loc, int64_t n) {
	xo_ctx *c = ctx();
	c->shield--;
	c->unwinding = (int)(tok & 1);
	int64_t el = mono_ns() - (tok >> 1);
	if (el > 5000000000LL) {
		char msg[128];
		snprintf(msg, sizeof msg, "XO-F005: a shielded defer ran for %.3gs, longer than 5s", (double)el / 1e9);
		fault_at(msg, loc, n);
	}
}

// xo_shield_abort ends deferred code started with token tok that faulted
// (decision 0085): the shield ends and the flag is set again as
// xo_shield_end does, without its time check; the rest of the cleanup
// runs on (internal/ssa faultCont). Without it the context stayed
// shielded, and a task that caught the fault (task.isolate) never saw
// cancellation again.
void xo_shield_abort(int64_t tok) {
	xo_ctx *c = ctx();
	c->shield--;
	c->unwinding = (int)(tok & 1);
}

// ctx_canceled reports whether c is canceled and not shielded.
static int ctx_canceled(xo_ctx *c) { return !c->shield && node_canceled(c->node); }

// canceled reports whether a cancellation point in c reports
// cancellation (and marks c unwinding when it does).
static int canceled(xo_ctx *c) {
	if (ctx_canceled(c)) {
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
// The worker pool (decisions 0068, 0069, 0077)
//
// A queue item is a task to start, or a parked fiber to resume (its
// pointer with the low bit set, decision 0069). Every worker thread, and
// every root thread once it spawns or waits, has a record with
//   - a run queue (Go's runq): a ring of 256 items that only the owner
//     adds to; the owner takes from the head with a compare and swap,
//     and a thief takes half of the items with one compare and swap. A
//     full ring moves half of itself to the inject queue in one batch;
//   - next: one fiber its thread woke (Go's runnext), run before the
//     ring, so a fiber that wakes another and then waits hands over on
//     the same thread; others take it only on their last look for work.
// Items from threads without a record (the poller) go to the inject
// queue, a locked queue that a worker takes a share of at once. Idle workers sleep on a futex word of their own. Waking follows
// Go's spinning rule: a push wakes an idle worker only when no worker is
// spinning (looking for work), the woken worker spins, and a spinning
// worker that finds work wakes the next one only if it was the last
// spinner. So a burst of spawns wakes workers one at a time and a steady
// stream of hand overs wakes nobody.

#define XO_RUNQ 256

typedef struct {
	uint32_t head; // atomic: consumers (the owner and thieves) CAS it
	char pad[124];
	uint32_t tail;        // atomic: only the owner writes it
	void *slot[XO_RUNQ]; // atomic accesses: a thief may read a stale one
} xo_runq;

// rq_put adds x (the owner); a full ring moves half of itself and x to
// the inject queue.
static int rq_put_slow(xo_runq *q, void *x, uint32_t h, uint32_t t);

static void rq_put(xo_runq *q, void *x) {
	for (;;) {
		uint32_t h = __atomic_load_n(&q->head, __ATOMIC_ACQUIRE);
		uint32_t t = q->tail;
		if (t - h < XO_RUNQ) {
			__atomic_store_n(&q->slot[t % XO_RUNQ], x, __ATOMIC_RELAXED);
			// sequentially consistent: ordered before the pusher's look at
			// the spinning and idle counts (wakep), as a worker going idle
			// counts itself idle before it looks at the queues again
			__atomic_store_n(&q->tail, t + 1, __ATOMIC_SEQ_CST);
			return;
		}
		if (rq_put_slow(q, x, h, t)) {
			return;
		}
	}
}

// rq_get takes the item at the head (the owner).
static void *rq_get(xo_runq *q) {
	for (;;) {
		uint32_t h = __atomic_load_n(&q->head, __ATOMIC_ACQUIRE);
		uint32_t t = q->tail;
		if (t == h) {
			return NULL;
		}
		void *x = __atomic_load_n(&q->slot[h % XO_RUNQ], __ATOMIC_RELAXED);
		if (__atomic_compare_exchange_n(&q->head, &h, h + 1, 0, __ATOMIC_RELEASE, __ATOMIC_RELAXED)) {
			return x;
		}
	}
}

// rq_grab takes half of q's items (rounded up) into batch (a thief) and
// returns how many.
static uint32_t rq_grab(xo_runq *q, void **batch) {
	for (;;) {
		uint32_t h = __atomic_load_n(&q->head, __ATOMIC_ACQUIRE);
		uint32_t t = __atomic_load_n(&q->tail, __ATOMIC_ACQUIRE);
		uint32_t n = t - h;
		n -= n / 2;
		if (n == 0) {
			return 0;
		}
		if (n > XO_RUNQ / 2) {
			continue; // h and t read at different times: again
		}
		for (uint32_t i = 0; i < n; i++) {
			batch[i] = __atomic_load_n(&q->slot[(h + i) % XO_RUNQ], __ATOMIC_RELAXED);
		}
		if (__atomic_compare_exchange_n(&q->head, &h, h + n, 0, __ATOMIC_RELEASE, __ATOMIC_RELAXED)) {
			return n;
		}
	}
}

// rq_put_batch adds n items to an empty ring (the owner).
static void rq_put_batch(xo_runq *q, void **batch, uint32_t n) {
	uint32_t t = q->tail;
	for (uint32_t i = 0; i < n; i++) {
		__atomic_store_n(&q->slot[(t + i) % XO_RUNQ], batch[i], __ATOMIC_RELAXED);
	}
	__atomic_store_n(&q->tail, t + n, __ATOMIC_SEQ_CST);
}

static int rq_nonempty(xo_runq *q) {
	return __atomic_load_n(&q->head, __ATOMIC_SEQ_CST) != __atomic_load_n(&q->tail, __ATOMIC_SEQ_CST);
}

#define XO_MAXW 4096
#define LOAD(x) __atomic_load_n(&(x), __ATOMIC_SEQ_CST)
#define STORE(x, v) __atomic_store_n(&(x), (v), __ATOMIC_SEQ_CST)
#define XO_FCACHE 8

typedef struct xo_worker {
	xo_runq q;
	void *next;      // atomic: a fiber its thread woke
	int64_t next_at; // atomic: when it was put there (mono_ns)
	int live;
	int root;       // the record of a root thread that helps while it waits
	void *sched_sp; // the scheduler's stack pointer while a fiber runs
	void *tsan;     // the scheduler's ThreadSanitizer fiber
	uint32_t wakew; // futex word: set to 1 to wake the idle worker
	struct xo_worker *idle_next;
	uint64_t rng;
	int nonext; // pool_push puts woken fibers in the ring (mutex_signal)
	struct xo_fiber *fcache[XO_FCACHE]; // stacks to reuse (owner only)
	int nfcache;
	// AddressSanitizer: the scheduler's stack (learned at the first
	// switch) and its fake stack while a fiber runs (ASAN_START below;
	// last, so the fields above keep their cache lines)
	const void *asan_bottom;
	size_t asan_size;
	void *asan_fake;
} xo_worker;

static struct {
	pthread_once_t once;
	xo_lock mu; // the idle list, starting workers
	int ncpu;
	int spin_rounds;
	xo_worker *ws[XO_MAXW]; // records (worker and root), never freed
	int nw;                 // records used (atomic)
	int alive;              // worker threads (atomic)
	int nidle;              // idle worker threads (atomic)
	int spinning;           // spinning worker threads (atomic)
	xo_worker *idle;        // the idle list (mu)
	// the inject queue: items from threads without a record
	xo_lock imu;
	void **ibuf;
	int64_t ihead, ilen, icap; // ilen read atomically to skip it
} pool = {.once = PTHREAD_ONCE_INIT};

static __thread xo_worker *tls_w;

__attribute__((noinline)) static xo_worker *w_get(void) { return tls_w; }
__attribute__((noinline)) static void w_set(xo_worker *w) { tls_w = w; }

static void rt_init(void) {}

static void pool_init(void) {
	xo_rt_threads_begin();
	stats_init();
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
	pool.spin_rounds = 16;
	e = getenv("XO_SPIN_ROUNDS"); // tuning (decision 0077)
	if (e && atoi(e) >= 0) {
		pool.spin_rounds = atoi(e);
	}
	__atomic_store_n(&pool.ncpu, (int)n, __ATOMIC_RELEASE);
}

// register_worker adds w to the records thieves look at (pool.mu held).
static void register_worker(xo_worker *w) {
	if (pool.nw >= XO_MAXW) {
		xo_oom();
	}
	w->rng = (uint64_t)(uintptr_t)w * 0x9E3779B97F4A7C15ull | 1;
	pool.ws[pool.nw] = w;
	STORE(pool.nw, pool.nw + 1);
}

static void *worker_main(void *arg);

// start_worker starts a worker thread, which starts spinning (pool.mu
// held, a spinning count already taken for it).
static int start_worker(void) {
	xo_worker *w = calloc(1, sizeof(xo_worker));
	if (!w) {
		xo_oom();
	}
	w->live = 1;
	register_worker(w);
	STORE(pool.alive, pool.alive + 1);
	pthread_attr_t attr;
	pthread_attr_init(&attr);
	pthread_attr_setstacksize(&attr, (size_t)64 << 20);
	pthread_attr_setdetachstate(&attr, PTHREAD_CREATE_DETACHED);
	pthread_t th;
	int ok = pthread_create(&th, &attr, worker_main, w) == 0;
	pthread_attr_destroy(&attr);
	if (!ok) {
		w->live = 0;
		STORE(pool.alive, pool.alive - 1);
	}
	return ok;
}

// wakep makes sure queued work gets a worker: unless one is spinning, an
// idle worker is woken (or a new one started while fewer than one per CPU
// run) as a spinning worker.
static void wakep(void) {
	if (LOAD(pool.spinning) != 0) {
		return;
	}
	if (LOAD(pool.nidle) == 0 && LOAD(pool.alive) >= pool.ncpu) {
		return;
	}
	int z = 0;
	if (!__atomic_compare_exchange_n(&pool.spinning, &z, 1, 0, __ATOMIC_SEQ_CST, __ATOMIC_SEQ_CST)) {
		return;
	}
	lk(&pool.mu);
	xo_worker *w = pool.idle;
	if (w) {
		pool.idle = w->idle_next;
		w->idle_next = NULL;
		__atomic_fetch_sub(&pool.nidle, 1, __ATOMIC_SEQ_CST);
		unlk(&pool.mu);
		STAT(wake);
		__atomic_store_n(&w->wakew, 1, __ATOMIC_RELEASE);
		futex_wake(&w->wakew);
		return;
	}
	int started = pool.alive < pool.ncpu && start_worker();
	unlk(&pool.mu);
	if (!started) {
		__atomic_fetch_sub(&pool.spinning, 1, __ATOMIC_SEQ_CST);
	}
}

// inject_push_batch adds n items to the inject queue.
static void inject_push_batch(void **items, int64_t n) {
	lk(&pool.imu);
	if (pool.ilen + n > pool.icap) {
		int64_t nc = pool.icap ? pool.icap * 2 : 64;
		while (nc < pool.ilen + n) {
			nc *= 2;
		}
		void **nb = malloc((size_t)nc * sizeof(*nb));
		if (!nb) {
			xo_oom();
		}
		for (int64_t i = 0; i < pool.ilen; i++) {
			nb[i] = pool.ibuf[(pool.ihead + i) % pool.icap];
		}
		free(pool.ibuf);
		pool.ibuf = nb;
		pool.ihead = 0;
		pool.icap = nc;
	}
	for (int64_t i = 0; i < n; i++) {
		pool.ibuf[(pool.ihead + pool.ilen + i) % pool.icap] = items[i];
	}
	__atomic_store_n(&pool.ilen, pool.ilen + n, __ATOMIC_SEQ_CST);
	unlk(&pool.imu);
}

static void inject_push(void *t) { inject_push_batch(&t, 1); }

static int rq_put_slow(xo_runq *q, void *x, uint32_t h, uint32_t t) {
	void *batch[XO_RUNQ / 2 + 1];
	uint32_t n = (t - h) / 2;
	for (uint32_t i = 0; i < n; i++) {
		batch[i] = __atomic_load_n(&q->slot[(h + i) % XO_RUNQ], __ATOMIC_RELAXED);
	}
	if (!__atomic_compare_exchange_n(&q->head, &h, h + n, 0, __ATOMIC_RELEASE, __ATOMIC_RELAXED)) {
		return 0; // thieves took some: the ring has room again
	}
	batch[n] = x;
	inject_push_batch(batch, n + 1);
	return 1;
}

// inject_pop takes a share of the inject queue for w: one item to run,
// the rest into w's empty ring.
static void *inject_pop(xo_worker *w) {
	if (__atomic_load_n(&pool.ilen, __ATOMIC_SEQ_CST) == 0) {
		return NULL;
	}
	void *batch[XO_RUNQ / 2];
	int64_t n = 0;
	lk(&pool.imu);
	if (pool.ilen > 0) {
		n = pool.ilen / (pool.ncpu + 1) + 1;
		if (n > pool.ilen) {
			n = pool.ilen;
		}
		if (n > XO_RUNQ / 2) {
			n = XO_RUNQ / 2;
		}
		if (!w) {
			n = 1;
		}
		for (int64_t i = 0; i < n; i++) {
			batch[i] = pool.ibuf[pool.ihead];
			pool.ihead = (pool.ihead + 1) % pool.icap;
		}
		__atomic_store_n(&pool.ilen, pool.ilen - n, __ATOMIC_RELAXED);
	}
	unlk(&pool.imu);
	if (n == 0) {
		return NULL;
	}
	STAT(inject);
	if (n > 1) {
		rq_put_batch(&w->q, batch + 1, (uint32_t)(n - 1));
	}
	return batch[0];
}

// pool_push queues t: a woken fiber goes to the waking thread's next
// (the one there before moves to its ring) unless the thread asked for
// its ring (nonext), a new task to its ring, and either from a thread
// without a record to the inject queue.
static void pool_push(void *t) {
	xo_worker *w = w_get();
	if (!w) {
		inject_push(t);
	} else if (((uintptr_t)t & 1) && !w->nonext) {
		__atomic_store_n(&w->next_at, mono_ns(), __ATOMIC_RELAXED);
		void *old = __atomic_exchange_n(&w->next, t, __ATOMIC_SEQ_CST);
		if (old) {
			rq_put(&w->q, old);
		}
	} else {
		rq_put(&w->q, t);
	}
	wakep();
}

// take_local takes w's own work: next, then the head of its ring.
static void *take_local(xo_worker *w) {
	void *t = NULL;
	if (__atomic_load_n(&w->next, __ATOMIC_RELAXED)) {
		t = __atomic_exchange_n(&w->next, NULL, __ATOMIC_ACQ_REL);
	}
	if (!t) {
		t = rq_get(&w->q);
	}
	return t;
}

// XO_NEXT_GRACE_NS is how long a fiber stays in its waker's next before
// a thief may take it: the waker usually waits soon after it woke a
// fiber and then runs it itself, with no thread wake up (decision 0077).
#define XO_NEXT_GRACE_NS 50000

// steal_round looks once at every other record, from a random one, and
// takes half of the first ring it finds items in: one to run, the rest
// into w's (empty) ring; next only when last is set and it waited there
// longer than the grace.
static void *steal_round(xo_worker *w, int last) {
	int n = LOAD(pool.nw);
	if (n == 0) {
		return NULL;
	}
	w->rng ^= w->rng << 13;
	w->rng ^= w->rng >> 7;
	w->rng ^= w->rng << 17;
	int start = (int)(w->rng % (uint64_t)n);
	for (int i = 0; i < n; i++) {
		xo_worker *v = pool.ws[(start + i) % n];
		if (v == w) {
			continue;
		}
		void *batch[XO_RUNQ / 2];
		void *t = NULL;
		uint32_t got = rq_nonempty(&v->q) ? rq_grab(&v->q, batch) : 0;
		if (got > 0) {
			t = batch[got - 1];
			if (got > 1) {
				rq_put_batch(&w->q, batch, got - 1);
			}
		}
		if (!t && last) {
			void *x = __atomic_load_n(&v->next, __ATOMIC_ACQUIRE);
			if (x && mono_ns() - __atomic_load_n(&v->next_at, __ATOMIC_RELAXED) > XO_NEXT_GRACE_NS && __atomic_compare_exchange_n(&v->next, &x, NULL, 0, __ATOMIC_ACQ_REL, __ATOMIC_RELAXED)) {
				t = x;
			}
		}
		if (t) {
			STAT(steal);
			return t;
		}
	}
	return NULL;
}

// any_work reports whether some queue holds an item (a worker about to
// sleep looks again after it counted itself idle).
static int any_work(void) {
	if (__atomic_load_n(&pool.ilen, __ATOMIC_SEQ_CST) > 0) {
		return 1;
	}
	int n = LOAD(pool.nw);
	for (int i = 0; i < n; i++) {
		xo_worker *v = pool.ws[i];
		if (__atomic_load_n(&v->next, __ATOMIC_SEQ_CST) || rq_nonempty(&v->q)) {
			return 1;
		}
	}
	return 0;
}

// find_work looks for an item for w: its own, the inject queue, then, if
// spinning, steals for a few rounds (the last one also takes next).
static void *find_work(xo_worker *w, int spinning) {
	void *t = take_local(w);
	if (!t) {
		t = inject_pop(w);
	}
	if (t || !spinning) {
		return t;
	}
	int rounds = pool.spin_rounds;
	for (int r = 0; r <= rounds && !t; r++) {
		STAT(spin_round);
		t = steal_round(w, r == rounds);
		if (!t) {
			t = inject_pop(w);
		}
		if (!t && r < rounds) {
			for (int i = 0; i < 32; i++) {
				spin_relax();
			}
		}
	}
	return t;
}

XO_NORETURN static void deadlock(void) {
	xo_flush();
	static const char m[] = "fault: XO-F001: deadlock: every task is blocked\n";
	write_all(2, m, sizeof m - 1);
	exit(70);
}

// dl_check_locked faults when the program is deadlocked (pool.mu held;
// see dlk).
static void dl_check_locked(void) {
	int64_t roots = __atomic_load_n(&dlk.roots, __ATOMIC_SEQ_CST);
	if (roots > 0 && __atomic_load_n(&dlk.blocked, __ATOMIC_SEQ_CST) == roots && LOAD(pool.nidle) == LOAD(pool.alive) &&
	    __atomic_load_n(&dlk.uncounted, __ATOMIC_SEQ_CST) == 0 && !any_work()) {
		if (tw.virt) {
			// the test world's clock jumps to the next timer
			// (tw_kick, after pool.mu is released), or it deadlocked
			__atomic_store_n(&tw.vpending, 1, __ATOMIC_RELEASE);
			return;
		}
		deadlock();
	}
}

static void tw_kick(void);

static void fiber_park(struct xo_fiber *f);
static void fiber_parking(struct xo_fiber *f);
static void run_item(xo_worker *w, void *it);
static void tsan_worker(xo_worker *w);

// root_worker returns the record of the calling root thread, made on
// first use: its spawns go to its deque, and while it waits it runs work
// (decision 0077).
static xo_worker *root_worker(void) {
	xo_worker *w = w_get();
	if (w) {
		return w;
	}
	pthread_once(&pool.once, pool_init);
	w = calloc(1, sizeof(xo_worker));
	if (!w) {
		xo_oom();
	}
	w->live = 1;
	w->root = 1;
	tsan_worker(w);
	lk(&pool.mu);
	register_worker(w);
	unlk(&pool.mu);
	w_set(w);
	return w;
}

// park_wait_n blocks the current context until its park is signaled or,
// when cancelable, its node is canceled. A task on a fiber parks: its
// worker runs other work meanwhile (decision 0069). A root thread runs
// queued work while it waits, and sleeps only when it finds none
// (decision 0077). counted tells that the wait counts as blocked for
// deadlock detection; a wait that a timer, the network, or a signal can
// end does not.
static void park_wait_n(xo_ctx *c, int cancelable, int counted) {
	flush_pending();
	if (tw.virt) {
		// in the test world nothing outside the program ends a wait:
		// timers are virtual and fire only when every context waits
		counted = 1;
	}
	if (c->shield) {
		cancelable = 0;
	}
	if (cancelable) {
		node_set_waiter(c->node, &c->park);
	}
	struct xo_fiber *f = fib_get();
	// a root thread helps once the pool exists (a program that only
	// sleeps stays single threaded)
	xo_worker *rw = f || !__atomic_load_n(&pool.ncpu, __ATOMIC_ACQUIRE) ? NULL : root_worker();
	xo_park *p = &c->park;
	if (!counted) {
		__atomic_fetch_add(&dlk.uncounted, 1, __ATOMIC_SEQ_CST);
	}
	lk(&p->mu);
	if (!p->signaled && !(cancelable && node_canceled(c->node))) {
		while (!p->signaled && !(cancelable && node_canceled(c->node))) {
			if (f) {
				p->fiber = f;
				fiber_parking(f);
				unlk(&p->mu);
				fiber_park(f);
				lk(&p->mu);
				p->fiber = NULL;
				continue;
			}
			unlk(&p->mu);
			void *it = NULL;
			if (rw) {
				it = find_work(rw, 0);
				if (!it) {
					it = steal_round(rw, 0);
				}
			}
			if (it) {
				run_item(rw, it);
				cur_set(c);
				lk(&p->mu);
				continue;
			}
			lk(&p->mu);
			if (p->signaled || (cancelable && node_canceled(c->node))) {
				break;
			}
			p->sleeping = 1;
			p->counted = counted;
			unlk(&p->mu);
			if (counted) {
				// a signal meanwhile cleared counted and took one off
				lk(&pool.mu);
				__atomic_fetch_add(&dlk.blocked, 1, __ATOMIC_SEQ_CST);
				dl_check_locked();
				unlk(&pool.mu);
				tw_kick();
			}
			STAT(thread_wait);
			futex_wait(&p->signaled, 0);
			lk(&p->mu);
			p->sleeping = 0;
			if (p->counted) {
				// not signaled (a spurious wake up)
				p->counted = 0;
				__atomic_fetch_sub(&dlk.blocked, 1, __ATOMIC_SEQ_CST);
			}
		}
	}
	__atomic_store_n(&p->signaled, 0, __ATOMIC_RELAXED);
	unlk(&p->mu);
	if (!counted) {
		__atomic_fetch_sub(&dlk.uncounted, 1, __ATOMIC_SEQ_CST);
	}
	if (cancelable) {
		node_set_waiter(c->node, NULL);
	}
}

static void park_wait(xo_ctx *c, int cancelable) { park_wait_n(c, cancelable, 1); }

// A wait list: the parks of the contexts waiting on an object, signaled
// all at once when its state changes (they check again).
typedef struct xo_went {
	xo_park *park;
	struct xo_went *prev, *next;
	int writer;  // a Mutex waiter that wants the lock exclusively
	int granted; // a Mutex reader the lock was handed to (atomic, mutex_grant_readers)
} xo_went;

typedef struct {
	xo_went *first, *last;
} xo_wl;

// wl_add adds e at the end of l (the oldest waiter is first).
static void wl_add(xo_wl *l, xo_went *e) {
	e->next = NULL;
	e->prev = l->last;
	if (l->last) {
		l->last->next = e;
	} else {
		l->first = e;
	}
	l->last = e;
}

static void wl_del(xo_wl *l, xo_went *e) {
	if (e->prev) {
		e->prev->next = e->next;
	} else {
		l->first = e->next;
	}
	if (e->next) {
		e->next->prev = e->prev;
	} else {
		l->last = e->prev;
	}
}

static void wl_wake(xo_wl *l) {
	for (xo_went *e = l->first; e; e = e->next) {
		park_signal(e->park);
	}
}

// wl_wait_as waits on list l of an object whose lock mu is held; mu is
// held again on return. It reports 0 when the context was canceled.
// writer marks a Mutex waiter that wants the lock exclusively.
static int wl_wait_as(xo_ctx *c, xo_lock *mu, xo_wl *l, int cancelable, int writer) {
	xo_went e = {.park = &c->park, .writer = writer};
	park_reset(&c->park);
	wl_add(l, &e);
	unlk(mu);
	park_wait(c, cancelable);
	lk(mu);
	wl_del(l, &e);
	return !(cancelable && ctx_canceled(c));
}

static int wl_wait(xo_ctx *c, xo_lock *mu, xo_wl *l, int cancelable) { return wl_wait_as(c, mu, l, cancelable, 0); }

// ---------------------------------------------------------------------------
// Scopes and tasks

typedef struct xo_scope {
	xo_hdr h;
	xo_node body;
	xo_node *tasks;       // the node of the scope's tasks (replaced by s.cancel())
	xo_node **old;        // earlier task nodes, freed with the scope
	int nold;
	xo_node *parent_node; // the opening context's node before the scope
	xo_lock mu;           // list, fail, closed, tasks
	// atomic: two for each task not finished and each queue entry of its
	// tasks not taken yet, plus 1 while the joiner waits for zero. Every
	// worker writes it: padded off the lines the spawning thread uses.
	char pad0[128];
	int64_t pending;
	xo_park *joiner; // the joiner's park (set before it sets bit 0)
	char pad1[128];
	struct xo_task *list; // every task, each holding a reference, until the scope closes
	struct xo_task *fail; // the first task that failed (a reference)
	int closed;
	// task.with_timeout (decision 0069): the timer that cancels the body,
	// and that it fired (atomic)
	struct xo_timer *deadline;
	int timed_out;
	struct xo_task *fault; // the first task that faulted (a reference, decision 0085)
	// list's length, and the length at which a spawn prunes it next
	// (scope_prune); closing: the join started, the list stays as it is
	int64_t nlist, prune_at;
	int closing;
} xo_scope;

static void timer_del(struct xo_timer *t);

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
	int unwound;      // the task ended by unwinding (canceled, or a fault)
	int background, consumed;
	int64_t site;
	xo_lock mu; // waiters
	xo_wl waiters;
	struct xo_task *snext;
	int gone; // atomic: 2 when its run and its queue entry are over (scope_prune)
	// the fault that ended it (decision 0085), malloc'd (last: the fields
	// above keep their places)
	char *fault_msg, *fault_loc, *fault_detail;
} xo_task;

enum { T_QUEUED, T_RUNNING, T_DONE };

static void scope_free(void *p) {
	xo_scope *s = p;
	if (s->fail) {
		h_release(s->fail);
	}
	if (s->fault) {
		h_release(s->fault);
	}
	free(s->tasks);
	for (int i = 0; i < s->nold; i++) {
		free(s->old[i]);
	}
	free(s->old);
	xo_free(s);
}

xo_scope *xo_scope_open(void) {
	xo_ctx *c = ctx();
	xo_scope *s = xo_alloc(sizeof(xo_scope));
	memset(s, 0, sizeof *s);
	h_init(s, scope_free);
	s->prune_at = 64;
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

// The program's task group, `os.tasks` (core 7.1, decision 0109): the
// group of the scope generated code opens around main (and around each
// test), set while it is open.
static xo_scope *prog_group;

// xo_prog_group_set makes s (NULL: none) the group os.tasks gives.
void xo_prog_group_set(xo_scope *s) {
	if (s) {
		h_retain(s);
	}
	xo_scope *old = __atomic_exchange_n(&prog_group, s, __ATOMIC_ACQ_REL);
	if (old) {
		h_release(old);
	}
}

// xo_prog_group_clear ends the program's group (its scope closed).
void xo_prog_group_clear(void) { xo_prog_group_set(NULL); }

// xo_prog_group is `os.tasks`: a reference to the program's group.
xo_scope *xo_prog_group(void) {
	xo_scope *s = __atomic_load_n(&prog_group, __ATOMIC_ACQUIRE);
	if (!s) {
		static const char m[] = "os.tasks is not available here";
		xo_fault(m, (int64_t)sizeof m - 1, "");
	}
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
	lk(&s->mu);
	xo_node *old = s->tasks;
	s->old = realloc(s->old, (size_t)(s->nold + 1) * sizeof(xo_node *));
	if (!s->old) {
		xo_oom();
	}
	s->old[s->nold++] = old;
	node_link(fresh, &s->body);
	s->tasks = fresh;
	unlk(&s->mu);
	node_cancel(old);
}

static void task_free(void *p) {
	xo_task *t = p;
	if (t->result) {
		list_release(t->result, t->drop);
	}
	if (t->fault_msg) {
		free(t->fault_msg);
		free(t->fault_loc);
		free(t->fault_detail);
	}
	xo_free(t);
}

// scope_units_done records that one of s's tasks ended or one of its
// queue entries was taken (units 2), or both (4); the last one wakes the
// joiner if it waits. It is
// the caller's last touch of s (decision 0077: the counts were under the
// scope's mutex, which every worker took twice per task).
static void scope_units_done(xo_scope *s, int64_t units) {
	if (__atomic_sub_fetch(&s->pending, units, __ATOMIC_ACQ_REL) == 1) {
		// the joiner waits for this signal, so s is still there
		park_signal(s->joiner);
	}
}

// claim_task reports whether the caller is the one to run t (nobody
// started it).
static int claim_task(xo_task *t) {
	int64_t q = T_QUEUED;
	return __atomic_compare_exchange_n(&t->state, &q, T_RUNNING, 0, __ATOMIC_ACQ_REL, __ATOMIC_ACQUIRE);
}

// run_claimed runs t, which the caller claimed, on this stack (a fiber,
// or a thread that helps).
static void run_claimed(xo_task *t, int64_t units) {
	xo_ctx c = {.node = &t->node};
	park_init(&c.park);
	// the entry function catches a fault in the task; the unwinding never
	// goes past this frame (decision 0085)
	c.boundary = (uintptr_t)__builtin_frame_address(0);
	xo_ctx *saved = cur_get();
	cur_set(&c);
	xo_rlist *fl = t->fl;
	t->fl = NULL;
	h_retain(t); // the entry function's own reference
	t->run(fl, t);
	cur_set(saved);
	park_destroy(&c.park);
	free(c.held);
	free(c.net_msg);
	fault_clear(&c);
	xo_scope *s = t->scope;
	lk(&t->mu);
	__atomic_store_n(&t->state, T_DONE, __ATOMIC_RELEASE);
	wl_wake(&t->waiters);
	unlk(&t->mu);
	// the last touch of t: a spawn may prune it after (scope_prune)
	__atomic_fetch_add(&t->gone, (int)(units / 2), __ATOMIC_RELEASE);
	// the last touch of s: the scope's join may free it after
	scope_units_done(s, units);
}

// run_task runs t on this thread if nobody has started it.
static void run_task(xo_task *t) {
	if (claim_task(t)) {
		run_claimed(t, 2); // its queue entry is still queued
	}
}

// dequeued records that a worker took t's queue entry (and ran it, or
// found it run by a helper). The scope's join waits for this too, so no
// queue holds a task the scope frees.
static void dequeued(xo_task *t) {
	xo_scope *s = t->scope;
	__atomic_fetch_add(&t->gone, 1, __ATOMIC_RELEASE); // the last touch of t
	scope_units_done(s, 2);
}

// scope_prune takes the tasks that are over and that only s's list
// holds out of the list (s->mu held) and returns them, linked by snext,
// for scope_drop_pruned. A scope that lives long and starts many tasks
// whose handles are dropped (a server's connections, decision 0118) kept
// every one, with its result, until it closed: about 1 KB per connection.
// A task is over when its run ended and its queue entry was taken
// (gone == 2: nothing touches it after either); with a count of one
// (shared: -1) nobody else has it, and nobody can get it again. The
// close never prunes (its helpers walk the list without the lock), and a
// spawn prunes only when the list doubled since the last time.
static xo_task *scope_prune(xo_scope *s) {
	xo_task *dead = NULL;
	xo_task **pp = &s->list;
	while (*pp) {
		xo_task *u = *pp;
		if (__atomic_load_n(&u->gone, __ATOMIC_ACQUIRE) == 2 && __atomic_load_n(&u->h.rc, __ATOMIC_ACQUIRE) == -1) {
			*pp = u->snext;
			u->snext = dead;
			dead = u;
			s->nlist--;
		} else {
			pp = &u->snext;
		}
	}
	s->prune_at = s->nlist * 2 > 64 ? s->nlist * 2 : 64;
	return dead;
}

// scope_drop_pruned unlinks the pruned tasks' nodes and drops the list's
// references (without s->mu: a task's result may run drop code).
static void scope_drop_pruned(xo_task *dead) {
	while (dead) {
		xo_task *next = dead->snext;
		node_unlink(&dead->node);
		h_release(dead);
		dead = next;
	}
}

// ---------------------------------------------------------------------------
// Fibers (decision 0069): every task a worker starts runs on a stack of
// its own, so a task that waits (a channel, a lock, a timer, a socket)
// parks and its worker runs other work meanwhile. A parked fiber is
// resumed by whoever signals its park: it goes back to a queue (its
// pointer with the low bit set) and any worker continues it. Stacks are
// mapped with a guard page below and kept in a small cache.
//
// Parking is a handshake on f->state, so a wake up never gets lost and a
// fiber never runs on two workers: the fiber sets F_PARKING under its
// park's lock and switches to its worker's scheduler, which then moves it
// to F_PARKED; a waker swaps in F_WOKEN and queues the fiber only if it
// saw F_PARKED. A scheduler that finds F_WOKEN instead runs it again.

enum { F_RUNNING, F_PARKING, F_PARKED, F_WOKEN };

typedef struct xo_fiber {
	void *sp;    // the saved stack pointer while switched out
	char *mem;   // the mapping: a guard page, then the stack
	size_t size; // of the mapping
	xo_task *task;
	xo_ctx *cur;   // the context running on it (a task it helps is nested)
	int64_t state; // F_* (atomic)
	int done;
	void *tsan;
	struct xo_fiber *next; // the cache
	void *asan_fake;       // AddressSanitizer: its fake stack while switched out
} xo_fiber;

// xo_fiber_switch saves the callee saved registers on the current stack,
// stores the stack pointer in *save, and continues on the stack to (saved
// by an earlier switch, or made by fiber_new). xo_fiber_start is where a
// new fiber starts: it calls the function in the second register slot
// with the first as its argument.
void xo_fiber_switch(void **save, void *to);
void xo_fiber_start(void);

#if defined(__APPLE__)
#define XO_ASM_FN(n) ".globl _" n "\n.private_extern _" n "\n.p2align 4\n_" n ":\n"
#else
#define XO_ASM_FN(n) ".globl " n "\n.hidden " n "\n.type " n ", @function\n.p2align 4\n" n ":\n"
#endif

#if defined(__aarch64__)
// AAPCS64: x19 to x28, the frame pointer, the link register, and d8 to
// d15 are callee saved (x18 is the platform's on Darwin: not touched).
__asm__(".text\n" XO_ASM_FN("xo_fiber_switch")
	"sub sp, sp, #160\n"
	"stp x19, x20, [sp, #0]\n"
	"stp x21, x22, [sp, #16]\n"
	"stp x23, x24, [sp, #32]\n"
	"stp x25, x26, [sp, #48]\n"
	"stp x27, x28, [sp, #64]\n"
	"stp x29, x30, [sp, #80]\n"
	"stp d8, d9, [sp, #96]\n"
	"stp d10, d11, [sp, #112]\n"
	"stp d12, d13, [sp, #128]\n"
	"stp d14, d15, [sp, #144]\n"
	"mov x9, sp\n"
	"str x9, [x0]\n"
	"mov sp, x1\n"
	"ldp x19, x20, [sp, #0]\n"
	"ldp x21, x22, [sp, #16]\n"
	"ldp x23, x24, [sp, #32]\n"
	"ldp x25, x26, [sp, #48]\n"
	"ldp x27, x28, [sp, #64]\n"
	"ldp x29, x30, [sp, #80]\n"
	"ldp d8, d9, [sp, #96]\n"
	"ldp d10, d11, [sp, #112]\n"
	"ldp d12, d13, [sp, #128]\n"
	"ldp d14, d15, [sp, #144]\n"
	"add sp, sp, #160\n"
	"ret\n"
	XO_ASM_FN("xo_fiber_start")
	"mov x0, x19\n"
	"blr x20\n"
	"brk #1\n");
#define XO_FRAME 160
#elif defined(__x86_64__)
// System V: rbx, rbp, r12 to r15, and the control words of MXCSR and the
// x87 unit are callee saved.
__asm__(".text\n" XO_ASM_FN("xo_fiber_switch")
	"pushq %rbp\n"
	"pushq %rbx\n"
	"pushq %r12\n"
	"pushq %r13\n"
	"pushq %r14\n"
	"pushq %r15\n"
	"subq $8, %rsp\n"
	"stmxcsr (%rsp)\n"
	"fnstcw 4(%rsp)\n"
	"movq %rsp, (%rdi)\n"
	"movq %rsi, %rsp\n"
	"ldmxcsr (%rsp)\n"
	"fldcw 4(%rsp)\n"
	"addq $8, %rsp\n"
	"popq %r15\n"
	"popq %r14\n"
	"popq %r13\n"
	"popq %r12\n"
	"popq %rbx\n"
	"popq %rbp\n"
	"ret\n"
	XO_ASM_FN("xo_fiber_start")
	"movq %rbx, %rdi\n"
	"callq *%r12\n"
	"ud2\n");
#define XO_FRAME 64
#else
#error "xo_rt.c: fibers support arm64 and x86_64"
#endif

// ThreadSanitizer must be told about every switch (its fiber API).
#if defined(__has_feature)
#if __has_feature(thread_sanitizer)
#define XO_TSAN 1
#endif
#endif
#if defined(__SANITIZE_THREAD__)
#define XO_TSAN 1
#endif

#ifdef XO_TSAN
void *__tsan_get_current_fiber(void);
void *__tsan_create_fiber(unsigned flags);
void __tsan_destroy_fiber(void *fiber);
void __tsan_switch_to_fiber(void *fiber, unsigned flags);
static void tsan_worker(xo_worker *w) { w->tsan = __tsan_get_current_fiber(); }
#define TSAN_SWITCH(x) __tsan_switch_to_fiber((x), 0)
#else
static void tsan_worker(xo_worker *w) { (void)w; }
#define TSAN_SWITCH(x) ((void)(x))
#endif

// AddressSanitizer must know which stack runs (its fiber API): a fault
// unwinds with __asan_handle_no_return, which unpoisons the current stack
// up to its top (decision 0085).
#if defined(__has_feature)
#if __has_feature(address_sanitizer)
#define XO_ASAN 1
#endif
#endif
#if defined(__SANITIZE_ADDRESS__)
#define XO_ASAN 1
#endif

#ifdef XO_ASAN
void __sanitizer_start_switch_fiber(void **fake_stack_save, const void *bottom, size_t size);
void __sanitizer_finish_switch_fiber(void *fake_stack_save, const void **bottom_old, size_t *size_old);
#define ASAN_START(save, bottom, size) __sanitizer_start_switch_fiber((save), (bottom), (size))
#define ASAN_FINISH(save, bottom, size) __sanitizer_finish_switch_fiber((save), (bottom), (size))
#else
#define ASAN_START(save, bottom, size) ((void)(save), (void)(bottom), (void)(size))
#define ASAN_FINISH(save, bottom, size) ((void)(save), (void)(bottom), (void)(size))
#endif

#ifdef MAP_NORESERVE
#define XO_MAP_NORESERVE MAP_NORESERVE
#else
#define XO_MAP_NORESERVE 0
#endif

#define XO_FIBER_CACHE 64

static struct {
	xo_lock mu;
	xo_fiber *list;
	int n;
	size_t stack; // bytes, without the guard page
	pthread_once_t once;
} fibers = {.once = PTHREAD_ONCE_INIT};

static void fibers_init(void) {
	// 64 MiB reserved, like the worker threads before fibers (deep
	// recursion in a task behaves the same); XO_TASK_STACK_MB overrides
	size_t mb = 64;
	const char *e = getenv("XO_TASK_STACK_MB");
	if (e && atoi(e) > 0) {
		mb = (size_t)atoi(e);
	}
	fibers.stack = mb << 20;
}

static void fiber_main(xo_fiber *f);

// fiber_new returns a fiber that starts running t (claimed) when it is
// first switched to.
static xo_fiber *fiber_new(xo_worker *w, xo_task *t) {
	xo_fiber *f = NULL;
	if (w->nfcache > 0) {
		f = w->fcache[--w->nfcache];
	} else if (__atomic_load_n(&fibers.list, __ATOMIC_RELAXED)) {
		lk(&fibers.mu);
		if (fibers.list) {
			f = fibers.list;
			__atomic_store_n(&fibers.list, f->next, __ATOMIC_RELAXED);
			fibers.n--;
		}
		unlk(&fibers.mu);
	}
	if (!f) {
		pthread_once(&fibers.once, fibers_init);
		STAT(fiber_new);
		f = calloc(1, sizeof(xo_fiber));
		if (!f) {
			xo_oom();
		}
		size_t page = (size_t)sysconf(_SC_PAGESIZE);
		f->size = fibers.stack + page;
		f->mem = mmap(NULL, f->size, PROT_READ | PROT_WRITE, MAP_PRIVATE | MAP_ANON | XO_MAP_NORESERVE, -1, 0);
		if (f->mem == MAP_FAILED) {
			xo_oom();
		}
		mprotect(f->mem, page, PROT_NONE);
#ifdef XO_TSAN
		f->tsan = __tsan_create_fiber(0);
#endif
	}
	f->task = t;
	f->cur = NULL;
	f->done = 0;
	__atomic_store_n(&f->state, F_RUNNING, __ATOMIC_RELAXED);
	uintptr_t top = ((uintptr_t)f->mem + f->size) & ~(uintptr_t)15;
	void **fr = (void **)(top - XO_FRAME);
	memset(fr, 0, XO_FRAME);
#if defined(__aarch64__)
	fr[0] = f;                     // x19
	fr[1] = (void *)fiber_main;    // x20
	fr[11] = (void *)xo_fiber_start; // x30
#else
	((uint32_t *)fr)[0] = 0x1F80; // MXCSR: exceptions masked, round to nearest
	((uint16_t *)fr)[2] = 0x037F; // x87 control word
	fr[4] = (void *)fiber_main;    // r12
	fr[5] = f;                     // rbx
	fr[7] = (void *)xo_fiber_start; // return address
#endif
	f->sp = fr;
	return f;
}

// fiber_free keeps f's stack for reuse: in its worker's cache, else the
// shared one (decision 0077: a shared cache only took a lock per task).
static void fiber_free(xo_worker *w, xo_fiber *f) {
	if (w->nfcache < XO_FCACHE) {
		w->fcache[w->nfcache++] = f;
		return;
	}
	lk(&fibers.mu);
	if (fibers.n < XO_FIBER_CACHE) {
		f->next = fibers.list;
		__atomic_store_n(&fibers.list, f, __ATOMIC_RELAXED);
		fibers.n++;
		unlk(&fibers.mu);
		return;
	}
	unlk(&fibers.mu);
#ifdef XO_TSAN
	__tsan_destroy_fiber(f->tsan);
#endif
	munmap(f->mem, f->size);
	free(f);
}

// fiber_main runs on the fiber: the task, then back to the scheduler for
// good.
static void fiber_main(xo_fiber *f) {
#ifdef XO_ASAN
	{
		xo_worker *w = w_get();
		ASAN_FINISH(NULL, &w->asan_bottom, &w->asan_size);
	}
#endif
	// the fiber's task came from a queue: its entry and its run end here
	run_claimed(f->task, 4);
	f->done = 1;
	xo_worker *w = w_get();
	TSAN_SWITCH(w->tsan);
	ASAN_START(NULL, w->asan_bottom, w->asan_size); // for good
	xo_fiber_switch(&f->sp, w->sched_sp);
	__builtin_trap();
}

// fiber_parking marks f about to park (its park's lock held).
static void fiber_parking(xo_fiber *f) { __atomic_store_n(&f->state, F_PARKING, __ATOMIC_RELEASE); }

// fiber_park switches from f to the scheduler of the worker running it;
// it returns when a worker resumes f.
static void fiber_park(xo_fiber *f) {
	STAT(fiber_park);
	f->cur = cur_get();
	xo_worker *w = w_get();
	TSAN_SWITCH(w->tsan);
	ASAN_START(&f->asan_fake, w->asan_bottom, w->asan_size);
	xo_fiber_switch(&f->sp, w->sched_sp);
#ifdef XO_ASAN
	// resumed, maybe by another worker
	w = w_get();
	ASAN_FINISH(f->asan_fake, &w->asan_bottom, &w->asan_size);
#endif
}

// fiber_wake makes a parked (or parking) fiber run again.
static void fiber_wake(xo_fiber *f) {
	if (__atomic_exchange_n(&f->state, F_WOKEN, __ATOMIC_ACQ_REL) == F_PARKED) {
		pool_push((void *)((uintptr_t)f | 1));
	}
}

// run_fiber runs f on worker w until it parks or ends. The thread's
// current context is restored after (a root thread runs fibers while it
// waits).
static void run_fiber(xo_worker *w, xo_fiber *f) {
	xo_ctx *saved = cur_get();
	for (;;) {
		__atomic_store_n(&f->state, F_RUNNING, __ATOMIC_RELAXED);
		fib_set(f);
		cur_set(f->cur);
		TSAN_SWITCH(f->tsan);
		ASAN_START(&w->asan_fake, f->mem + (f->size - fibers.stack), fibers.stack);
		xo_fiber_switch(&w->sched_sp, f->sp);
		ASAN_FINISH(w->asan_fake, NULL, NULL);
		fib_set(NULL);
		cur_set(saved);
		if (f->done) {
			fiber_free(w, f);
			return;
		}
		int64_t st = F_PARKING;
		if (__atomic_compare_exchange_n(&f->state, &st, F_PARKED, 0, __ATOMIC_ACQ_REL, __ATOMIC_ACQUIRE)) {
			return; // a waker queues it
		}
		// woken while it parked: run it again
	}
}

// run_item runs a queue item on w's thread: resumes a fiber, or starts a
// task on a new fiber unless a helper ran it already.
static void run_item(xo_worker *w, void *it) {
	if ((uintptr_t)it & 1) {
		run_fiber(w, (struct xo_fiber *)((uintptr_t)it & ~(uintptr_t)1));
		return;
	}
	xo_task *t = it;
	if (!claim_task(t)) {
		// run by a thread that helped
		dequeued(t);
		return;
	}
	// claimed: the entry is counted taken when the task ends (one atomic
	// on the scope instead of two)
	run_fiber(w, fiber_new(w, t));
}

// go_idle puts w on the idle list and sleeps until wakep takes it off;
// it returns 0 instead when work showed up meanwhile (w is then off the
// list again and not spinning).
static int go_idle(xo_worker *w) {
	lk(&pool.mu);
	__atomic_store_n(&w->wakew, 0, __ATOMIC_RELAXED);
	w->idle_next = pool.idle;
	pool.idle = w;
	__atomic_fetch_add(&pool.nidle, 1, __ATOMIC_SEQ_CST);
	dl_check_locked();
	unlk(&pool.mu);
	tw_kick();
	// a push before the count went up was not seen by its wakep: look
	if (any_work()) {
		lk(&pool.mu);
		xo_worker **pp = &pool.idle;
		while (*pp && *pp != w) {
			pp = &(*pp)->idle_next;
		}
		if (*pp) {
			*pp = w->idle_next;
			w->idle_next = NULL;
			__atomic_fetch_sub(&pool.nidle, 1, __ATOMIC_SEQ_CST);
			unlk(&pool.mu);
			return 0;
		}
		unlk(&pool.mu);
		// wakep took it off the list and counted it spinning: wait for
		// its wake up
	}
	while (__atomic_load_n(&w->wakew, __ATOMIC_ACQUIRE) == 0) {
		STAT(idle_sleep);
		futex_wait(&w->wakew, 0);
	}
	return 1;
}

static void *worker_main(void *arg) {
	xo_worker *w = arg;
	w_set(w);
	tsan_worker(w);
	int spinning = 1; // wakep started it as a spinning worker
	for (;;) {
		void *it = find_work(w, spinning);
		if (it) {
			if (spinning) {
				spinning = 0;
				// the last spinner found work: another one looks for more
				if (__atomic_sub_fetch(&pool.spinning, 1, __ATOMIC_SEQ_CST) == 0) {
					wakep();
				}
			}
			run_item(w, it);
			continue;
		}
		if (spinning) {
			spinning = 0;
			__atomic_fetch_sub(&pool.spinning, 1, __ATOMIC_SEQ_CST);
		} else if (2 * LOAD(pool.spinning) < LOAD(pool.alive) - LOAD(pool.nidle)) {
			// at most half the busy workers spin (Go's rule)
			__atomic_fetch_add(&pool.spinning, 1, __ATOMIC_SEQ_CST);
			spinning = 1;
			continue;
		}
		if (go_idle(w)) {
			spinning = 1; // wakep counted it spinning
		} else {
			__atomic_fetch_add(&pool.spinning, 1, __ATOMIC_SEQ_CST);
			spinning = 1;
		}
	}
	return NULL;
}

// xo_task_spawn starts a task in scope s (`s.spawn(f)`, flags bit 0:
// background, bit 1: its handle is bound and awaited, so its error does
// not fail the scope). fl is the closure in a list of one (consumed);
// run is the entry function that calls it, drop the one that releases
// its result list, fdrop the one that releases fl (a spawn on a scope
// that has ended faults and gives it back, decision 0085). site tells the
// scope which spawn failed.
xo_task *xo_task_spawn(xo_scope *s, xo_rlist *fl, int64_t run, int64_t drop, int64_t fdrop, int64_t flags, int64_t site, const char *loc, int64_t n) {
	pthread_once(&pool.once, pool_init);
	STAT(spawn);
	if (!w_get()) {
		root_worker();
	}
	xo_task *t = xo_alloc(sizeof(xo_task));
	memset(t, 0, sizeof *t);
	h_init(t, task_free);
	// references: the handle returned, and the scope's list (which keeps
	// t alive while a queue holds it)
	t->h.rc = -2;
	t->run = (xo_run_fn)run;
	t->drop = (xo_ldrop)drop;
	t->fl = fl;
	t->background = (int)(flags & 1);
	t->consumed = (int)((flags >> 1) & 1);
	t->site = site;
	lk(&s->mu);
	if (s->closed) {
		unlk(&s->mu);
		xo_free(t);
		list_release(fl, (xo_ldrop)fdrop);
		fault_at("a task was started on a scope that has ended", loc, n);
	}
	t->scope = s;
	// counted before a helper can find it in the list: the task runs and
	// its queue entry is taken (two units of two)
	__atomic_fetch_add(&s->pending, 4, __ATOMIC_ACQ_REL);
	node_link(&t->node, s->tasks);
	t->snext = s->list;
	s->list = t;
	s->nlist++;
	xo_task *dead = NULL;
	if (s->nlist >= s->prune_at && !s->closing) {
		dead = scope_prune(s);
	}
	unlk(&s->mu);
	pool_push(t);
	scope_drop_pruned(dead);
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
	lk(&s->mu);
	int first = s->fail == NULL;
	if (first) {
		h_retain(t);
		s->fail = t;
	}
	unlk(&s->mu);
	if (first) {
		// the body and every task (core 7.1, child failure)
		node_cancel(&s->body);
	}
}

// xo_task_unwound ends the current task that unwound (canceled), or
// faulted (decision 0085): its scope then treats it like a failed child
// (cancels the body and every task) and raises the fault where it closes.
void xo_task_unwound(xo_task *t) {
	xo_ctx *c = cur_get();
	c->unwinding = 0;
	t->unwound = 1;
	if (!c->faulting) {
		return;
	}
	c->faulting = 0;
	t->fault_msg = c->fault_msg;
	t->fault_loc = c->fault_loc;
	t->fault_detail = c->fault_detail;
	c->fault_msg = c->fault_loc = c->fault_detail = NULL;
	xo_scope *s = t->scope;
	lk(&s->mu);
	int first = s->fault == NULL;
	if (first) {
		h_retain(t);
		s->fault = t;
	}
	unlk(&s->mu);
	if (first) {
		node_cancel(&s->body);
	}
}

// xo_fault_rethrow raises the fault of a task of s in the context that
// closed s (status 3 of xo_scope_close).
XO_NORETURN void xo_fault_rethrow(xo_scope *s) {
	xo_ctx *c = ctx();
	xo_task *t = s->fault;
	fault_clear(c);
	c->fault_msg = dup_bytes(t->fault_msg, strlen(t->fault_msg));
	c->fault_loc = dup_bytes(t->fault_loc, strlen(t->fault_loc));
	if (t->fault_detail) {
		c->fault_detail = dup_bytes(t->fault_detail, strlen(t->fault_detail));
	}
	fault_raise(c);
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
		lk(&t->mu);
		while (__atomic_load_n(&t->state, __ATOMIC_ACQUIRE) != T_DONE) {
			if (!wl_wait(c, &t->mu, &t->waiters, 1)) {
				unlk(&t->mu);
				c->unwinding = 1;
				return &empty_list;
			}
		}
		unlk(&t->mu);
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
	if (s->deadline) {
		timer_del(s->deadline);
		free(s->deadline);
		s->deadline = NULL;
	}
	lk(&s->mu);
	s->closing = 1; // no pruning from here: the helpers below walk the list unlocked
	if (mode == 0) {
		for (xo_task *t = s->list; t; t = t->snext) {
			if (t->background) {
				node_cancel(&t->node);
			}
		}
	} else {
		node_cancel(s->tasks);
	}
	unlk(&s->mu);
	// help: run the tasks that have not started on this thread (tasks
	// spawned meanwhile go to the head of the list: look again until a
	// pass finds none; a task's link is fixed once it is in the list)
	for (int found = 1; found;) {
		found = 0;
		lk(&s->mu);
		xo_task *t = s->list;
		unlk(&s->mu);
		for (; t; t = t->snext) {
			if (__atomic_load_n(&t->state, __ATOMIC_ACQUIRE) == T_QUEUED) {
				run_task(t);
				found = 1;
			}
		}
	}
	// wait (not a cancellation point: a canceled body still waits) until
	// every task ended and every queue entry was taken: set bit 0 of
	// pending while it is not zero, and the last unit wakes the park
	s->joiner = &c->park;
	for (;;) {
		park_reset(&c->park);
		int64_t v = __atomic_load_n(&s->pending, __ATOMIC_ACQUIRE);
		if (v >> 1 == 0) {
			break;
		}
		if (!(v & 1) && !__atomic_compare_exchange_n(&s->pending, &v, v | 1, 0, __ATOMIC_ACQ_REL, __ATOMIC_ACQUIRE)) {
			continue;
		}
		park_wait(c, 0);
	}
	lk(&s->mu);
	s->closed = 1;
	xo_task *list = s->list;
	s->list = NULL;
	unlk(&s->mu);
	// the tasks' nodes hang below s->tasks (and the nodes s.cancel()
	// replaced): unlink those before the tasks go
	node_unlink(s->tasks);
	for (int i = 0; i < s->nold; i++) {
		node_unlink(s->old[i]);
	}
	node_unlink(&s->body);
	while (list) {
		xo_task *next = list->snext;
		h_release(list);
		list = next;
	}
	c->node = s->parent_node;
	if (c->faulting) {
		// the body's own fault goes on (decision 0085)
		return 0;
	}
	if (s->fault) {
		// a task faulted: the caller raises its fault here
		return 3;
	}
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
	xo_lock mu;
	int64_t cap, head, len;
	xo_rlist **buf;
	int closed;
	// closed from outside the program's tasks (task.stopping()): waiting
	// for it does not count as blocked
	int ext;
	xo_ldrop drop;
	xo_cq recvq, sendq;
} xo_chan;

static void chan_free(void *p) {
	xo_chan *c = p;
	for (int64_t i = 0; i < c->len; i++) {
		list_release(c->buf[(c->head + i) % c->cap], c->drop);
	}
	free(c->buf);
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
	lk(&c->mu);
	if (c->closed) {
		unlk(&c->mu);
		list_release(l, c->drop); // the fault unwinds the sender (decision 0085)
		fault_at("send on a closed channel", loc, n);
	}
	if (try_send_locked(c, l)) {
		unlk(&c->mu);
		return;
	}
	int64_t fired = 0;
	xo_cent e = {.park = &x->park, .fired = &fired, .item = l};
	park_reset(&x->park);
	cq_push(&c->sendq, &e);
	unlk(&c->mu);
	for (;;) {
		park_wait_n(x, 1, !c->ext);
		if (__atomic_load_n(&fired, __ATOMIC_ACQUIRE)) {
			break;
		}
		if (ctx_canceled(x)) {
			lk(&c->mu);
			if (claim(&fired)) {
				cq_del(&c->sendq, &e);
				unlk(&c->mu);
				list_release(l, c->drop);
				x->unwinding = 1;
				return;
			}
			unlk(&c->mu);
			break; // taken meanwhile: sent
		}
	}
	lk(&c->mu); // the waker's writes
	int shut = e.shut;
	unlk(&c->mu);
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
	lk(&c->mu);
	if (try_recv_locked(c, &out)) {
		unlk(&c->mu);
		return out ? out : &empty_list;
	}
	int64_t fired = 0;
	xo_cent e = {.park = &x->park, .fired = &fired};
	park_reset(&x->park);
	cq_push(&c->recvq, &e);
	unlk(&c->mu);
	for (;;) {
		park_wait_n(x, 1, !c->ext);
		if (__atomic_load_n(&fired, __ATOMIC_ACQUIRE)) {
			break;
		}
		if (ctx_canceled(x)) {
			lk(&c->mu);
			if (claim(&fired)) {
				cq_del(&c->recvq, &e);
				unlk(&c->mu);
				x->unwinding = 1;
				return &empty_list;
			}
			unlk(&c->mu);
			break; // a value came meanwhile: it wins
		}
	}
	lk(&c->mu);
	out = e.item;
	unlk(&c->mu);
	return out ? out : &empty_list;
}

// xo_chan_try_send is `ch.try_send(x)`: false (and x released) when the
// channel is full or closed.
int64_t xo_chan_try_send(xo_chan *c, xo_rlist *l) {
	lk(&c->mu);
	int ok = !c->closed && try_send_locked(c, l);
	unlk(&c->mu);
	if (!ok) {
		list_release(l, c->drop);
	}
	return ok;
}

// xo_chan_try_recv is `ch.try_recv()`: never blocks.
xo_rlist *xo_chan_try_recv(xo_chan *c) {
	xo_rlist *out = NULL;
	lk(&c->mu);
	if (!try_recv_locked(c, &out)) {
		out = NULL;
	}
	unlk(&c->mu);
	return out ? out : &empty_list;
}

// xo_chan_close is `ch.close()`: waiting receivers get None, waiting
// senders fault. Closing twice has no effect.
void xo_chan_close(xo_chan *c) {
	lk(&c->mu);
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
	unlk(&c->mu);
}

int64_t xo_chan_len(xo_chan *c) {
	lk(&c->mu);
	int64_t n = c->len;
	unlk(&c->mu);
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
	// an `after` arm (xo_select_after): its duration, and the park its
	// timer signals
	int has_after;
	int64_t after;
	xo_park *park;
} xo_select;

struct xo_timer;
static void select_timer(xo_select *s, struct xo_timer **tp);
static void select_untimer(struct xo_timer *t);

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
		lk(&cs[i]->mu);
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
	if (done < 0 && s->has_after && s->after <= 0) {
		done = -2;
	}
	int queued = done < 0;
	int counted = !s->has_after;
	if (queued) {
		for (int i = 0; i < s->n; i++) {
			counted = counted && !s->arms[i].ch->ext;
			xo_sarm *a = &s->arms[i];
			a->e = (xo_cent){.park = &x->park, .fired = &s->fired, .item = a->item, .idx = i, .chosen = &s->chosen};
			cq_push(a->send ? &a->ch->sendq : &a->ch->recvq, &a->e);
		}
	}
	for (int i = nc - 1; i >= 0; i--) {
		unlk(&cs[i]->mu);
	}
	int unwound = 0;
	if (queued) {
		struct xo_timer *tm = NULL;
		if (s->has_after) {
			s->park = &x->park;
			select_timer(s, &tm);
		}
		for (;;) {
			park_wait_n(x, 1, counted);
			if (__atomic_load_n(&s->fired, __ATOMIC_ACQUIRE)) {
				break;
			}
			if (ctx_canceled(x)) {
				if (claim(&s->fired)) {
					unwound = 1;
				}
				break;
			}
		}
		if (tm) {
			select_untimer(tm);
		}
		// take the arms off every queue; the waker's writes are visible
		// under the channel locks
		for (int i = 0; i < nc; i++) {
			lk(&cs[i]->mu);
		}
		for (int i = 0; i < s->n; i++) {
			xo_sarm *a = &s->arms[i];
			cq_del(a->send ? &a->ch->sendq : &a->ch->recvq, &a->e);
		}
		if (!unwound && (done = (int)__atomic_load_n(&s->chosen, __ATOMIC_ACQUIRE)) >= 0) {
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
			unlk(&cs[i]->mu);
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
	xo_lock mu;
	int writer;
	int64_t readers, wwait, rwait; // rwait: readers in waiters
	xo_rlist *slot;
	xo_ldrop drop;
	xo_wl waiters;
} xo_mutex;

// mutex_wake wakes the waiters that can take m now (m->mu held). Phase
// fair, as Go's RWMutex: after a writer's release the readers waiting
// then get the lock together (handed over: mutex_grant_readers), and the
// next writer waits for them; after the last reader's release (or when
// no reader waits) the oldest waiting writer is woken. Waking every
// waiter at each release made 32 contending request tasks park about 12
// times per request and convoy on m->mu
// (bench/results/perf-throughput-2026-10-08.md, "Native backends");
// waking readers that then checked again parked them again whenever a
// writer had queued meanwhile, 3.6 parks per access with 64 tasks
// (bench/results/native-runtime-2026-10-10.md, decision 0118). A woken
// writer checks again and waits again when another writer took m first
// (barging); whoever holds m wakes the next one when it releases, so no
// wake up is lost.
// mutex_signal wakes a waiter of a Mutex into the waking thread's ring,
// not its next: the waker still holds the thread for the rest of its
// request, and a fiber in next waits for that (thieves leave next alone
// for XO_NEXT_GRACE_NS) while the lock sits free; in the ring a spinning
// worker takes it at once. Measured on bench/perf/http: p99 900 to 800
// us (bench/results/perf-throughput-2026-10-08.md, "Native backends").
static void mutex_signal(xo_park *p) {
	xo_worker *w = w_get();
	if (w) {
		w->nonext = 1;
	}
	park_signal(p);
	if (w) {
		w->nonext = 0;
	}
}

// mutex_grant_readers hands m to every waiting reader (m->mu held, no
// writer holds m): each is counted in m->readers, taken off the list, and
// marked granted before it is woken, so it runs without taking m->mu or
// checking again. Its list entry lives on its stack: nothing here
// touches it after the wake up.
static void mutex_grant_readers(xo_mutex *m) {
	xo_went *e = m->waiters.first;
	while (e) {
		xo_went *next = e->next;
		if (!e->writer) {
			xo_park *p = e->park;
			wl_del(&m->waiters, e);
			m->readers++;
			m->rwait--;
			__atomic_store_n(&e->granted, 1, __ATOMIC_RELEASE);
			mutex_signal(p);
		}
		e = next;
	}
}

// after_writer: a writer just released m (readers go first).
static void mutex_wake(xo_mutex *m, int after_writer) {
	if (m->writer || !m->waiters.first) {
		return;
	}
	if (m->rwait > 0 && (after_writer || m->wwait == 0)) {
		mutex_grant_readers(m);
		return;
	}
	if (m->wwait > 0 && m->readers == 0) {
		for (xo_went *e = m->waiters.first; e; e = e->next) {
			if (e->writer) {
				mutex_signal(e->park);
				return;
			}
		}
	}
}

static void mutex_free(void *p) {
	xo_mutex *m = p;
	list_release(m->slot, m->drop);
	xo_free(m);
}

xo_mutex *xo_mutex_new(xo_rlist *l, int64_t drop) {
	xo_mutex *m = xo_alloc(sizeof(xo_mutex));
	memset(m, 0, sizeof *m);
	h_init(m, mutex_free);
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
	lk(&m->mu);
	if (m->writer || m->readers > 0) {
		m->wwait++;
		while (m->writer || m->readers > 0) {
			if (!wl_wait_as(c, &m->mu, &m->waiters, 1, 1)) {
				m->wwait--;
				mutex_wake(m, 0); // readers held back by this writer, or the next writer
				unlk(&m->mu);
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
	unlk(&m->mu);
	return l;
}

void xo_mutex_unlock(xo_mutex *m, xo_rlist *l) {
	lk(&m->mu);
	m->slot = l;
	m->writer = 0;
	mutex_wake(m, 1);
	unlk(&m->mu);
	held_del(ctx(), m);
}

// xo_mutex_rlock takes m shared for `with_read` and returns a reference
// to its value.
xo_rlist *xo_mutex_rlock(xo_mutex *m, const char *loc, int64_t n) {
	xo_ctx *c = ctx();
	held_check(c, m, "with_read", loc, n);
	lk(&m->mu);
	if (m->writer || m->wwait > 0) {
		// wait until a writer's release hands m over (mutex_grant_readers);
		// a cancellation unwinds unless m was handed over first
		xo_went e = {.park = &c->park};
		park_reset(&c->park);
		wl_add(&m->waiters, &e);
		m->rwait++;
		unlk(&m->mu);
		for (;;) {
			park_wait(c, 1);
			if (__atomic_load_n(&e.granted, __ATOMIC_ACQUIRE)) {
				// the last writer set m->slot under m->mu before the
				// grant, and none can change it while this share is held
				xo_rlist *l = m->slot;
				rc_inc(&l->rc);
				return l;
			}
			lk(&m->mu);
			if (e.granted) {
				break;
			}
			if (ctx_canceled(c)) {
				wl_del(&m->waiters, &e);
				m->rwait--;
				unlk(&m->mu);
				held_del(c, m);
				c->unwinding = 1;
				return &empty_list;
			}
			park_reset(&c->park);
			unlk(&m->mu);
		}
	} else {
		m->readers++;
	}
	xo_rlist *l = m->slot;
	rc_inc(&l->rc);
	unlk(&m->mu);
	return l;
}

// xo_mutex_runlock gives back a reader's reference and its share of m.
void xo_mutex_runlock(xo_mutex *m, xo_rlist *l) {
	if (rc_dec(&l->rc)) {
		list_release(l, m->drop); // not reached: the mutex holds it
	}
	lk(&m->mu);
	if (--m->readers == 0) {
		mutex_wake(m, 0);
	}
	unlk(&m->mu);
	held_del(ctx(), m);
}

// ---------------------------------------------------------------------------
// Atomic[T] for Int and Bool (a Bool is 0 or 1)

typedef struct {
	xo_hdr h;
	// the value on a cache line apart from the count, which every closure
	// that captures the Atomic retains and releases (decision 0077)
	char pad[128 - sizeof(xo_hdr)];
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
// The event loop (decision 0069): timers, sockets, and signals
//
// One poller thread waits in kqueue (Darwin) or epoll (Linux) for socket
// readiness, the next timer, and its self pipe (wake ups and signals). A
// context that waits for a timer or a socket parks (a task on a fiber
// gives its worker back) and the poller signals its park. These waits do
// not count as blocked for deadlock detection: something outside the
// program's tasks ends them.
//
//   - Timers: a binary heap by deadline (monotonic ns). A timer either
//     signals a park or calls a function on the poller thread with ev.mu
//     held (the function must not take a lock that is held while timers
//     are added or removed).
//   - Sockets: nonblocking, registered once for read and write readiness,
//     edge triggered (EV_CLEAR, EPOLLET); a descriptor (xo_pd) keeps the
//     waiting park of each direction, or that readiness came while nobody
//     waited. A descriptor closes its socket only when no operation runs
//     (so a socket number is never reused under one), and is freed by the
//     poller between two waits (an event it already collected may name
//     it).
//   - Signals: SIGINT and SIGTERM write to the self pipe; the poller runs
//     the two phase shutdown (core 7.5): stop requested (the stopping
//     channel closes, the grace timer starts), then main's root context is
//     canceled (grace over or a second signal), and a third signal exits
//     with status 130.
//
// System calls that set errno go through the x_* wrappers, which are never
// inlined and return -errno: errno is a thread local (see cur_get).

#if defined(__APPLE__) || defined(__FreeBSD__)
#include <sys/event.h>
#define XO_KQUEUE 1
#else
#include <sys/epoll.h>
#endif

typedef struct xo_timer xo_timer;
typedef void (*xo_timer_fn)(xo_timer *);

struct xo_timer {
	int64_t when;   // monotonic ns
	int64_t idx;    // its place in the heap, -1 when not in it
	xo_park *park;  // signaled when it fires, or
	xo_timer_fn fn; // called on the poller thread (ev.mu held)
	void *arg;
	int fired; // atomic
};

typedef struct xo_pd {
	pthread_mutex_t mu;
	int fd;
	int closed; // close was called
	int ops;    // operations running (the socket closes when none runs)
	int rready, wready;
	xo_park *rwait, *wwait;
	struct xo_pd *next; // ev.dead
} xo_pd;

static struct {
	pthread_once_t once;
	pthread_mutex_t mu; // the timer heap and ev.dead
	xo_timer **heap;
	int64_t n, cap;
	int pfd[2]; // the self pipe
	int poll;   // the kqueue or epoll descriptor
	xo_pd *dead;
	int sigs; // signals not handled yet (atomic)
} ev = {.once = PTHREAD_ONCE_INIT, .mu = PTHREAD_MUTEX_INITIALIZER};

__attribute__((noinline)) static int64_t x_ret(int64_t r) {
	if (r < 0) {
		int e = errno;
		if (e == EAGAIN || e == EWOULDBLOCK) {
			STAT(net_again);
		}
		return -(int64_t)e;
	}
	return r;
}

__attribute__((noinline)) static int64_t x_read(int fd, void *b, size_t n) {
	STAT(net_read);
	return x_ret(read(fd, b, n));
}

__attribute__((noinline)) static int64_t x_write(int fd, const void *b, size_t n) {
	STAT(net_write);
#ifdef MSG_NOSIGNAL
	return x_ret(send(fd, b, n, MSG_NOSIGNAL));
#else
	return x_ret(write(fd, b, n));
#endif
}

__attribute__((noinline)) static int64_t x_accept(int fd) { return x_ret(accept(fd, NULL, NULL)); }

__attribute__((noinline)) static int64_t x_connect(int fd, const struct sockaddr *a, socklen_t n) { return x_ret(connect(fd, a, n)); }

__attribute__((noinline)) static int64_t x_sockerr(int fd) {
	int e = 0;
	socklen_t n = sizeof e;
	if (getsockopt(fd, SOL_SOCKET, SO_ERROR, &e, &n) < 0) {
		return -(int64_t)errno;
	}
	return -(int64_t)e;
}

// x_errno is errno now (for calls that report errors in it).
__attribute__((noinline)) static int x_errno(void) { return errno; }

static void set_nonblock(int fd) {
	fcntl(fd, F_SETFL, fcntl(fd, F_GETFL) | O_NONBLOCK);
	fcntl(fd, F_SETFD, FD_CLOEXEC);
}

// ev_poke wakes the poller (a new first timer).
static void ev_poke(void) {
	char b = 'w';
	ssize_t r = write(ev.pfd[1], &b, 1);
	(void)r; // a full pipe wakes it anyway
}

static void *poller_main(void *arg);

static void ev_init(void) {
	if (pipe(ev.pfd) < 0) {
		xo_oom();
	}
	set_nonblock(ev.pfd[0]);
	set_nonblock(ev.pfd[1]);
#ifdef XO_KQUEUE
	ev.poll = kqueue();
	if (ev.poll < 0) {
		xo_oom();
	}
	fcntl(ev.poll, F_SETFD, FD_CLOEXEC);
	struct kevent k;
	EV_SET(&k, ev.pfd[0], EVFILT_READ, EV_ADD, 0, 0, NULL);
	if (kevent(ev.poll, &k, 1, NULL, 0, NULL) < 0) {
		xo_oom();
	}
#else
	ev.poll = epoll_create1(EPOLL_CLOEXEC);
	if (ev.poll < 0) {
		xo_oom();
	}
	struct epoll_event e = {.events = EPOLLIN};
	e.data.ptr = NULL;
	if (epoll_ctl(ev.poll, EPOLL_CTL_ADD, ev.pfd[0], &e) < 0) {
		xo_oom();
	}
#endif
	pthread_attr_t attr;
	pthread_attr_init(&attr);
	pthread_attr_setdetachstate(&attr, PTHREAD_CREATE_DETACHED);
	pthread_t th;
	if (pthread_create(&th, &attr, poller_main, NULL) != 0) {
		xo_oom();
	}
	pthread_attr_destroy(&attr);
}

// ev_start starts the poller thread once.
static void ev_start(void) { pthread_once(&ev.once, ev_init); }

// Timer heap (ev.mu held)

static void th_set(int64_t i, xo_timer *t) {
	ev.heap[i] = t;
	t->idx = i;
}

static void th_up(int64_t i) {
	xo_timer *t = ev.heap[i];
	while (i > 0) {
		int64_t p = (i - 1) / 2;
		if (ev.heap[p]->when <= t->when) {
			break;
		}
		th_set(i, ev.heap[p]);
		i = p;
	}
	th_set(i, t);
}

static void th_down(int64_t i) {
	xo_timer *t = ev.heap[i];
	for (;;) {
		int64_t c = 2 * i + 1;
		if (c >= ev.n) {
			break;
		}
		if (c + 1 < ev.n && ev.heap[c + 1]->when < ev.heap[c]->when) {
			c++;
		}
		if (t->when <= ev.heap[c]->when) {
			break;
		}
		th_set(i, ev.heap[c]);
		i = c;
	}
	th_set(i, t);
}

static void th_remove(xo_timer *t) {
	int64_t i = t->idx;
	xo_timer *last = ev.heap[--ev.n];
	t->idx = -1;
	if (last != t) {
		th_set(i, last);
		th_down(i);
		th_up(last->idx);
	}
}

// timer_add arms t (when, and park or fn, set).
static void timer_add(xo_timer *t) {
	if (!tw.virt) {
		// the test world has no poller: its timers fire when the
		// virtual clock moves (tw_kick)
		ev_start();
	}
	pthread_mutex_lock(&ev.mu);
	if (ev.n == ev.cap) {
		ev.cap = ev.cap ? ev.cap * 2 : 64;
		ev.heap = realloc(ev.heap, (size_t)ev.cap * sizeof(xo_timer *));
		if (!ev.heap) {
			xo_oom();
		}
	}
	ev.heap[ev.n] = t;
	t->idx = ev.n++;
	th_up(t->idx);
	int first = t->idx == 0;
	pthread_mutex_unlock(&ev.mu);
	if (first && !tw.virt) {
		ev_poke();
	}
}

// timer_del disarms t; after it returns t's function does not run.
static void timer_del(xo_timer *t) {
	pthread_mutex_lock(&ev.mu);
	if (t->idx >= 0) {
		th_remove(t);
	}
	pthread_mutex_unlock(&ev.mu);
}

// deadline is the monotonic time d from now (saturating).
static int64_t deadline(int64_t d) {
	int64_t now = tnow(), w;
	if (__builtin_add_overflow(now, d, &w)) {
		w = INT64_MAX;
	}
	return w;
}

// Socket descriptors

static void pd_ready(xo_pd *pd, int mode) {
	STAT(poll_event);
	pthread_mutex_lock(&pd->mu);
	if (mode & 1) {
		if (pd->rwait) {
			STAT(poll_wake);
			park_signal(pd->rwait);
			pd->rwait = NULL;
		} else {
			pd->rready = 1;
		}
	}
	if (mode & 2) {
		if (pd->wwait) {
			STAT(poll_wake);
			park_signal(pd->wwait);
			pd->wwait = NULL;
		} else {
			pd->wready = 1;
		}
	}
	pthread_mutex_unlock(&pd->mu);
}

// pd_new registers socket fd (nonblocking) with the poller; write tells
// to watch write readiness too. NULL when the poller refuses it (errno).
static xo_pd *pd_new(int fd, int write) {
	ev_start();
	xo_pd *pd = calloc(1, sizeof(xo_pd));
	if (!pd) {
		xo_oom();
	}
	mu_init(&pd->mu);
	pd->fd = fd;
#ifdef XO_KQUEUE
	struct kevent k[2];
	EV_SET(&k[0], fd, EVFILT_READ, EV_ADD | EV_CLEAR, 0, 0, pd);
	EV_SET(&k[1], fd, EVFILT_WRITE, EV_ADD | EV_CLEAR, 0, 0, pd);
	int r = kevent(ev.poll, k, write ? 2 : 1, NULL, 0, NULL);
#else
	struct epoll_event e = {.events = EPOLLIN | EPOLLRDHUP | EPOLLET | (write ? EPOLLOUT : 0)};
	e.data.ptr = pd;
	int r = epoll_ctl(ev.poll, EPOLL_CTL_ADD, fd, &e);
#endif
	if (r < 0) {
		pthread_mutex_destroy(&pd->mu);
		free(pd);
		return NULL;
	}
	return pd;
}

// pd_begin starts an operation: 0 when the descriptor is closed.
static int pd_begin(xo_pd *pd) {
	pthread_mutex_lock(&pd->mu);
	int ok = !pd->closed;
	if (ok) {
		pd->ops++;
	}
	pthread_mutex_unlock(&pd->mu);
	return ok;
}

static void pd_end(xo_pd *pd) {
	int fd = -1;
	pthread_mutex_lock(&pd->mu);
	if (--pd->ops == 0 && pd->closed && pd->fd >= 0) {
		fd = pd->fd;
		pd->fd = -1;
	}
	pthread_mutex_unlock(&pd->mu);
	if (fd >= 0) {
		close(fd);
	}
}

// pd_close closes the descriptor: waiting operations return, and the
// socket closes once no operation runs.
static void pd_close(xo_pd *pd) {
	int fd = -1;
	pthread_mutex_lock(&pd->mu);
	if (!pd->closed) {
		pd->closed = 1;
		if (pd->fd >= 0) {
			shutdown(pd->fd, SHUT_RDWR);
		}
		if (pd->rwait) {
			park_signal(pd->rwait);
			pd->rwait = NULL;
		}
		if (pd->wwait) {
			park_signal(pd->wwait);
			pd->wwait = NULL;
		}
	}
	if (pd->ops == 0 && pd->fd >= 0) {
		fd = pd->fd;
		pd->fd = -1;
	}
	pthread_mutex_unlock(&pd->mu);
	if (fd >= 0) {
		close(fd);
	}
}

// pd_free closes the descriptor and hands it to the poller to free.
static void pd_free(xo_pd *pd) {
	if (!pd) {
		return;
	}
	pd_close(pd);
	pthread_mutex_lock(&ev.mu);
	pd->next = ev.dead;
	ev.dead = pd;
	pthread_mutex_unlock(&ev.mu);
}

enum { PD_READY, PD_CANCELED, PD_CLOSED, PD_TIMEOUT };

// pd_wait_until waits until the socket is ready for reading (or
// writing), the context is canceled, the descriptor is closed, or (when
// is not 0) the monotonic time when passes.
static int pd_wait_until(xo_pd *pd, int write, xo_ctx *c, int64_t when) {
	xo_park **slot = write ? &pd->wwait : &pd->rwait;
	int *ready = write ? &pd->wready : &pd->rready;
	xo_timer t = {.when = when, .idx = -1, .park = &c->park};
	int timed = 0;
	int res;
	for (;;) {
		if (ctx_canceled(c)) {
			res = PD_CANCELED;
			break;
		}
		pthread_mutex_lock(&pd->mu);
		if (pd->closed) {
			pthread_mutex_unlock(&pd->mu);
			res = PD_CLOSED;
			break;
		}
		if (*ready) {
			*ready = 0;
			pthread_mutex_unlock(&pd->mu);
			res = PD_READY;
			break;
		}
		park_reset(&c->park);
		*slot = &c->park;
		pthread_mutex_unlock(&pd->mu);
		if (when && !timed) {
			timed = 1;
			timer_add(&t);
		}
		if (!(when && __atomic_load_n(&t.fired, __ATOMIC_ACQUIRE))) {
			park_wait_n(c, 1, 0);
		}
		pthread_mutex_lock(&pd->mu);
		int mine = *slot == &c->park;
		if (mine) {
			*slot = NULL;
		}
		int closed = pd->closed;
		pthread_mutex_unlock(&pd->mu);
		if (closed) {
			res = PD_CLOSED;
			break;
		}
		if (!mine) {
			res = PD_READY; // the poller handed it readiness
			break;
		}
		if (when && __atomic_load_n(&t.fired, __ATOMIC_ACQUIRE)) {
			res = PD_TIMEOUT;
			break;
		}
	}
	if (timed) {
		timer_del(&t);
	}
	return res;
}

// pd_wait is pd_wait_until without a deadline.
static int pd_wait(xo_pd *pd, int write, xo_ctx *c) { return pd_wait_until(pd, write, c, 0); }

// The poller thread

static void stop_signal(void);

#ifdef XO_KQUEUE
typedef struct kevent xo_event;
#else
typedef struct epoll_event xo_event;
#endif

// ev_dispatch hands the readiness of socket event e to its descriptor's
// waiters: 0 when e is the self pipe's.
static int ev_dispatch(xo_event *e) {
#ifdef XO_KQUEUE
	xo_pd *pd = e->udata;
	int mode = e->filter == EVFILT_READ ? 1 : e->filter == EVFILT_WRITE ? 2 : 0;
	if (e->flags & EV_ERROR) {
		mode = 3;
	}
#else
	xo_pd *pd = e->data.ptr;
	uint32_t f = e->events;
	int mode = ((f & (EPOLLIN | EPOLLRDHUP | EPOLLHUP | EPOLLERR)) ? 1 : 0) | ((f & (EPOLLOUT | EPOLLHUP | EPOLLERR)) ? 2 : 0);
#endif
	if (!pd) {
		return 0;
	}
	pd_ready(pd, mode);
	return 1;
}

static void *poller_main(void *arg) {
	(void)arg;
#ifdef XO_KQUEUE
	struct kevent evs[64];
#else
	struct epoll_event evs[64];
#endif
	for (;;) {
		pthread_mutex_lock(&ev.mu);
		while (ev.dead) {
			xo_pd *pd = ev.dead;
			ev.dead = pd->next;
			pthread_mutex_destroy(&pd->mu);
			free(pd);
		}
		int64_t now = mono_ns();
		while (ev.n > 0 && ev.heap[0]->when <= now) {
			xo_timer *t = ev.heap[0];
			th_remove(t);
			__atomic_store_n(&t->fired, 1, __ATOMIC_RELEASE);
			if (t->park) {
				park_signal(t->park);
			} else {
				t->fn(t);
			}
		}
		int64_t wait = ev.n > 0 ? ev.heap[0]->when - now : -1;
		pthread_mutex_unlock(&ev.mu);
#ifdef XO_KQUEUE
		struct timespec ts = {.tv_sec = wait / 1000000000, .tv_nsec = wait % 1000000000};
		int n = kevent(ev.poll, NULL, 0, evs, 64, wait < 0 ? NULL : &ts);
		STAT(poll_return);
#else
		int ms = wait < 0 ? -1 : (int)((wait + 999999) / 1000000);
		if (wait > (int64_t)1 << 40) {
			ms = 1 << 30;
		}
		int n = epoll_wait(ev.poll, evs, 64, ms);
		STAT(poll_return);
#endif
		for (int i = 0; i < n; i++) {
			if (ev_dispatch(&evs[i])) {
				continue;
			}
			char b[64];
			while (read(ev.pfd[0], b, sizeof b) > 0) {
			}
			for (int k = __atomic_exchange_n(&ev.sigs, 0, __ATOMIC_ACQ_REL); k > 0; k--) {
				stop_signal();
			}
		}
	}
	return NULL;
}

// ---------------------------------------------------------------------------
// Clock (std.md 3.3): Time is wall clock ns since the Unix epoch, Duration
// ns (the Go backend's representation)

static void tw_sleep_record(int64_t d);

int64_t xo_clock_now(void) {
	if (tw.virt) {
		return __atomic_load_n(&tw.vnow, __ATOMIC_ACQUIRE);
	}
	struct timespec ts;
	clock_gettime(CLOCK_REALTIME, &ts);
	return (int64_t)ts.tv_sec * 1000000000 + ts.tv_nsec;
}

// clock_wait sleeps d unless the task is or becomes canceled; it returns
// 1 when cancellation ended (or prevented) the sleep, without starting
// an unwind.
static int clock_wait(xo_ctx *c, int64_t d) {
	if (ctx_canceled(c)) {
		return 1;
	}
	if (tw.on) {
		// FakeClock.sleeps() lists every sleep asked for
		tw_sleep_record(d);
	}
	if (d <= 0) {
		return 0;
	}
	xo_timer t = {.when = deadline(d), .idx = -1, .park = &c->park};
	park_reset(&c->park);
	timer_add(&t);
	while (!__atomic_load_n(&t.fired, __ATOMIC_ACQUIRE) && !ctx_canceled(c)) {
		park_wait_n(c, 1, 0);
	}
	timer_del(&t);
	return !__atomic_load_n(&t.fired, __ATOMIC_ACQUIRE) && ctx_canceled(c);
}

// xo_clock_sleep is `clock.sleep(d)`: a cancellation point that unwinds
// (core 7.2), also while it waits.
void xo_clock_sleep(int64_t d) {
	xo_ctx *c = ctx();
	if (clock_wait(c, d)) {
		canceled(c);
	}
}

// xo_clock_sleep_or_cancel is `clock.sleep_or_cancel(d)` (decision 0088):
// 1 when the task was canceled before or during the sleep, which the
// caller returns as ClockErr.Canceled instead of unwinding.
int64_t xo_clock_sleep_or_cancel(int64_t d) {
	return clock_wait(ctx(), d);
}

// xo_select_after adds the `after d` arm to select s: when no other arm
// proceeds within d, xo_select_wait returns -2.
void xo_select_after(xo_select *s, int64_t d) {
	if (!s->has_after || d < s->after) {
		s->after = d;
	}
	s->has_after = 1;
}

static void select_timeout(xo_timer *t) {
	xo_select *s = t->arg;
	if (claim(&s->fired)) {
		__atomic_store_n(&s->chosen, -2, __ATOMIC_RELEASE);
		park_signal(s->park);
	}
}

// task.with_timeout (std.md 7): the scope s runs the closure as its body;
// at the deadline its body is canceled and xo_scope_timed_out tells it.

static void scope_deadline_fire(xo_timer *t) {
	xo_scope *s = t->arg;
	__atomic_store_n(&s->timed_out, 1, __ATOMIC_RELEASE);
	node_cancel(&s->body);
}

void xo_scope_deadline(xo_scope *s, int64_t d) {
	xo_timer *t = calloc(1, sizeof(xo_timer));
	if (!t) {
		xo_oom();
	}
	t->when = deadline(d);
	t->idx = -1;
	t->fn = scope_deadline_fire;
	t->arg = s;
	s->deadline = t;
	timer_add(t);
}

// xo_scope_timed_out reports, after scope s closed while its body
// unwound, whether the unwinding came from its deadline (and not from a
// cancellation of the code around it): the unwinding then stops.
int64_t xo_scope_timed_out(xo_scope *s) {
	xo_ctx *c = ctx();
	if (!__atomic_load_n(&s->timed_out, __ATOMIC_ACQUIRE) || ctx_canceled(c)) {
		return 0;
	}
	c->unwinding = 0;
	return 1;
}

// ---------------------------------------------------------------------------
// Network (std.md 3.2): TCP listeners and connections. Each operation
// records its outcome in the context (net_status, net_msg), which the
// generated code reads to build the NetErr: 1 InvalidAddr, 2 Refused,
// 3 Timeout, 4 Io(msg), 5 Canceled (the Go backend's tags).

typedef struct {
	xo_hdr h;
	xo_pd *pd;
	char *addr;
	void *mem; // the test world's in memory listener (xo_testing.c), or NULL
} xo_listener;

typedef struct {
	xo_hdr h;
	xo_pd *pd;
} xo_conn;

static void net_ok(xo_ctx *c) { c->net_status = 0; }

static void net_fail(xo_ctx *c, int st, const char *fmt, ...) {
	char buf[512];
	va_list ap;
	va_start(ap, fmt);
	vsnprintf(buf, sizeof buf, fmt, ap);
	va_end(ap);
	free(c->net_msg);
	c->net_msg = strdup(buf);
	c->net_status = st;
}

int64_t xo_net_status(void) { return ctx()->net_status; }

// str_list is a new List[Str] of one element: the n bytes at p.
static xo_rlist *str_list(const char *p, int64_t n) {
	xo_rstr *s = rs_new(n);
	memcpy(s->data, p, (size_t)n);
	xo_rlist *l = xo_rl_new(1, 16);
	l->len = 1;
	((xo_rstr **)rl_data(l))[0] = s;
	((int64_t *)rl_data(l))[1] = n;
	return l;
}

// xo_net_msg is the message of the last Io error, in a list of one.
xo_rlist *xo_net_msg(void) {
	xo_ctx *c = ctx();
	const char *m = c->net_msg ? c->net_msg : "";
	return str_list(m, (int64_t)strlen(m));
}

static void conn_free(void *p) {
	xo_conn *cn = p;
	pd_free(cn->pd);
	xo_free(cn);
}

static void tw_listener_free(xo_listener *l);
static xo_listener *tw_listen(xo_ctx *c, const char *addr);
static void tw_listener_close(xo_listener *l);
static xo_conn *tw_listener_accept(xo_ctx *c, xo_listener *l);

static void listener_free(void *p) {
	xo_listener *l = p;
	if (l->mem) {
		tw_listener_free(l);
	} else {
		pd_free(l->pd);
	}
	free(l->addr);
	xo_free(l);
}

// split_hostport splits "host:port", "[v6]:port", or ":port".
static int split_hostport(const char *a, char *host, size_t hn, char *port, size_t pn) {
	const char *colon = strrchr(a, ':');
	if (!colon) {
		return -1;
	}
	const char *h = a;
	size_t hl = (size_t)(colon - a);
	if (hl >= 2 && h[0] == '[' && h[hl - 1] == ']') {
		h++;
		hl -= 2;
	}
	if (hl >= hn || strlen(colon + 1) >= pn) {
		return -1;
	}
	memcpy(host, h, hl);
	host[hl] = 0;
	strcpy(port, colon + 1);
	return 0;
}

// port_out_of_range reports a numeric port above 65535. Darwin's
// getaddrinfo rejects one; glibc's truncates it to 16 bits and binds
// another port (gotchas/glibc-getaddrinfo-port-truncation.md).
static int port_out_of_range(const char *port) {
	long v = 0;
	if (!port[0]) {
		return 0;
	}
	for (const char *q = port; *q; q++) {
		if (*q < '0' || *q > '9') {
			return 0;
		}
		v = v * 10 + (*q - '0');
		if (v > 65535) {
			return 1;
		}
	}
	return 0;
}

// fmt_addr formats a socket address as "ip:port" or "[ip]:port"; an
// unspecified address becomes the loopback address (std.md 3.2).
static char *fmt_addr(const struct sockaddr *sa) {
	char ip[INET6_ADDRSTRLEN];
	char *out = malloc(INET6_ADDRSTRLEN + 16);
	if (!out) {
		xo_oom();
	}
	if (sa->sa_family == AF_INET) {
		const struct sockaddr_in *a = (const struct sockaddr_in *)sa;
		if (a->sin_addr.s_addr == htonl(INADDR_ANY)) {
			strcpy(ip, "127.0.0.1");
		} else {
			inet_ntop(AF_INET, &a->sin_addr, ip, sizeof ip);
		}
		sprintf(out, "%s:%d", ip, ntohs(a->sin_port));
	} else {
		const struct sockaddr_in6 *a = (const struct sockaddr_in6 *)sa;
		if (IN6_IS_ADDR_UNSPECIFIED(&a->sin6_addr)) {
			strcpy(ip, "::1");
		} else {
			inet_ntop(AF_INET6, &a->sin6_addr, ip, sizeof ip);
		}
		sprintf(out, "[%s]:%d", ip, ntohs(a->sin6_port));
	}
	return out;
}

static const char *err_text(int e) {
	switch (e) {
	case ECONNREFUSED:
		return "connection refused";
	case ECONNRESET:
		return "connection reset by peer";
	case EPIPE:
		return "broken pipe";
	case EADDRINUSE:
		return "address already in use";
	case EADDRNOTAVAIL:
		return "can't assign requested address";
	case EACCES:
		return "permission denied";
	case ETIMEDOUT:
		return "operation timed out";
	}
	return strerror(e);
}

static const char closed_text[] = "use of closed network connection";

// xo_net_listen is `net.listen(addr)` (std.md 3.2).
xo_listener *xo_net_listen(const char *p, int64_t n) {
	xo_ctx *c = ctx();
	if (ctx_canceled(c)) {
		net_fail(c, 5, "");
		return xo_h_zero();
	}
	char *addr = cstr_of(p, n);
	if (tw_is_fake(TW_REAL_NET)) {
		// FakeNet.listen: an in memory listener (std/rt's memListener)
		xo_listener *ml = tw_listen(c, addr);
		free(addr);
		return ml;
	}
	char host[256], port[32];
	if (split_hostport(addr, host, sizeof host, port, sizeof port) < 0) {
		net_fail(c, 4, "listen tcp %s: address %s: missing port in address", addr, addr);
		free(addr);
		return xo_h_zero();
	}
	if (port_out_of_range(port)) {
		net_fail(c, 4, "listen tcp %s: address %s: invalid port", addr, port);
		free(addr);
		return xo_h_zero();
	}
	struct addrinfo hints = {.ai_socktype = SOCK_STREAM, .ai_flags = AI_PASSIVE | AI_NUMERICSERV};
	hints.ai_family = host[0] ? AF_UNSPEC : AF_INET6;
	struct addrinfo *res = NULL;
	int g = getaddrinfo(host[0] ? host : NULL, port, &hints, &res);
	if (g != 0) {
		net_fail(c, 4, "listen tcp %s: %s", addr, gai_strerror(g));
		free(addr);
		return xo_h_zero();
	}
	int fd = -1, e = 0;
	for (struct addrinfo *a = res; a; a = a->ai_next) {
		fd = socket(a->ai_family, a->ai_socktype, a->ai_protocol);
		if (fd < 0) {
			e = x_errno();
			continue;
		}
		int one = 1, zero = 0;
		setsockopt(fd, SOL_SOCKET, SO_REUSEADDR, &one, sizeof one);
		if (a->ai_family == AF_INET6 && !host[0]) {
			setsockopt(fd, IPPROTO_IPV6, IPV6_V6ONLY, &zero, sizeof zero);
		}
		if (bind(fd, a->ai_addr, a->ai_addrlen) == 0 && listen(fd, SOMAXCONN) == 0) {
			break;
		}
		e = x_errno();
		close(fd);
		fd = -1;
	}
	freeaddrinfo(res);
	if (fd < 0) {
		net_fail(c, 4, "listen tcp %s: bind: %s", addr, err_text(e));
		free(addr);
		return xo_h_zero();
	}
	free(addr);
	set_nonblock(fd);
	xo_pd *pd = pd_new(fd, 0);
	if (!pd) {
		e = x_errno();
		close(fd);
		net_fail(c, 4, "listen tcp: %s", err_text(e));
		return xo_h_zero();
	}
	struct sockaddr_storage ss;
	socklen_t sl = sizeof ss;
	getsockname(fd, (struct sockaddr *)&ss, &sl);
	xo_listener *l = xo_alloc(sizeof(xo_listener));
	h_init(l, listener_free);
	l->pd = pd;
	l->mem = NULL;
	l->addr = fmt_addr((struct sockaddr *)&ss);
	net_ok(c);
	return l;
}

// xo_listener_addr is `l.addr()`, in a list of one.
xo_rlist *xo_listener_addr(xo_listener *l) { return str_list(l->addr, (int64_t)strlen(l->addr)); }

void xo_listener_close(xo_listener *l) {
	if (l->mem) {
		tw_listener_close(l);
		return;
	}
	pd_close(l->pd);
}

// conn_new makes a connection handle of a connected socket.
static xo_conn *conn_new(xo_ctx *c, int fd, xo_pd *pd) {
	int one = 1;
	setsockopt(fd, IPPROTO_TCP, TCP_NODELAY, &one, sizeof one);
#ifdef SO_NOSIGPIPE
	setsockopt(fd, SOL_SOCKET, SO_NOSIGPIPE, &one, sizeof one);
#endif
	if (!pd) {
		pd = pd_new(fd, 1);
		if (!pd) {
			int e = x_errno();
			close(fd);
			net_fail(c, 4, "%s", err_text(e));
			return xo_h_zero();
		}
	}
	xo_conn *cn = xo_alloc(sizeof(xo_conn));
	h_init(cn, conn_free);
	cn->pd = pd;
	net_ok(c);
	return cn;
}

// xo_listener_accept is `l.accept()`: waits for a connection (canceled:
// NetErr.Canceled).
xo_conn *xo_listener_accept(xo_listener *l) {
	xo_ctx *c = ctx();
	if (ctx_canceled(c)) {
		net_fail(c, 5, "");
		return xo_h_zero();
	}
	if (l->mem) {
		return tw_listener_accept(c, l);
	}
	xo_pd *pd = l->pd;
	if (!pd_begin(pd)) {
		net_fail(c, 4, "accept tcp %s: %s", l->addr, closed_text);
		return xo_h_zero();
	}
	for (;;) {
		int64_t fd = x_accept(pd->fd);
		if (fd >= 0) {
			pd_end(pd);
			set_nonblock((int)fd);
			return conn_new(c, (int)fd, NULL);
		}
		if (fd == -EINTR || fd == -ECONNABORTED) {
			continue;
		}
		if (fd == -EAGAIN || fd == -EWOULDBLOCK) {
			int w = pd_wait(pd, 0, c);
			if (w == PD_READY) {
				continue;
			}
			pd_end(pd);
			if (w == PD_CANCELED) {
				net_fail(c, 5, "");
			} else {
				net_fail(c, 4, "accept tcp %s: %s", l->addr, closed_text);
			}
			return xo_h_zero();
		}
		pd_end(pd);
		net_fail(c, 4, "accept tcp %s: %s", l->addr, err_text((int)-fd));
		return xo_h_zero();
	}
}

// xo_net_dial is `net.dial(addr)` (std.md 3.2): connection refused is
// NetErr.Refused; it gives up after 30 s like the Go backend's
// net.DialTimeout (NetErr.Io "i/o timeout").
xo_conn *xo_net_dial_timeout(const char *p, int64_t n, int64_t d);

xo_conn *xo_net_dial(const char *p, int64_t n) { return xo_net_dial_timeout(p, n, (int64_t)30 * 1000000000); }

// xo_net_dial_timeout is xo_net_dial giving up after d ns (d <= 0: no
// limit).
xo_conn *xo_net_dial_timeout(const char *p, int64_t n, int64_t d) {
	int64_t when = d > 0 ? deadline(d) : 0;
	xo_ctx *c = ctx();
	if (ctx_canceled(c)) {
		net_fail(c, 5, "");
		return xo_h_zero();
	}
	if (tw_is_fake(TW_REAL_NET)) {
		// FakeNet.dial: raw connections are not faked (Refused)
		net_fail(c, 2, "");
		return xo_h_zero();
	}
	char *addr = cstr_of(p, n);
	char host[256], port[32];
	if (split_hostport(addr, host, sizeof host, port, sizeof port) < 0) {
		net_fail(c, 4, "dial tcp %s: address %s: missing port in address", addr, addr);
		free(addr);
		return xo_h_zero();
	}
	if (port_out_of_range(port)) {
		net_fail(c, 4, "dial tcp %s: address %s: invalid port", addr, port);
		free(addr);
		return xo_h_zero();
	}
	struct addrinfo hints = {.ai_family = AF_UNSPEC, .ai_socktype = SOCK_STREAM, .ai_flags = AI_NUMERICSERV};
	struct addrinfo *res = NULL;
	int g = getaddrinfo(host[0] ? host : "localhost", port, &hints, &res);
	if (g != 0) {
		net_fail(c, 4, "dial tcp %s: %s", addr, gai_strerror(g));
		free(addr);
		return xo_h_zero();
	}
	int64_t e = 0;
	xo_conn *out = NULL;
	for (struct addrinfo *a = res; a && !out; a = a->ai_next) {
		int fd = socket(a->ai_family, a->ai_socktype, a->ai_protocol);
		if (fd < 0) {
			e = -x_errno();
			continue;
		}
		set_nonblock(fd);
		xo_pd *pd = pd_new(fd, 1);
		if (!pd) {
			e = -x_errno();
			close(fd);
			continue;
		}
		pd_begin(pd);
		e = x_connect(fd, a->ai_addr, a->ai_addrlen);
		while (e == -EINTR) {
			e = x_connect(fd, a->ai_addr, a->ai_addrlen);
		}
		if (e == -EINPROGRESS) {
			int w = pd_wait_until(pd, 1, c, when);
			e = w == PD_READY ? x_sockerr(fd) : w == PD_CANCELED ? 1 : w == PD_TIMEOUT ? 2 : -ECONNABORTED;
		}
		pd_end(pd);
		if (e == 0) {
			out = conn_new(c, fd, pd);
			break;
		}
		pd_free(pd);
		if (e == 1 || e == 2) {
			break; // canceled, or the time is up
		}
	}
	freeaddrinfo(res);
	if (out) {
		free(addr);
		return out;
	}
	if (e == 1) {
		net_fail(c, 5, "");
	} else if (e == 2) {
		net_fail(c, 4, "dial tcp %s: i/o timeout", addr);
	} else if (e == -ECONNREFUSED) {
		net_fail(c, 2, "");
	} else {
		net_fail(c, 4, "dial tcp %s: connect: %s", addr, err_text((int)-e));
	}
	free(addr);
	return xo_h_zero();
}

// xo_conn_read is `c.read(max)`: at most max bytes (at most 1 MiB) in a
// list of one, or the empty list at the end of the stream (None).
xo_rlist *xo_conn_read(xo_conn *cn, int64_t max) {
	xo_ctx *c = ctx();
	if (ctx_canceled(c)) {
		net_fail(c, 5, "");
		return &empty_list;
	}
	if (max <= 0) {
		net_ok(c);
		return str_list("", 0);
	}
	if (max > (1 << 20)) {
		max = 1 << 20;
	}
	xo_pd *pd = cn->pd;
	if (!pd_begin(pd)) {
		net_fail(c, 4, "read tcp: %s", closed_text);
		return &empty_list;
	}
	xo_rstr *s = rs_new(max);
	for (;;) {
		int64_t r = x_read(pd->fd, s->data, (size_t)max);
		if (r > 0) {
			pd_end(pd);
			xo_rlist *l = xo_rl_new(1, 16);
			l->len = 1;
			((xo_rstr **)rl_data(l))[0] = s;
			((int64_t *)rl_data(l))[1] = r;
			net_ok(c);
			return l;
		}
		if (r == 0) {
			pd_end(pd);
			xo_free(s);
			net_ok(c);
			return &empty_list;
		}
		if (r == -EINTR) {
			continue;
		}
		if (r == -EAGAIN || r == -EWOULDBLOCK) {
			int w = pd_wait(pd, 0, c);
			if (w == PD_READY) {
				continue;
			}
			pd_end(pd);
			xo_free(s);
			if (w == PD_CANCELED) {
				net_fail(c, 5, "");
			} else {
				net_fail(c, 4, "read tcp: %s", closed_text);
			}
			return &empty_list;
		}
		pd_end(pd);
		xo_free(s);
		net_fail(c, 4, "read tcp: %s", err_text((int)-r));
		return &empty_list;
	}
}

// xo_conn_write is `c.write(data)`: writes every byte.
void xo_conn_write(xo_conn *cn, const char *p, int64_t n) {
	xo_ctx *c = ctx();
	if (ctx_canceled(c)) {
		net_fail(c, 5, "");
		return;
	}
	xo_pd *pd = cn->pd;
	if (!pd_begin(pd)) {
		net_fail(c, 4, "write tcp: %s", closed_text);
		return;
	}
	int64_t off = 0;
	while (off < n) {
		int64_t w = x_write(pd->fd, p + off, (size_t)(n - off));
		if (w >= 0) {
			off += w;
			continue;
		}
		if (w == -EINTR) {
			continue;
		}
		if (w == -EAGAIN || w == -EWOULDBLOCK) {
			int r = pd_wait(pd, 1, c);
			if (r == PD_READY) {
				continue;
			}
			pd_end(pd);
			if (r == PD_CANCELED) {
				net_fail(c, 5, "");
			} else {
				net_fail(c, 4, "write tcp: %s", closed_text);
			}
			return;
		}
		pd_end(pd);
		net_fail(c, 4, "write tcp: %s", err_text((int)-w));
		return;
	}
	pd_end(pd);
	net_ok(c);
}

void xo_conn_close(xo_conn *cn) { pd_close(cn->pd); }

// xo_utf8_valid reports whether the n bytes at p are valid UTF-8 (as Go's
// utf8.Valid: no surrogates, no overlong forms, at most U+10FFFF), for
// `Bytes.to_str()`.
int64_t xo_utf8_valid(const char *p, int64_t n) {
	const unsigned char *s = (const unsigned char *)p;
	int64_t i = 0;
	while (i < n) {
		unsigned c = s[i];
		if (c < 0x80) {
			i++;
			continue;
		}
		int k;
		unsigned lo = 0x80, hi = 0xBF;
		if (c >= 0xC2 && c <= 0xDF) {
			k = 1;
		} else if (c >= 0xE0 && c <= 0xEF) {
			k = 2;
			if (c == 0xE0) {
				lo = 0xA0;
			} else if (c == 0xED) {
				hi = 0x9F;
			}
		} else if (c >= 0xF0 && c <= 0xF4) {
			k = 3;
			if (c == 0xF0) {
				lo = 0x90;
			} else if (c == 0xF4) {
				hi = 0x8F;
			}
		} else {
			return 0;
		}
		if (i + k >= n) {
			return 0; // cut short
		}
		if (s[i + 1] < lo || s[i + 1] > hi) {
			return 0;
		}
		for (int j = 2; j <= k; j++) {
			if (s[i + j] < 0x80 || s[i + j] > 0xBF) {
				return 0;
			}
		}
		i += k + 1;
	}
	return 1;
}

// xo_str_hash hashes the n bytes at p (FNV-1a, then a final mix): Map
// keys and Set elements (decision 0072).
int64_t xo_str_hash(const char *p, int64_t n) {
	uint64_t h = 1469598103934665603ULL;
	for (int64_t i = 0; i < n; i++) {
		h ^= (unsigned char)p[i];
		h *= 1099511628211ULL;
	}
	h ^= h >> 31;
	h *= 0x9E3779B97F4A7C15ULL;
	h ^= h >> 29;
	return (int64_t)h;
}

// xo_str_quote is the text of a Str in double quotes with Go's escapes
// for quotes, backslashes, and control bytes (the Go backend's Debug of a
// Str in fault messages), in a list of one.
xo_rlist *xo_str_quote(const char *p, int64_t n) {
	char *b = malloc((size_t)n * 4 + 3);
	if (!b) {
		xo_oom();
	}
	int64_t o = 0;
	b[o++] = '"';
	for (int64_t i = 0; i < n; i++) {
		unsigned char c = (unsigned char)p[i];
		switch (c) {
		case '"':
			b[o++] = '\\';
			b[o++] = '"';
			break;
		case '\\':
			b[o++] = '\\';
			b[o++] = '\\';
			break;
		case '\n':
			b[o++] = '\\';
			b[o++] = 'n';
			break;
		case '\t':
			b[o++] = '\\';
			b[o++] = 't';
			break;
		case '\r':
			b[o++] = '\\';
			b[o++] = 'r';
			break;
		default:
			if (c < 0x20 || c == 0x7f) {
				o += sprintf(b + o, "\\x%02x", c);
			} else {
				b[o++] = (char)c;
			}
		}
	}
	b[o++] = '"';
	xo_rlist *l = str_list(b, o);
	free(b);
	return l;
}

// xo_floor_div is a / b rounded down (b > 0): `Time.unix()`.
int64_t xo_floor_div(int64_t a, int64_t b) {
	int64_t q = a / b;
	if (a % b != 0 && ((a < 0) != (b < 0))) {
		q--;
	}
	return q;
}

// ---------------------------------------------------------------------------
// Environment (std.md 3.5)

// xo_env_get is `env.get(name)`: the value in a list of one, or the empty
// list when it is not set.
static xo_rlist *tw_env_get(const char *p, int64_t n);

xo_rlist *xo_env_get(const char *p, int64_t n) {
	if (tw_is_fake(TW_REAL_ENV)) {
		return tw_env_get(p, n);
	}
	char *name = cstr_of(p, n);
	const char *v = getenv(name);
	free(name);
	if (!v) {
		return &empty_list;
	}
	return str_list(v, (int64_t)strlen(v));
}

// ---------------------------------------------------------------------------
// Signals and two phase shutdown (core 7.5, decision 0042)

#define XO_DEFAULT_GRACE ((int64_t)30 * 1000000000)

static struct {
	pthread_mutex_t mu;
	int signals;
	int requested; // atomic
	int canceled;  // atomic
	int exited;    // a third signal in the test world: run_main gives 130
	int64_t stop_at, grace;
	// the test world: set_grace before testing.run_main sets the grace of
	// the next run (std/rt's FakeProc.set_grace)
	int has_next_grace;
	int64_t next_grace;
	xo_timer grace_t;
	xo_chan *stopping; // task.stopping(), made on first use
	xo_node *root;     // main's root context
} stop = {.mu = PTHREAD_MUTEX_INITIALIZER, .grace = XO_DEFAULT_GRACE, .grace_t = {.idx = -1}};

// stop_cancel enters phase 2: main's root context is canceled.
static void stop_cancel(void) {
	int z = 0;
	if (stop.root && __atomic_compare_exchange_n(&stop.canceled, &z, 1, 0, __ATOMIC_ACQ_REL, __ATOMIC_ACQUIRE)) {
		node_cancel(stop.root);
	}
}

static void grace_fire(xo_timer *t) {
	(void)t;
	stop_cancel();
}

// arm_grace (stop.mu held) arms the grace timer for the time left.
static void arm_grace(void) {
	timer_del(&stop.grace_t);
	if (__atomic_load_n(&stop.canceled, __ATOMIC_ACQUIRE)) {
		return;
	}
	int64_t left = stop.grace - (tnow() - stop.stop_at);
	if (left <= 0) {
		stop_cancel();
		return;
	}
	stop.grace_t.when = deadline(left);
	stop.grace_t.fn = grace_fire;
	stop.grace_t.park = NULL;
	timer_add(&stop.grace_t);
}

// stop_signal handles SIGINT or SIGTERM (on the poller thread).
static void stop_signal(void) {
	pthread_mutex_lock(&stop.mu);
	stop.signals++;
	switch (stop.signals) {
	case 1:
		stop.stop_at = tnow();
		__atomic_store_n(&stop.requested, 1, __ATOMIC_RELEASE);
		if (stop.stopping) {
			xo_chan_close(stop.stopping);
		}
		arm_grace();
		break;
	case 2:
		stop_cancel();
		break;
	default:
		if (tw_is_fake(TW_REAL_PROC)) {
			// a test's run_main returns 130 once main has unwound
			// (std/rt's stopState.exited), FakeProc.exit_code is Some(130)
			stop.exited = 1;
			stop_cancel();
			break;
		}
		pthread_mutex_unlock(&stop.mu);
		stop_cancel();
		xo_flush();
		_exit(130);
	}
	pthread_mutex_unlock(&stop.mu);
}

static void on_signal(int sig) {
	(void)sig;
	int e = errno;
	__atomic_fetch_add(&ev.sigs, 1, __ATOMIC_ACQ_REL);
	char b = 's';
	ssize_t r = write(ev.pfd[1], &b, 1);
	(void)r;
	errno = e;
}

// xo_signals_init installs the handlers (main, before the program runs).
__attribute__((unused)) static void xo_signals_init(void) {
	ev_start();
	struct sigaction sa;
	memset(&sa, 0, sizeof sa);
	sa.sa_handler = on_signal;
	sa.sa_flags = SA_RESTART;
	sigemptyset(&sa.sa_mask);
	sigaction(SIGINT, &sa, NULL);
	sigaction(SIGTERM, &sa, NULL);
	signal(SIGPIPE, SIG_IGN);
}

int64_t xo_stop_requested(void) { return __atomic_load_n(&stop.requested, __ATOMIC_ACQUIRE); }

// xo_task_stopping is `task.stopping()`: a channel closed when a stop is
// requested (drop releases values someone sends on it).
xo_chan *xo_task_stopping(int64_t drop) {
	pthread_mutex_lock(&stop.mu);
	if (!stop.stopping) {
		static const char loc[] = "task.stopping";
		stop.stopping = xo_chan_new(0, drop, loc, (int64_t)sizeof loc - 1);
		stop.stopping->ext = 1;
		if (__atomic_load_n(&stop.requested, __ATOMIC_ACQUIRE)) {
			xo_chan_close(stop.stopping);
		}
	}
	xo_chan *ch = stop.stopping;
	h_retain(ch);
	pthread_mutex_unlock(&stop.mu);
	return ch;
}

// xo_set_grace is `proc.set_grace(d)`: after a stop request the deadline
// moves to the stop time plus d.
void xo_set_grace(int64_t d) {
	pthread_mutex_lock(&stop.mu);
	if (tw_is_fake(TW_REAL_PROC) && !__atomic_load_n(&tw.in_main, __ATOMIC_ACQUIRE)) {
		// before testing.run_main: the grace period of the next run
		stop.has_next_grace = 1;
		stop.next_grace = d < 0 ? 0 : d;
		pthread_mutex_unlock(&stop.mu);
		return;
	}
	stop.grace = d < 0 ? 0 : d;
	if (__atomic_load_n(&stop.requested, __ATOMIC_ACQUIRE)) {
		arm_grace();
	}
	pthread_mutex_unlock(&stop.mu);
}

// select's `after` timer (a timer on the heap of the waiting call)
static void select_timer(xo_select *s, struct xo_timer **tp) {
	xo_timer *t = calloc(1, sizeof(xo_timer));
	if (!t) {
		xo_oom();
	}
	t->when = deadline(s->after);
	t->idx = -1;
	t->fn = select_timeout;
	t->arg = s;
	timer_add(t);
	*tp = t;
}

static void select_untimer(struct xo_timer *t) {
	timer_del(t);
	free(t);
}

// ---------------------------------------------------------------------------
// Entry (left out with -DXO_RT_NO_MAIN: the C tests of std/llrt/testdata
// include this file and bring their own main)

// the program's arguments, for env.args (std.md 3.5)
static int xo_argc;
static char **xo_argv;

// constants of collection types (xo_bytes.c)
void xo_consts_free(void);

#ifndef XO_RT_NO_MAIN
extern void xo_entry(void);

static void *run_main(void *arg) {
	(void)arg;
	// main's root context: the second shutdown signal cancels it
	xo_node *none = NULL;
	__atomic_compare_exchange_n(&stop.root, &none, ctx()->node, 0, __ATOMIC_ACQ_REL, __ATOMIC_ACQUIRE);
	// a fault that unwinds main reaches this frame: reported, status 70
	// (decision 0085)
	ctx()->boundary = (uintptr_t)__builtin_frame_address(0);
	xo_entry();
	return NULL;
}

static void tw_init(void);
static void tw_cleanup(void);
static void xo_streams_free(void);

int main(int argc, char **argv) {
	xo_argc = argc;
	xo_argv = argv;
	if (getenv("XO_TEST")) {
		// a test binary runs one test in the test world (xo_testing.c)
		tw_init();
	} else {
		xo_signals_init();
	}
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
	pthread_mutex_lock(&stop.mu);
	if (stop.stopping) {
		h_release(stop.stopping);
		stop.stopping = NULL;
	}
	pthread_mutex_unlock(&stop.mu);
	xo_rt_stats();
	xo_streams_free();
	if (tw.on) {
		tw_cleanup();
	}
	if (getenv("XO_LEAKCHECK")) {
		xo_consts_free();
		int64_t live, total;
		xo_rt_live(&live, &total);
		fprintf(stderr, "xo leakcheck: %lld live of %lld allocations\n", (long long)live, (long long)total);
	}
	return 0;
}
#endif

// The intrinsics of the native std implementation (decision 0074).
#include "xo_std.c"
