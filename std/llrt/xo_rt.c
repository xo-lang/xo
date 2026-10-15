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

#ifdef _WIN32
// Windows (decision 0197): the POSIX subset on Win32 and Winsock
#include "xo_win.c"
#else
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
#include <sys/un.h>
#include <netinet/tcp.h>
#include <sys/mman.h>
#include <sys/socket.h>
#endif
#include <stddef.h>
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
// the largest block the size classes serve (the rest is malloc's): at
// most XO_MID_MAX, lowered by XO_HEAP_MID_MAX (an experiment of the xo1
// speed work: blocks on a class's free list are never reused for another
// class nor returned, so growing lists leave their old blocks behind)
static int64_t xo_mid_max = XO_MID_MAX;
#define XO_NCLASS (64 + 8) // 16..1024 by 16, then 2 KiB..256 KiB

typedef void (*xo_drop_fn)(void *);

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
	// The words std/native/llrt/alloc.xo reads and writes, at the offsets
	// of its constants (checked below): with the heap in Xo (decision
	// 0207 stage 3) the C finds a thread's heap and hands its address
	// to the cabi.xo exports.
	//
	// blocks allocated, and blocks freed, by this heap's thread (the
	// leak check reports the difference): one counter per call
	int64_t allocs;
	int64_t frees;
	// bytes allocated by this heap's thread (resource budgets, decision
	// 0175: a context charges what its thread's count grew by while it
	// ran)
	int64_t abytes;
	char *bump_cur;
	char *bump_end;
	// real-time sections (decision 0133): calls of real-time functions
	// entered and not left on this thread, allocations since the
	// outermost one began
	int64_t rt_depth;
	int64_t rt_allocs;
	// xo_mid_max, and the central lists (every heap has the same)
	int64_t mid_max;
	void **central;
	// the usable bytes of the block alloc.xo returned last (rs_new reads
	// it for a string's capacity)
	int64_t last_size;
	void *free_lists[XO_NCLASS + 1];
	int64_t nfree[XO_NCLASS + 1]; // blocks on each free list (about: a magazine taken counts as full)
	void *mag[XO_NCLASS + 1];     // a full magazine per class (threaded: free_mt)
	// the worklist of xo_rc_defer (std/native/llrt/obj.xo rc_defer):
	// pairs of words {object, drop}, from malloc
	void *defer_q;
	int64_t defer_n, defer_cap;
	int64_t defer_busy;
	// xo_rl_free's element size and drop, from obj.xo rl_free_n_slow to
	// rl_free_rt (a real-time section; rt.call_c2 passes two words)
	int64_t pend_esize, pend_drop;
	// the thread's scratch for the structures of system calls (timespec,
	// statx, sigaction; obj.xo scratch, work item B02)
	char scratch[256];
	struct xo_heap *next; // all heaps, for the leak check (heapos.xo's list)
	// the frees deferred by real-time sections (a fixed queue, so
	// deferring never allocates)
	int64_t rt_n;
	xo_rtdef rt_q[XO_RTQ];
} xo_heap;

#include <stddef.h>
_Static_assert(offsetof(xo_heap, rt_depth) == 40 && offsetof(xo_heap, mid_max) == 56 && offsetof(xo_heap, central) == 64 && offsetof(xo_heap, last_size) == 72, "alloc.xo's heap record offsets");
_Static_assert(offsetof(xo_heap, free_lists) == 80 && offsetof(xo_heap, nfree) == 664 && offsetof(xo_heap, mag) == 1248, "alloc.xo's heap record offsets");
_Static_assert(offsetof(xo_heap, defer_q) == 1832 && offsetof(xo_heap, defer_n) == 1840 && offsetof(xo_heap, defer_cap) == 1848 && offsetof(xo_heap, defer_busy) == 1856, "obj.xo's heap record offsets");
_Static_assert(offsetof(xo_heap, pend_esize) == 1864 && offsetof(xo_heap, pend_drop) == 1872 && offsetof(xo_heap, scratch) == 1880, "obj.xo's heap record offsets");
_Static_assert(offsetof(xo_heap, next) == 2136, "heapos.xo's heap record offsets");

// The central lists: per class, a stack of magazines (free lists of
// about central_batch(c) blocks) linked by their first block's second
// word (every block has at least 16 bytes).
static void *central[XO_NCLASS + 1];

static xo_heap main_heap = {.mid_max = XO_MID_MAX, .central = central};
static int xo_threaded; // set once, before a second thread runs Xo code
// solo_heap is main_heap while one thread runs, null after (one load
// both tells the case and gives the heap)
static xo_heap *solo_heap = &main_heap;
// the calling thread's heap is word 3 of the thread local table the
// compiler emits (decision 0212; std/native/llrt/sched.xo TLS_HEAP)
extern __thread int64_t xo_tlocal[16];
#define tls_heap (*(xo_heap **)&xo_tlocal[3])

XO_NORETURN static void xo_oom(void) {
	fputs("fault: out of memory\n", stderr);
	exit(70);
}

// heap_slow is the calling thread's heap: its thread local, else a new
// record that std/native/llrt/heapos.xo maps and links into the list of
// every record (work item B21 part 2).
int64_t xo_x_heap_thread(void);
__attribute__((noinline)) static xo_heap *heap_slow(void) {
	xo_heap *h = tls_heap;
	if (h) {
		return h;
	}
	return (xo_heap *)(intptr_t)xo_x_heap_thread();
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
void xo_x_stdio_threads(void);
void xo_x_obj_heap_init(int64_t h, int64_t slow, int64_t rl_free_rt, int64_t rc_defer_rt, int64_t libc, int64_t hsize);
void xo_x_heap_live(int64_t lp, int64_t tp);
int64_t xo_x_heap_nfree(int64_t c);

// heap_libc tells heapos.xo to map the heap's memory through the C
// library: ThreadSanitizer and AddressSanitizer see the mappings
// through their mmap interceptors, which a system call bypasses.
#if defined(XO_TSAN) || defined(XO_ASAN)
#define heap_libc 1
#else
#define heap_libc 0
#endif

// hw_slow_c is the calling thread's heap record for obj.xo's heap_word
// (called through rt.call_c2: two Int arguments, unused).
static int64_t hw_slow_c(int64_t a, int64_t b) {
	(void)a;
	(void)b;
	return (int64_t)(intptr_t)heap_slow();
}

// rl_free_rt and rc_defer_rt are obj.xo's way back to the C in a
// real-time section (its queue, rt_defer): rl_free_n_slow and
// rc_defer_n call them through rt.call_c2.
static int64_t rl_free_rt(int64_t l, int64_t h);
static int64_t rc_defer_rt(int64_t obj, int64_t drop);

static void obj_heap_init(xo_heap *h) {
	xo_x_obj_heap_init((int64_t)(intptr_t)h, (int64_t)(intptr_t)&hw_slow_c, (int64_t)(intptr_t)&rl_free_rt, (int64_t)(intptr_t)&rc_defer_rt, heap_libc, (int64_t)sizeof(xo_heap));
}

// The heap words of obj.xo (its *_n entries, which arm64 generated code
// calls without a C level) are set before anything allocates: first in
// main, or, in a program with its own main (XO_RT_NO_MAIN: the C tests
// of internal/llvm, linked by clang), in a constructor. Not a
// constructor always: xo's own ELF linker takes no .init_array
// (gotchas/runtime-c-no-constructors.md).
#ifdef XO_RT_NO_MAIN
static void fault_init(void);
void xo_x_proc_start(int64_t argc, int64_t argv, int64_t envp);
#ifndef _WIN32
extern char **environ;
#endif
__attribute__((constructor)) static void obj_heap_words(void) {
	obj_heap_init(&main_heap);
	// the environment for the runtime's settings (poolsetup.xo reads
	// XO_THREADS)
	xo_x_proc_start(0, 0, (int64_t)(intptr_t)environ);
	fault_init();
}
#endif

void xo_rt_threads_begin(void) {
	if (__atomic_load_n(&xo_threaded, __ATOMIC_RELAXED)) {
		return;
	}
	tls_heap = &main_heap;
	__atomic_store_n(&solo_heap, NULL, __ATOMIC_RELAXED);
	obj_heap_init(NULL);
	__atomic_store_n(&xo_threaded, 1, __ATOMIC_RELAXED);
	// standard output's buffer takes its lock from now on (stdio.xo)
	xo_x_stdio_threads();
}

// xo_rt_live returns the blocks allocated and not freed, and the blocks
// allocated, over all heaps (for the leak check, when no other thread
// runs).
void xo_rt_live(int64_t *live, int64_t *total) {
	xo_x_heap_live((int64_t)(intptr_t)live, (int64_t)(intptr_t)total);
}

// The runtime's records from malloc (std/native rt.heap_large): the C
// frees some of them with free (xo_testing.c, tasks.xo task_free), so these stay
// the C library's until the runtime has an allocator of its own for them.
//
// xo_heap_large returns a block of n bytes from malloc (the prefix is
// minus its size).
__attribute__((noinline)) int64_t xo_heap_large(int64_t n) {
	int64_t *b = malloc((size_t)n + 8);
	if (!b) {
		xo_oom();
	}
	b[0] = -n;
	return (int64_t)(intptr_t)(b + 1);
}

// xo_heap_large_resize reallocates the malloc'd block at b (its prefix)
// to n bytes.
int64_t xo_heap_large_resize(int64_t b, int64_t n) {
	int64_t *nb = realloc((void *)(intptr_t)b, (size_t)n + 8);
	if (!nb) {
		xo_oom();
	}
	nb[0] = -n;
	return (int64_t)(intptr_t)(nb + 1);
}

void xo_heap_release(int64_t b) { free((void *)(intptr_t)b); }

// The heap's platform calls (zero-C audit B21): std/native/llrt/heapos.xo
// maps the chunks and alloc.xo's big blocks (above mid_max) from the
// system and locks the central lists; alloc.xo calls these as rt.heap_*,
// which forward to cabi.xo's exports.
int64_t xo_x_heap_big(int64_t n);
int64_t xo_x_heap_big_resize(int64_t b, int64_t n);
void xo_x_heap_big_release(int64_t b);
int64_t xo_x_heap_grow(int64_t h, int64_t n);
void xo_x_heap_lock(void);
void xo_x_heap_unlock(void);
int64_t xo_x_heap_chunks(void);

// xo_heap_big returns a big block of n bytes (its prefix minus n): the
// address after the prefix.
int64_t xo_heap_big(int64_t n) { return xo_x_heap_big(n); }

// xo_heap_big_resize makes the big block whose prefix is at b n bytes.
int64_t xo_heap_big_resize(int64_t b, int64_t n) { return xo_x_heap_big_resize(b, n); }

// xo_heap_big_release gives back the big block whose prefix is at b.
void xo_heap_big_release(int64_t b) { xo_x_heap_big_release(b); }

// xo_heap_grow gives heap h a new chunk and allocates n bytes again (the
// heap in Xo calls it last, so its arm64 code keeps no register across
// a call).
int64_t xo_heap_grow(int64_t h, int64_t n) { return xo_x_heap_grow(h, n); }

void xo_heap_lock(void) { xo_x_heap_lock(); }

void xo_heap_unlock(void) { xo_x_heap_unlock(); }

// xo_chunks is the chunks mapped so far (std/llrt/testdata/tasks.c and
// alloc_threads.c check that blocks freed on another thread come back
// instead, decision 0118).
#define xo_chunks_now() xo_x_heap_chunks()

// The heap in Xo (decision 0207 stage 3): std/native/llrt/alloc.xo
// through the C ABI exports of cabi.xo (its C path was deleted by
// decision 0198); the C finds the calling thread's heap record.
int64_t xo_x_heap_alloc(int64_t n, int64_t h);
void xo_x_heap_free(int64_t p, int64_t h);
void xo_x_heap_refill(int64_t h, int64_t n);
void xo_x_heap_free_mt(int64_t h, int64_t p);
int64_t xo_x_heap_resize(int64_t h, int64_t p, int64_t old, int64_t n, int64_t mt);

#define HA(h) ((int64_t)(intptr_t)(h))
// With LTO (the LLVM backend) the calls of the single threaded paths are
// inlined as the C's alloc_in and free_in were (always_inline on the
// call: LLVM's own cost model kept the Xo function out of line in the
// hot string builders, bench/results/raw-memory-2026-10-13.md stage 3).
// The statement form needs clang 17 (LLVM; Apple clang 16): older ones,
// such as Amazon Linux's clang 15, fail on `[[` in C and even on
// `__has_c_attribute(clang::always_inline)`.
#if defined(__clang__) && ((defined(__apple_build_version__) && __clang_major__ >= 16) || (!defined(__apple_build_version__) && __clang_major__ >= 17))
#define XO_INLINE_CALL [[clang::always_inline]]
#endif
#ifndef XO_INLINE_CALL
#define XO_INLINE_CALL
#endif
static inline __attribute__((always_inline)) void *alloc_in(xo_heap *h, int64_t n) {
	int64_t p;
	XO_INLINE_CALL p = xo_x_heap_alloc(n, HA(h));
	return (void *)(intptr_t)p;
}
static inline __attribute__((always_inline)) void free_in(xo_heap *h, void *p) {
	XO_INLINE_CALL xo_x_heap_free(HA(p), HA(h));
}
__attribute__((noinline)) static void *alloc_mt(int64_t n) {
	xo_heap *h = heap_slow();
	xo_x_heap_refill(HA(h), n);
	return alloc_in(h, n);
}
__attribute__((noinline)) static void free_mt(void *p) { xo_x_heap_free_mt(HA(heap_slow()), HA(p)); }
static inline void *resize_in(xo_heap *h, void *p, int64_t old, int64_t n, int64_t mt) { return (void *)(intptr_t)xo_x_heap_resize(HA(h), HA(p), old, n, mt); }
#undef HA

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
// block of old bytes, which is freed). Out of line: rs_append is its only
// caller since the lists went to obj.xo, and inlined there it made the
// in place append save registers on every call (LLVM text replace and
// json enc_small 1.03 to 1.05 times the instructions).
__attribute__((noinline)) static void *xo_resize(void *p, int64_t old, int64_t n) {
	xo_heap *h = __atomic_load_n(&solo_heap, __ATOMIC_RELAXED);
	int64_t mt = 0;
	if (__builtin_expect(!h, 0)) {
		h = heap_slow();
		mt = 1;
	}
	// the heap in Xo moves a block itself unless mt
	void *q = resize_in(h, p, old, n, mt);
	if (q) {
		return q;
	}
	q = xo_alloc(n);
	memcpy(q, p, (size_t)(old < n ? old : n));
	xo_free(p);
	return q;
}

void *xo_rc_alloc(int64_t n) { return xo_alloc(n); }

// The lists in Xo (zero-C audit B13): std/native/llrt/obj.xo
// through cabi.xo's obj_* exports. hw is the heap word they take: the
// calling thread's heap record, plus 1 when other threads run (obj.xo
// then takes alloc_mt's, free_mt's, and xo_resize's paths).
int64_t xo_x_obj_rl_new(int64_t hw, int64_t cap, int64_t esize);
void xo_x_obj_rl_share(int64_t l, int64_t esize, int64_t share);
void xo_x_obj_rl_free(int64_t hw, int64_t l, int64_t esize, int64_t drop);
void xo_x_obj_rc_defer(int64_t h, int64_t obj, int64_t drop);

__attribute__((noinline)) static int64_t hw_slow(void) { return (int64_t)(intptr_t)heap_slow() | 1; }

static inline __attribute__((always_inline)) int64_t hw(void) {
	xo_heap *h = __atomic_load_n(&solo_heap, __ATOMIC_RELAXED);
	if (__builtin_expect(!h, 0)) {
		return hw_slow();
	}
	return (int64_t)(intptr_t)h;
}

#define XO_HW_HEAP(w) ((xo_heap *)(intptr_t)((w) & ~(int64_t)1))

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
// calling thread's (obj.xo rc_defer_n, which arm64 generated code calls
// directly; LLVM generated code and the seed's call this entry, which LTO
// folds into its callers).
void xo_x_obj_rc_defer_n(int64_t obj, int64_t drop);

void xo_rc_defer(void *obj, xo_drop_fn drop) { XO_INLINE_CALL xo_x_obj_rc_defer_n((int64_t)(intptr_t)obj, (int64_t)(intptr_t)drop); }

// rc_defer_c is xo_rc_defer in a real-time section (obj.xo rc_defer_n
// through rc_defer_rt): the section's queue, else the worklist.
static void rc_defer_c(void *obj, xo_drop_fn drop) {
	xo_heap *h = heap();
	if (__builtin_expect(h->rt_depth != 0, 0) && !h->defer_busy && rt_defer(h, RT_DEFER, obj, 0, (void *)drop)) {
		return;
	}
	XO_INLINE_CALL xo_x_obj_rc_defer((int64_t)(intptr_t)h, (int64_t)(intptr_t)obj, (int64_t)(intptr_t)drop);
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

// The counts in Xo (decision 0207 stage 2): std/native/llrt/rc.xo through
// the C ABI exports of cabi.xo (its C path was deleted by decision 0198).
void xo_x_rc_inc(int64_t p);
int64_t xo_x_rc_dec(int64_t p);
int64_t xo_x_rc_dec_shared(int64_t p);
int64_t xo_x_rc_claim(int64_t p);
int64_t xo_x_rc_mark(int64_t p);

static inline __attribute__((always_inline)) void rc_inc(int64_t *p) { xo_x_rc_inc((int64_t)(intptr_t)p); }
static inline __attribute__((always_inline)) int rc_dec_shared(int64_t *p) { return (int)xo_x_rc_dec_shared((int64_t)(intptr_t)p); }
static inline __attribute__((always_inline)) int rc_dec(int64_t *p) { return (int)xo_x_rc_dec((int64_t)(intptr_t)p); }
static inline __attribute__((always_inline)) int rc_claim(int64_t *p) { return (int)xo_x_rc_claim((int64_t)(intptr_t)p); }
static inline __attribute__((always_inline)) int rc_mark(int64_t *p) { return (int)xo_x_rc_mark((int64_t)(intptr_t)p); }

// xo_rc_inc adds a reference (rc_threads.c plays generated code with it).
void xo_rc_inc(int64_t *p) { rc_inc(p); }

// xo_rc_dec drops a reference and reports whether it was the last one.
int xo_rc_dec(int64_t *p) { return rc_dec(p); }

// xo_rc_claim reports whether the caller holds the only reference.
int xo_rc_claim(int64_t *p) { return rc_claim(p); }

// xo_rc_mark makes the count at p shared and reports whether it was
// thread local (the caller then marks the object's children).
int xo_rc_mark(int64_t *p) { return rc_mark(p); }

// ---------------------------------------------------------------------------
// The test world (decision 0083, xo_testing.c): a test binary run with
// XO_TEST set runs one test against fakes. tw.on switches the clock,
// timers, standard streams, environment, network listeners, and random
// numbers of this file to the test world; the rest of it is in
// xo_testing.c (included at the end).

static struct {
	int on;
	int64_t vnow;    // the virtual clock: wall time, ns (atomic)
	// every context is blocked: advance the clock (atomic): 0, or 1 +
	// the vepoch the deadlock check that found every context blocked
	// read first; tw_kick moves the clock only while vepoch is still
	// that one (gotchas/virtual-clock-double-kick.md)
	int64_t vpending;
	int64_t vepoch; // moves of the clock begun and ended, 2 per move (atomic, ev.mu)
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


// ---------------------------------------------------------------------------
// Output: standard output's buffer is Xo (std/native/llrt/stdio.xo, the
// zero-C audit's work item B03); the C flushes it through cabi.xo's
// exports before a wait, at exit, and before a fault's message.

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

void xo_x_flush(void);
void xo_x_flush_try(void);

void xo_flush(void) { xo_x_flush(); }

// flush_pending writes buffered output before the caller waits (decision
// 0069; nothing when the buffer is empty).
static void flush_pending(void) { xo_x_flush(); }

// ---------------------------------------------------------------------------
// C ABI exports (decisions 0204, 0207): functions of
// std/native/llrt/cabi.xo that every native program carries, called here
// by name. Addresses pass as int64_t; each writes its text and a zero
// byte into the buffer it is given and returns the text's length.

int64_t xo_x_fmt_int(int64_t buf, int64_t v);
int64_t xo_x_fmt_float(int64_t buf, double f);
int64_t xo_x_utf8_decode(int64_t p, int64_t n);
int64_t xo_x_utf8_encode(int64_t p, int64_t r);
int64_t xo_x_is_space(int64_t r);
int64_t xo_x_parse_int(int64_t p, int64_t n, int64_t out);
int64_t xo_x_fmt_duration(int64_t buf, int64_t d);
int64_t xo_x_fmt_time(int64_t buf, int64_t t);
int64_t xo_x_b64_val(int64_t c);
int64_t xo_x_days_from_civil(int64_t y, int64_t m, int64_t d);
int64_t xo_x_split_hostport(int64_t a, int64_t host, int64_t hn, int64_t port, int64_t pn);
int64_t xo_x_port_out_of_range(int64_t p);
int64_t xo_x_net_admits_addr(int64_t lp, int64_t ln, int64_t a, int64_t n);

#define XO_ADDR(p) ((int64_t)(intptr_t)(p))

// Number text (the Go backend's: strconv.FormatInt, and FloatStr, the
// shortest 'g' form with ".0" added to whole numbers), in Xo (num.xo):
// buf holds at least 32 bytes for an Int, 48 for a Float.
static int fmt_int(char *buf, int64_t v) { return (int)xo_x_fmt_int(XO_ADDR(buf), v); }

static int fmt_float(char *buf, double f) { return (int)xo_x_fmt_float(XO_ADDR(buf), f); }

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

// The fault entry points of generated code are cabi.xo's fault_raise_*
// exports (decision 0228: fault.xo writes the message and calls
// fault_raise_msg, whose address fault_init gives it), which the
// emitters name. These forward to them for code from an xo whose
// emitters named xo_fault_* (seed 0.11.0 builds the next xo with them):
// delete them once a seed names the exports.
void xo_x_fault_raise_text(int64_t p, int64_t n, int64_t loc);
void xo_x_fault_raise_overflow(int64_t a, int64_t op, int64_t b, int64_t loc);
void xo_x_fault_raise_shl(int64_t a, int64_t b, int64_t loc);
void xo_x_fault_raise_shift(int64_t n, int64_t loc);
void xo_x_fault_raise_divzero(int64_t a, int64_t op, int64_t loc);
void xo_x_fault_raise_neg(int64_t a, int64_t loc);
void xo_x_fault_raise_abs(int64_t a, int64_t loc);
void xo_x_fault_raise_index(int64_t i, int64_t n, int64_t loc);
void xo_x_fault_raise_float_int(double f, int64_t loc);
void xo_x_fault_init(int64_t raise_msg);

XO_NORETURN void xo_fault(const char *msg, int64_t n, const char *loc) {
	xo_x_fault_raise_text(XO_ADDR(msg), n, XO_ADDR(loc));
	__builtin_unreachable();
}
XO_NORETURN void xo_fault_overflow(int64_t a, int64_t op, int64_t b, const char *loc) {
	xo_x_fault_raise_overflow(a, op, b, XO_ADDR(loc));
	__builtin_unreachable();
}
XO_NORETURN void xo_fault_shl(int64_t a, int64_t b, const char *loc) {
	xo_x_fault_raise_shl(a, b, XO_ADDR(loc));
	__builtin_unreachable();
}
XO_NORETURN void xo_fault_shift(int64_t n, const char *loc) {
	xo_x_fault_raise_shift(n, XO_ADDR(loc));
	__builtin_unreachable();
}
XO_NORETURN void xo_fault_divzero(int64_t a, int64_t op, const char *loc) {
	xo_x_fault_raise_divzero(a, op, XO_ADDR(loc));
	__builtin_unreachable();
}
XO_NORETURN void xo_fault_neg(int64_t a, const char *loc) {
	xo_x_fault_raise_neg(a, XO_ADDR(loc));
	__builtin_unreachable();
}
XO_NORETURN void xo_fault_abs(int64_t a, const char *loc) {
	xo_x_fault_raise_abs(a, XO_ADDR(loc));
	__builtin_unreachable();
}
XO_NORETURN void xo_fault_index(int64_t i, int64_t n, const char *loc) {
	xo_x_fault_raise_index(i, n, XO_ADDR(loc));
	__builtin_unreachable();
}
XO_NORETURN void xo_fault_float_int(double f, const char *loc) {
	xo_x_fault_raise_float_int(f, XO_ADDR(loc));
	__builtin_unreachable();
}

// fault_init gives fault.xo the address of fault_raise_msg and ctx.xo
// that of ctx_root (with the heap words: first in main, or in
// XO_RT_NO_MAIN's constructor).
void xo_x_ctx_init(int64_t root, int64_t pl);
static int64_t ctx_root(int64_t a, int64_t b);
static int64_t svc_addr(void);
static void pd_svc_init(void);
static void stop_svc_init(void);
static void fault_init(void) {
	xo_x_fault_init(XO_ADDR(fault_raise_msg));
	xo_x_ctx_init(XO_ADDR(ctx_root), svc_addr());
	pd_svc_init();
	stop_svc_init();
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

// alloc_usable returns a block of at least n bytes and in *bytes the
// bytes it really has (its size class), so strings can grow into the
// slack. The heap in Xo leaves them in the heap record's last_size (one
// call instead of two, decision 0207 stage 3).
static inline void *alloc_usable(int64_t n, int64_t *bytes) {
	void *p = xo_alloc(n);
	*bytes = heap()->last_size;
	return p;
}

// rs_new is a new string object with room for at least n bytes: the C
// callers' (xo_str_slice, the number text, standard input, ...), kept in
// C so clang inlines the allocation into them (forwarding it to obj.xo
// took text lines to 1.054 on LLVM; obj.xo's rs_new_n is the same).
static xo_rstr *rs_new(int64_t n) {
	int64_t bytes;
	xo_rstr *s = alloc_usable((int64_t)sizeof(xo_rstr) + n, &bytes);
	s->rc = 1;
	s->cap = bytes - (int64_t)sizeof(xo_rstr);
	return s;
}

// rs_append appends the n bytes at p to a (of length an), consuming a:
// in place when a is unique (doubling when it is full: Perceus reuse, so
// a loop of `s = s + x` is linear), else into a new string: obj.xo's
// (zero-C audit B13).
int64_t xo_x_obj_rs_append_at(int64_t a, int64_t an, int64_t p, int64_t n);
static xo_rstr *rs_append(xo_rstr *a, int64_t an, const char *p, int64_t n) {
	return (xo_rstr *)(intptr_t)xo_x_obj_rs_append_at(XO_ADDR(a), an, XO_ADDR(p), n);
}

// xo_rs_concat is `a + b` (consumes a, borrows b): obj.xo's rs_append
// (zero-C audit B13), which arm64 generated code calls directly; LLVM
// generated code and the seed call this.
int64_t xo_x_obj_rs_concat(int64_t a, int64_t an, int64_t b, int64_t bn);
xo_rstr *xo_rs_concat(xo_rstr *a, int64_t an, xo_rstr *b, int64_t bn) {
	return (xo_rstr *)(intptr_t)xo_x_obj_rs_concat(XO_ADDR(a), an, XO_ADDR(b), bn);
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

// xo_rl_new is an empty list with room for cap elements (obj.xo rl_new).
xo_rlist *xo_rl_new(int64_t cap, int64_t esize) {
	int64_t l;
	XO_INLINE_CALL l = xo_x_obj_rl_new(hw(), cap, esize);
	return (xo_rlist *)(intptr_t)l;
}

// The entries generated code calls (LLVM generated code and the seed's;
// arm64 generated code calls the obj_*_n exports directly): they forward
// to obj.xo's entries, which find the heap themselves, so LTO folds this
// level into the caller.
int64_t xo_x_obj_rl_new_n(int64_t n, int64_t esize);
int64_t xo_x_obj_rl_grow_n(int64_t l, int64_t esize);
void xo_x_obj_rl_free_n(int64_t l, int64_t esize, int64_t drop);
int64_t xo_x_obj_rl_copy_n(int64_t l, int64_t esize, int64_t dup, int64_t drop);

// xo_rl_new_n is an empty list with room for n elements (at least 4):
// `list.new` with a size, for results whose length is known.
xo_rlist *xo_rl_new_n(int64_t n, int64_t esize) {
	int64_t l;
	XO_INLINE_CALL l = xo_x_obj_rl_new_n(n, esize);
	return (xo_rlist *)(intptr_t)l;
}

void xo_rl_free(xo_rlist *l, int64_t esize, xo_elem_fn drop);

#define XO_A(p) ((int64_t)(intptr_t)(p))

// xo_rl_copy is the slow path of `reuse`: l's count is not 1 (obj.xo
// rl_copy).
__attribute__((noinline)) xo_rlist *xo_rl_copy(xo_rlist *l, int64_t esize, xo_elem_fn dup, xo_elem_fn drop) {
	return (xo_rlist *)(intptr_t)xo_x_obj_rl_copy_n(XO_A(l), esize, XO_A(dup), XO_A(drop));
}

// xo_rl_share marks the elements of a list that was just marked shared
// (obj.xo rl_share; boxes and environments below go through
// xo_rc_defer's worklist, so the stack is bounded by the nesting of
// types, not of values).
void xo_rl_share(xo_rlist *l, int64_t esize, xo_elem_fn share) { xo_x_obj_rl_share(XO_A(l), esize, XO_A(share)); }

// xo_rl_grow doubles the capacity of a unique full list (obj.xo rl_grow).
__attribute__((noinline)) xo_rlist *xo_rl_grow(xo_rlist *l, int64_t esize) { return (xo_rlist *)(intptr_t)xo_x_obj_rl_grow_n(XO_A(l), esize); }

// xo_rl_free frees a list whose count reached zero, releasing its
// elements first (obj.xo rl_free_n); a real-time section defers it.
void xo_rl_free(xo_rlist *l, int64_t esize, xo_elem_fn drop) { XO_INLINE_CALL xo_x_obj_rl_free_n(XO_A(l), esize, XO_A(drop)); }

// rl_free_c is xo_rl_free in a real-time section (obj.xo rl_free_n
// through rl_free_rt): the section's queue, else freed at once.
static void rl_free_c(xo_rlist *l, int64_t esize, xo_elem_fn drop) {
	int64_t w = hw();
	xo_heap *h = XO_HW_HEAP(w);
	if (__builtin_expect(h->rt_depth != 0, 0) && rt_defer(h, RT_LIST, l, esize, (void *)drop)) {
		return;
	}
	if (!drop) {
		// no elements to release: the free alone (obj.xo's loop would
		// save its registers on this path too)
		xo_free(l);
		return;
	}
	xo_x_obj_rl_free(w, XO_A(l), esize, XO_A(drop));
}

static int64_t rl_free_rt(int64_t l, int64_t h) {
	xo_heap *hp = (xo_heap *)(intptr_t)h;
	rl_free_c((xo_rlist *)(intptr_t)l, hp->pend_esize, (xo_elem_fn)(intptr_t)hp->pend_drop);
	return 0;
}

static int64_t rc_defer_rt(int64_t obj, int64_t drop) {
	rc_defer_c((void *)(intptr_t)obj, (xo_drop_fn)(intptr_t)drop);
	return 0;
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

#ifndef _WIN32
#include <sys/types.h>
#endif

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
// scheduler's events to stderr at exit (std/native/llrt/entry.xo
// rt_stats prints them from st, in its order of names). Each count is one
// relaxed atomic add, done only when enabled.
static int rtstats = -1; // -1 not read yet
static struct {
	int64_t spawn, steal, inject, wake, idle_sleep, spin_round, fiber_park, fiber_new, thread_wait, thread_wake, lock_slow, spin_yield;
	// sockets: read and write system calls, those that would block
	// (EAGAIN), poller returns, readiness events, and waiters they woke
	int64_t net_read, net_write, net_again, poll_return, poll_event, poll_wake;
	// real-time sections (decision 0133): sections run, frees deferred,
	// frees done at once because the queue was full
	int64_t rt_section, rt_defer, rt_overflow;
	// fiber stacks mapped now (always counted) and at most at once (each
	// fibers.stack plus a guard page of address space): the measure of a
	// program's address space need under ThreadSanitizer on Linux
	// (gotchas/linux-tsan-fiber-stacks.md)
	int64_t fiber_live, fiber_peak;
} st;

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

// Futexes (decision 0077): wait while a 32 bit word holds a value, and
// wake the threads waiting on a word. Darwin has no public futex; its
// __ulock calls are what libc++ uses for std::atomic::wait. Both are
// never inlined: they may set errno (Linux), which must not be read
// across a fiber switch (gotchas/fiber-thread-local-address.md).
// Decision 0212 stage 2: they and the lock's slow path are Xo
// (std/native/llrt/pool.xo, through cabi.xo).
void xo_x_futex_wake(int64_t a);
void xo_x_lock_slow(int64_t l);
static void futex_wake(void *a) { xo_x_futex_wake(XO_ADDR(a)); }

// xo_lock is the lock of the task runtime's objects (decision 0077): a
// word that is 0 (free), 1 (held), or 2 (held, a thread may sleep on
// it; the futex word is its low half). A contended lock spins a little,
// then sleeps on the word; an unlock makes a system call only when
// someone may sleep. Unlike a pthread mutex it needs no initialization
// (zero is free) and has no fair share hand off
// (gotchas/darwin-mutex-fairshare.md). A word of 64 bits: the slot
// atomics of std/native/llrt/pool.xo, which takes and releases the
// pool's locks too, are of words (decision 0212 stage 2).
typedef struct {
	int64_t v;
} xo_lock;

// the slow path in Xo, then a load ThreadSanitizer sees acquire the
// word's last release (the Xo code is not instrumented)
static void lock_slow(xo_lock *l) {
	xo_x_lock_slow(XO_ADDR(l));
	(void)__atomic_load_n(&l->v, __ATOMIC_ACQUIRE);
}

static inline void lk(xo_lock *l) {
	int64_t z = 0;
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
	int64_t signaled; // written under mu, read atomically (the futex word: its low half)
	int sleeping;     // a thread sleeps on signaled
	int counted;      // a root asleep here counts in dlk.blocked
	// the fiber parked here (decision 0069): a signal makes it runnable
	struct xo_fiber *fiber;
} xo_park;

// ThreadSanitizer does not see the synchronization of the Xo code (it is
// not instrumented): the C around the parks of std/native/llrt/pool.xo
// tells it that a signal releases the park and a wait that returns
// acquires it (decision 0212 stage 2).
#if defined(__has_feature)
#if __has_feature(thread_sanitizer)
#define XO_TSAN_ANN 1
#endif
#endif
#if defined(__SANITIZE_THREAD__)
#define XO_TSAN_ANN 1
#endif
#ifdef XO_TSAN_ANN
void __tsan_acquire(void *addr);
void __tsan_release(void *addr);
#define TSAN_ACQUIRE(a) __tsan_acquire(a)
#define TSAN_RELEASE(a) __tsan_release(a)
#else
#define TSAN_ACQUIRE(a) ((void)(a))
#define TSAN_RELEASE(a) ((void)(a))
#endif

static void park_init(xo_park *p) { memset(p, 0, sizeof *p); }


// The parks and the cancellation tree in Xo (decision 0212 stage 2):
// std/native/llrt/pool.xo through cabi.xo, wrapped below the pool's
// record.
static void park_reset(xo_park *p);
static void park_signal(xo_park *p);

// The cancellation tree: every node has a lock for its list of children
// and its waiter. A walk that cancels a subtree holds the locks from the
// node down to the one it visits, so locks are always taken parent
// first; linking and unlinking take only the parent's (decision 0077:
// one lock for the whole tree serialized every spawn and every wait).
typedef struct xo_node {
	struct xo_node *parent, *child, *prev, *next;
	int64_t canceled; // atomic reads; written under the parent's and its own lock
	xo_park *waiter; // the park of a context blocked at this node (mu)
	xo_lock mu;      // child, the children's prev and next, waiter
} xo_node;

static int node_canceled(xo_node *n) { return __atomic_load_n(&n->canceled, __ATOMIC_ACQUIRE) != 0; }

static void node_link(xo_node *n, xo_node *parent);
static void node_unlink(xo_node *n);
static void node_cancel(xo_node *n);

typedef struct {
	void *m;
	const char *loc;
	int64_t n;
} xo_held;

struct xo_task;

// A wait list entry: a context's park in the list of an object it waits
// on (tasks.xo E_*).
typedef struct xo_went {
	xo_park *park;
	struct xo_went *prev, *next;
	int writer;  // a Mutex waiter that wants the lock exclusively
	int granted; // a Mutex reader the lock was handed to (atomic, mutex_grant_readers)
} xo_went;

// A channel's queue entry: a waiter's park in a channel's queue
// (chan.xo E_*).
typedef struct xo_cent {
	xo_park *park;
	int64_t *fired; // claimed by the first waker (atomic)
	xo_rlist *item; // a send's value, or what a receive got (NULL: closed)
	int shut;       // the channel closed while a send waited
	// a select's arm: the waker stores idx in *chosen
	int64_t idx, *chosen;
	struct xo_cent *prev, *next;
} xo_cent;

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
	// resource budgets (decision 0175): the budget the context works
	// under (a reference) or NULL, the polls left before its next charge
	// and the quantum they started from, and the stretch being charged:
	// its start (monotonic ns) and the allocation count of its thread's
	// heap then
	struct xo_budget *bud;
	int64_t bud_left, bud_q, bud_run, bud_amark;
	struct xo_heap *bud_heap;
	int bud_paused; // bud_pause depth
	// the context's entry in the wait list it waits on (wl_wait_as,
	// tasks.xo wl_wait: one at a time, decision 0212 stage 3)
	xo_went went;
	// the context's entry in the channel queue it waits on and the word
	// its waker claims (chan.xo; one at a time)
	xo_cent cent;
	int64_t fired;
} xo_ctx;

// Thread locals are read and written only through these functions, which
// are never inlined (decision 0069): a task on a fiber may go on on
// another thread after it parks, and within one function the compiler may
// keep a thread local's address (it treats it as a constant, also after
// link time inlining), which would then be the old thread's.
// Decision 0212: the thread locals are words of the table the compiler
// emits for rt.tlocal_get and rt.tlocal_set (xo_tlocal, a thread local
// of the program's code; std/native/llrt/sched.xo reads and writes the
// same words through the emitted xo_tlocal_get and xo_tlocal_set). The C
// reads the table itself, through these accessors only (above).
extern __thread int64_t xo_tlocal[16];
// the words (sched.xo TLS_*): the current context, the fiber running on
// the thread, and the thread's worker record
enum { XO_TLS_CUR = 0, XO_TLS_FIB = 1, XO_TLS_W = 2 };

__attribute__((noinline)) static xo_ctx *cur_get(void) { return (xo_ctx *)(intptr_t)xo_tlocal[XO_TLS_CUR]; }
__attribute__((noinline)) static void cur_set(xo_ctx *c) { xo_tlocal[XO_TLS_CUR] = XO_ADDR(c); }
__attribute__((noinline)) static struct xo_fiber *fib_get(void) { return (struct xo_fiber *)(intptr_t)xo_tlocal[XO_TLS_FIB]; }

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

// The context record's layout, which ctx.xo's C_* name (decision 0229).
_Static_assert(offsetof(xo_ctx, node) == 0 && offsetof(xo_ctx, unwinding) == 8 && offsetof(xo_ctx, shield) == 12 &&
                   offsetof(xo_ctx, park) == 16 && offsetof(xo_ctx, held) == 48 && offsetof(xo_ctx, nheld) == 56 &&
                   offsetof(xo_ctx, capheld) == 60 && offsetof(xo_ctx, net_status) == 64 && offsetof(xo_ctx, net_msg) == 72 &&
                   offsetof(xo_ctx, faulting) == 80 && offsetof(xo_ctx, fault_msg) == 88 && offsetof(xo_ctx, fault_loc) == 96 &&
                   offsetof(xo_ctx, fault_detail) == 104 && offsetof(xo_ctx, boundary) == 112 && offsetof(xo_ctx, fault_exc) == 128,
               "ctx.xo C_*");
_Static_assert(offsetof(xo_ctx, bud) == 160 && offsetof(xo_ctx, bud_left) == 168 && offsetof(xo_ctx, bud_q) == 176 &&
                   offsetof(xo_ctx, bud_run) == 184 && offsetof(xo_ctx, bud_amark) == 192 && offsetof(xo_ctx, bud_heap) == 200 &&
                   offsetof(xo_ctx, bud_paused) == 208 && offsetof(xo_ctx, went) == 216 && offsetof(xo_ctx, cent) == 248 && offsetof(xo_ctx, fired) == 312 && sizeof(xo_ctx) == 320 &&
                   sizeof(xo_went) == 32 && offsetof(xo_went, writer) == 24,
               "ctx.xo C_BUD_*");

// ctx_root is ctx for ctx.xo cur (its address in rt.static_word 15).
static int64_t ctx_root(int64_t a, int64_t b) {
	(void)a;
	(void)b;
	return XO_ADDR(ctx());
}

// xo_unwinding is ctx.xo unwinding, which generated code calls directly
// (this forwards for older xo's code).
int64_t xo_x_ctx_unwinding(void);
int64_t xo_unwinding(void) { return xo_x_ctx_unwinding(); }

// ---------------------------------------------------------------------------
// Fault unwinding (core 8, decision 0085)
//
// A fault is raised as a forced unwind (decision 0221): on Linux and
// macOS the Xo unwinder walks the frames (std/native/llrt/unwind.xo
// raise), on Windows the system's unwinder (decision 0197); glibc
// linux/amd64 keeps libgcc's forced unwind until decision 0221's stage 2.
// Every frame of generated code has unwind tables, and its landing pads (the
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
// Frames of C with cleanups between a fault and its catch are passed
// over, their cleanups not run (the runtime's C has none).

// XO_UNWIND_LIBGCC: x86_64 with a C library (glibc linux/amd64 and
// darwin/amd64), which still raise a fault with the platform's
// _Unwind_ForcedUnwind (libgcc, libunwind) and the C personality below,
// until decision 0221's stage 2 gives the Xo unwinder x86_64.
#if !defined(_WIN32) && !defined(XO_NOLIBC) && !defined(__aarch64__)
#define XO_UNWIND_LIBGCC 1
#endif

#define XO_EXC_CLASS 0x584f00004641554cULL // "XO\0\0FAUL"

// The fault record (faulting, fault_msg, fault_loc, fault_detail) is
// kept by std/native/llrt/fault.xo (the zero-C audit's B06).
_Static_assert(offsetof(xo_ctx, unwinding) == 8 && offsetof(xo_ctx, faulting) == 80 && offsetof(xo_ctx, fault_msg) == 88 &&
                   offsetof(xo_ctx, fault_loc) == 96 && offsetof(xo_ctx, fault_detail) == 104,
               "fault.xo C_*");
void xo_x_fault_record_set(int64_t c, int64_t mp, int64_t mn, int64_t lp, int64_t ln);
void xo_x_fault_record_detail(int64_t c, int64_t p, int64_t n);
void xo_x_fault_record_clear(int64_t c);
int64_t xo_x_fault_record_take(int64_t c);
void xo_x_fault_record_exit(int64_t c);

// fault_exit reports the fault of c that nothing caught and ends the
// process with status 70 (fault.xo exit).
XO_NORETURN static void fault_exit(xo_ctx *c) {
	xo_x_fault_record_exit(XO_ADDR(c));
	__builtin_unreachable();
}

#if !defined(XO_UNWIND_LIBGCC)
int64_t xo_x_unwind_fault(int64_t exc, int64_t boundary, int64_t pers, int64_t pers_old, int64_t wcx);
int64_t xo_x_personality(int64_t version, int64_t actions, int64_t cls, int64_t exc, int64_t uc);
#if defined(_WIN32)
EXCEPTION_DISPOSITION xo_personality(PEXCEPTION_RECORD rec, PVOID frame, PCONTEXT cx, PVOID disp);
#elif defined(XO_NOLIBC)
int xo_personality(void); // the link's --defsym of xo_x_personality
#else
int xo_personality(int version, int actions, uint64_t cls, void *e, void *uc);
#endif

// fault_raise on Linux, macOS, and Windows (decision 0221; Windows,
// decision 0197 and the zero-C audit's B06): the unwinder is Xo
// (std/native/llrt/unwind.xo over the .eh_frame_hdr tables of the
// program and its shared objects on Linux, the compact unwind of
// __unwind_info and __eh_frame on macOS, the system's RtlVirtualUnwind
// on Windows, win_raise), which runs the first landing pad of
// a frame of generated code (its personality is xo_x_personality, or
// the forwarder xo_personality that an xo before decision 0221's
// emitter change names) below the context's boundary, and returns when
// nothing catches the fault.
XO_NORETURN static void fault_raise(xo_ctx *c) {
	rt_fault_leave();
	c->faulting = 1;
	c->unwinding = 1;
#ifdef _WIN32
	// the walk starts here, so the unwinder's own frames are not unwound
	// on every raise (unwind.xo win_raise)
	CONTEXT cx;
	RtlCaptureContext(&cx);
	xo_x_unwind_fault(XO_ADDR(&c->fault_exc), (int64_t)c->boundary, XO_ADDR(xo_x_personality), XO_ADDR(xo_personality), XO_ADDR(&cx));
#else
	xo_x_unwind_fault(XO_ADDR(&c->fault_exc), (int64_t)c->boundary, XO_ADDR(xo_x_personality), XO_ADDR(xo_personality), 0);
#endif
	fault_exit(c);
}
#else
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
#endif

// fault_set records a fault in c (fault.xo set).
static void fault_set(xo_ctx *c, const char *msg, const char *loc) {
	xo_x_fault_record_set(XO_ADDR(c), XO_ADDR(msg), (int64_t)strlen(msg), XO_ADDR(loc), (int64_t)strlen(loc));
}

static void fault_clear(xo_ctx *c) { xo_x_fault_record_clear(XO_ADDR(c)); }

XO_NORETURN static void fault_raise_msg(const char *msg, const char *loc) {
	xo_ctx *c = ctx();
	fault_set(c, msg, loc);
	fault_raise(c);
}

void xo_fault_detail(const char *p, int64_t n) { xo_x_fault_record_detail(XO_ADDR(ctx()), XO_ADDR(p), n); }

// xo_fault_raise is `fault(msg)` and a failed contract at loc.
XO_NORETURN void xo_fault_raise(const char *msg, int64_t n, const char *loc, int64_t ln) {
	xo_ctx *c = ctx();
	xo_x_fault_record_set(XO_ADDR(c), XO_ADDR(msg), n, XO_ADDR(loc), ln);
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
int64_t xo_fault_take(void) { return xo_x_fault_record_take(XO_ADDR(ctx())); }

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
// action), offsets from the function's start. It is C on Windows and
// x86_64 (glibc linux/amd64 and darwin/amd64, until decision 0221's
// stage 2). Elsewhere the Xo unwinder reads the call site tables itself
// and only compares a frame's personality with the address of the
// export xo_x_personality, which the emitters name. With
// no C library the link defines xo_personality as the export; otherwise
// it is the forwarder below, for code from an xo whose emitters still
// name xo_personality (the seed 0.11.0 builds the next xo with them):
// delete it once the seed names the export.
#if !defined(_WIN32) && !defined(XO_UNWIND_LIBGCC) && !defined(XO_NOLIBC)
int64_t xo_x_personality(int64_t version, int64_t actions, int64_t cls, int64_t exc, int64_t uc);

int xo_personality(int version, int actions, uint64_t cls, void *e, void *uc) {
	return (int)xo_x_personality(version, actions, (int64_t)cls, XO_ADDR(e), XO_ADDR(uc));
}
#endif

#if defined(_WIN32)
// xo_personality is, on Windows, the language handler the system's
// exception dispatcher calls for frames of generated code (and the
// handler unwind.xo win_raise looks for): faults never go through the
// dispatcher, so it passes every system exception on.
EXCEPTION_DISPOSITION xo_personality(PEXCEPTION_RECORD rec, PVOID frame, PCONTEXT cx, PVOID disp) {
	(void)rec;
	(void)frame;
	(void)cx;
	(void)disp;
	return ExceptionContinueSearch;
}
#endif

#if defined(XO_UNWIND_LIBGCC)
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
#endif // XO_UNWIND_LIBGCC

int64_t xo_is_canceled(void) { return node_canceled(ctx()->node); }

// mono_ns is the monotonic clock in nanoseconds, read in Xo
// (std/native/llrt/sysio.xo mono_ns through cabi.xo; work item B01).
int64_t xo_x_mono_ns(void);
static inline int64_t mono_ns(void) { return xo_x_mono_ns(); }

// tnow is the time of timers: monotonic ns, or the virtual clock in the
// test world.
static int64_t tnow(void) { return tw.virt ? __atomic_load_n(&tw.vnow, __ATOMIC_ACQUIRE) : mono_ns(); }

// ---------------------------------------------------------------------------
// Resource budgets (std.md 7, decisions 0173 and 0175): std/rt budget.go.
// `task.with_budget(clock, b, f)` runs f as the body of a scope whose
// budget (xo_budget_open) every context working in it charges: the
// generated code of a program that calls with_budget polls (xo_tick) at
// the start of every loop iteration and at the entry of every function
// of the program's own modules. Every 1024 polls (budget.xo QUANTUM) a context
// charges its steps, its running time (monotonic time outside waits:
// park_wait_n and a task run inline stop the stretch), and the bytes it
// allocated (its thread's heap counts them, xo_heap.abytes) to its
// budget and the budgets enclosing it; a passed ceiling cancels the
// budget's scope, and the poll unwinds the context like a cancellation
// point. A task started under a budget works under it (xo_task_spawn).

// The budget record; std/native/llrt/budget.xo reads and writes it
// (decision 0229), the C only takes and drops references.
typedef struct xo_budget {
	struct xo_budget *parent; // a reference
	int64_t refs;             // atomic: the contexts and tasks under it, its children
	int64_t cpu, alloc, steps; // ceilings, 0: none
	int64_t used_cpu, used_alloc, used_steps; // atomic
	int64_t over;              // atomic: the resource passed first, 1 cpu 2 alloc 3 steps
	xo_lock mu;                // node
	xo_node *node;             // the scope body a passed ceiling cancels (NULL once with_budget returned)
	char *loc;                 // the with_budget call (the location of XO-F008)
} xo_budget;

_Static_assert(offsetof(xo_budget, refs) == 8 && offsetof(xo_budget, cpu) == 16 && offsetof(xo_budget, used_cpu) == 40 &&
                   offsetof(xo_budget, over) == 64 && offsetof(xo_budget, mu) == 72 && offsetof(xo_budget, node) == 80 &&
                   offsetof(xo_budget, loc) == 88 && sizeof(xo_budget) == 96,
               "budget.xo B_*");

void xo_x_budget_tick(void);
void xo_x_budget_open(int64_t s, int64_t cpu, int64_t alloc, int64_t steps, int64_t lp, int64_t ln);
int64_t xo_x_budget_end(int64_t s);
void xo_x_budget_retain(int64_t b);
void xo_x_budget_pause(int64_t c);
void xo_x_budget_resume(int64_t c);

static void bud_pause(xo_ctx *c) { xo_x_budget_pause(XO_ADDR(c)); }
static void bud_resume(xo_ctx *c) { xo_x_budget_resume(XO_ADDR(c)); }

// xo_tick is the budget poll of generated code (budget.xo tick, which
// arm64 code calls directly).
void xo_tick(void) { xo_x_budget_tick(); }

// The shields of deferred code (core 3.5, decision 0022) are ctx.xo's
// (decision 0229), which arm64 generated code calls through its exports;
// these forward to them for LLVM code and older xo's code.
int64_t xo_x_ctx_shield_begin(void);
void xo_x_ctx_shield_end(int64_t tok, int64_t lp, int64_t ln);
void xo_x_ctx_shield_abort(int64_t tok);

int64_t xo_shield_begin(void) { return xo_x_ctx_shield_begin(); }

void xo_shield_end(int64_t tok, const char *loc, int64_t n) { xo_x_ctx_shield_end(tok, XO_ADDR(loc), n); }

void xo_shield_abort(int64_t tok) { xo_x_ctx_shield_abort(tok); }

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

// The ring's head and tail count every item ever added and taken (64 bit:
// they never wrap). std/native/llrt/sched.xo works on this layout
// (decision 0212; the offsets are asserted below).
typedef struct {
	int64_t head; // atomic: consumers (the owner and thieves) CAS it
	char pad[120];
	int64_t tail;        // atomic: only the owner writes it
	void *slot[XO_RUNQ]; // atomic accesses: a thief may read a stale one
} xo_runq;

// The ring's operations, and rq_put, which moves half of a full ring and
// x to the inject queue, are std/native/llrt/sched.xo and pool.xo
// (decision 0212).

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
	// nonzero when the fiber code in Xo (sched.xo, decision 0212) calls
	// xo_sched_hook around each switch: the sanitizers' fiber calls, and
	// the statistics (sched_hooks)
	int64_t hooks;
	void *tsan; // the scheduler's ThreadSanitizer fiber
	int64_t wakew; // futex word (its low half): set to 1 to wake the idle worker
	struct xo_worker *idle_next;
	uint64_t rng;
	int64_t nonext; // pool_push puts woken fibers in the ring (mutex_signal)
	struct xo_fiber *fcache[XO_FCACHE]; // stacks to reuse (owner only)
	int64_t nfcache;
	// AddressSanitizer: the scheduler's stack (learned at the first
	// switch) and its fake stack while a fiber runs (ASAN_START below;
	// last, so the fields above keep their cache lines)
	const void *asan_bottom;
	size_t asan_size;
	void *asan_fake;
	// the pool (pool.xo's worker_main finds it here), and the owner's
	// scratch: a steal's batch, the inject queue's share, a full ring's
	// half (decision 0212 stage 2)
	void *pool;
	void *batch[XO_RUNQ / 2 + 1];
} xo_worker;

// The pool's record: words first, in the layout std/native/llrt/pool.xo
// reads (decision 0212 stage 2, asserted below), the once last (its size
// is the OS's).
static struct {
	xo_lock mu; // the idle list, starting workers
	int64_t ncpu;
	int64_t spin_rounds;
	int64_t nw;       // records used (atomic)
	int64_t alive;    // worker threads (atomic)
	int64_t nidle;    // idle worker threads (atomic)
	int64_t spinning; // spinning worker threads (atomic)
	xo_worker *idle;  // the idle list (mu)
	// the inject queue: items from threads without a record
	xo_lock imu;
	void **ibuf;
	int64_t ihead, ilen, icap; // ilen read atomically to skip it
	void *fibers;              // the fiber cache's record (pool.xo)
	int64_t worker_main;       // where a worker thread starts (pool.xo)
	void *dlk;                 // the deadlock counts (pool.xo's parks)
	xo_worker *ws[XO_MAXW]; // records (worker and root), never freed
} pool = {.dlk = &dlk}; // a root waits before the pool starts


static void rt_init(void) {}


// The pool's setup (pool_init until B20) and the root threads' worker
// records are poolsetup.xo's ready and root_worker; root_worker calls it from
// the C.
int64_t xo_x_pool_root_worker(int64_t p);

// The worker pool in Xo (decision 0212 stage 2): std/native/llrt/pool.xo
// through the C ABI exports of cabi.xo, over the records above (their
// layout is asserted below). The C keeps the records, their setup, the
// root threads' records, and the deadlock check; the Xo code calls back
// for the services in xo_sched_rt.
void xo_x_register_worker(int64_t p, int64_t w);
int64_t xo_x_any_work(int64_t p);
int64_t xo_x_worker_main(int64_t w);

static int any_work(void) { return (int)xo_x_any_work(XO_ADDR(&pool)); }

void xo_x_park_reset(int64_t p);
int64_t xo_x_scope_add(int64_t pl, int64_t s, int64_t t, int64_t w);
void xo_x_scope_join(int64_t pl, int64_t s, int64_t c, int64_t f, int64_t rw);
void xo_x_park_signal(int64_t pl, int64_t p);
void xo_x_node_link(int64_t n, int64_t parent);
void xo_x_node_unlink(int64_t n);
void xo_x_node_cancel(int64_t pl, int64_t n);

static void park_reset(xo_park *p) { xo_x_park_reset(XO_ADDR(p)); }
static void park_signal(xo_park *p) {
	TSAN_RELEASE(p);
	xo_x_park_signal(XO_ADDR(&pool), XO_ADDR(p));
}
static void node_link(xo_node *n, xo_node *parent) { xo_x_node_link(XO_ADDR(n), XO_ADDR(parent)); }
static void node_unlink(xo_node *n) { xo_x_node_unlink(XO_ADDR(n)); }
static void node_cancel(xo_node *n) { xo_x_node_cancel(XO_ADDR(&pool), XO_ADDR(n)); }

// the handles' free functions: cabi.xo's exports (tasks.xo, mutex.xo,
// chan.xo, selects.xo; B18)
void xo_x_task_free(int64_t p);
void xo_x_scope_free(int64_t p);
void xo_x_mutex_free(int64_t p);
void xo_x_chan_free(int64_t p);
void xo_x_select_free(int64_t p);
void xo_x_atomic_free(int64_t p);
void xo_x_fs_file_free(int64_t p);
void xo_x_fs_lock_free(int64_t p);
#define XO_FREE_FN(f) ((void (*)(void *))(void *)(f))

// The services record of ctx.xo (SV_*): the pool record (budget.xo and
// tasks.xo reach pool.xo through it), the free functions of a task and a
// scope (tasks.xo makes them), whether ThreadSanitizer is on, and the
// statistics' switch.
static struct {
	void *pool;
	void (*task_free)(void *);
	void (*scope_free)(void *);
	int64_t tsan;
	int *stats;
	xo_lock once; // poolsetup.xo ready's lock
	void (*mutex_free)(void *);
	void (*chan_free)(void *);
	void (*select_free)(void *);
	void (*atomic_free)(void *);
	void *fibers;
	int64_t fiber_main, worker_main;
	int64_t h2_pool[3]; // hclient.xo's idle HTTP/2 connections
	void (*fs_file_free)(void *), (*fs_lock_free)(void *); // fsops.xo's handles
	int64_t consts; // consts.xo's table
} svc = {&pool, XO_FREE_FN(xo_x_task_free), XO_FREE_FN(xo_x_scope_free),
#ifdef XO_TSAN_ANN
	1,
#else
	0,
#endif
	&rtstats, {0}, XO_FREE_FN(xo_x_mutex_free), XO_FREE_FN(xo_x_chan_free), XO_FREE_FN(xo_x_select_free), XO_FREE_FN(xo_x_atomic_free), NULL, 0, 0, {0}, XO_FREE_FN(xo_x_fs_file_free), XO_FREE_FN(xo_x_fs_lock_free), 0};

_Static_assert(offsetof(__typeof__(svc), task_free) == 8 && offsetof(__typeof__(svc), scope_free) == 16 && offsetof(__typeof__(svc), tsan) == 24 &&
                   offsetof(__typeof__(svc), stats) == 32 && offsetof(__typeof__(svc), once) == 40 && offsetof(__typeof__(svc), mutex_free) == 48 && offsetof(__typeof__(svc), chan_free) == 56 && offsetof(__typeof__(svc), select_free) == 64 && offsetof(__typeof__(svc), atomic_free) == 72 && offsetof(__typeof__(svc), fibers) == 80 && offsetof(__typeof__(svc), worker_main) == 96 && offsetof(__typeof__(svc), h2_pool) == 104 && offsetof(__typeof__(svc), fs_file_free) == 128 && offsetof(__typeof__(svc), fs_lock_free) == 136 && offsetof(__typeof__(svc), consts) == 144,
               "ctx.xo SV_*");



// The deadlock check is pool.xo dl_check; the test world's clock move
// (tw_kick) checks again after it.
void xo_x_dl_check(int64_t p);
XO_NORETURN void xo_x_deadlock(void);

static void tw_kick(void);

static void tsan_worker(xo_worker *w);
static void sched_hooks(xo_worker *w);

// root_worker returns the record of the calling root thread, made on
// first use: its spawns go to its deque, and while it waits it runs work
// (decision 0077).
static xo_worker *root_worker(void) { return (xo_worker *)(intptr_t)xo_x_pool_root_worker(XO_ADDR(&pool)); }

// park_wait_n blocks the current context until its park is signaled or,
// when cancelable, its node is canceled. A task on a fiber parks: its
// worker runs other work meanwhile (decision 0069). A root thread runs
// queued work while it waits, and sleeps only when it finds none
// (decision 0077). counted tells that the wait counts as blocked for
// deadlock detection; a wait that a timer, the network, or a signal can
// end does not.
void xo_x_park_wait(int64_t pl, int64_t c, int64_t cancelable, int64_t counted, int64_t f, int64_t rw);

// wait_where finds the fiber the current context runs on, or the record
// of its root thread when that helps while it waits (a root helps once
// the pool exists: a program that only sleeps stays single threaded).
static void wait_where(int64_t *f, int64_t *rw) {
	*f = XO_ADDR(fib_get());
	*rw = *f || !__atomic_load_n(&pool.ncpu, __ATOMIC_ACQUIRE) ? 0 : XO_ADDR(root_worker());
}

static void park_wait_n(xo_ctx *c, int cancelable, int counted) {
	flush_pending();
	bud_pause(c); // waiting is not running time (decision 0175)
	int64_t f, rw;
	wait_where(&f, &rw);
	// in the test world nothing outside the program ends a wait
	xo_x_park_wait(XO_ADDR(&pool), XO_ADDR(c), cancelable, counted || tw.virt, f, rw);
	TSAN_ACQUIRE(&c->park);
	bud_resume(c);
}

static void park_wait(xo_ctx *c, int cancelable) { park_wait_n(c, cancelable, 1); }

// A wait list: the parks of the contexts waiting on an object, signaled
// all at once when its state changes (they check again). The entry
// (xo_went, above) is the waiting context's own (xo_ctx.went).

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
	xo_went *e = &c->went;
	*e = (xo_went){.park = &c->park, .writer = writer};
	park_reset(&c->park);
	wl_add(l, e);
	unlk(mu);
	park_wait(c, cancelable);
	lk(mu);
	wl_del(l, e);
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
	// task.with_budget (decision 0175): the budget of the body, a
	// reference until xo_budget_end
	struct xo_budget *bud;
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
	int64_t gone; // atomic: 2 when its run and its queue entry are over (scope_prune)
	// the fault that ended it (decision 0085), malloc'd (last: the fields
	// above keep their places)
	char *fault_msg, *fault_loc, *fault_detail;
	// the budget it was started under (a reference until it runs,
	// decision 0175)
	struct xo_budget *bud;
} xo_task;

enum { T_QUEUED, T_RUNNING, T_DONE };

// xo_scope_group is `s.group()` (tasks.xo scope_group).
int64_t xo_x_scope_group(int64_t s);
xo_scope *xo_scope_group(xo_scope *s) { return (xo_scope *)(intptr_t)xo_x_scope_group(XO_ADDR(s)); }

// The program's task group (os.tasks) and s.cancel() are tasks.xo's
// (B18), which generated code calls through its exports; these forward
// for older xo's code.
void xo_x_prog_group_set(int64_t s);
int64_t xo_x_prog_group(void);
void xo_x_scope_cancel(int64_t s);

void xo_prog_group_set(xo_scope *s) { xo_x_prog_group_set(XO_ADDR(s)); }

void xo_prog_group_clear(void) { xo_x_prog_group_set(0); }

xo_scope *xo_prog_group(void) { return (xo_scope *)(intptr_t)xo_x_prog_group(); }

void xo_scope_cancel(xo_scope *s) { xo_x_scope_cancel(XO_ADDR(s)); }

_Static_assert(offsetof(xo_scope, tasks) == 72 && offsetof(xo_scope, old) == 80 && offsetof(xo_scope, nold) == 88 && sizeof(xo_node) == 56,
               "tasks.xo S_TASKS, S_OLD, S_NOLD, the node's size");

// The task states in Xo (decision 0212, std/native/llrt/sched.xo).
int64_t xo_x_task_claim(int64_t t);
int64_t xo_x_task_dequeued(int64_t t, int64_t s);

void xo_x_run_claimed(int64_t t, int64_t units);

// run_claimed runs t, which the caller claimed, on this stack (a fiber,
// or a thread that helps): tasks.xo run_claimed (B18), which calls
// run_entry for the entry function itself.
static void run_claimed(xo_task *t, int64_t units) { xo_x_run_claimed(XO_ADDR(t), units); }

// run_entry calls t's entry function on the current context (tasks.xo
// run_claimed made it): the entry function catches a fault in the task,
// and the unwinding never goes past this frame (decision 0085).
__attribute__((noinline)) static void run_entry(xo_task *t, xo_rlist *fl) {
	cur_get()->boundary = (uintptr_t)__builtin_frame_address(0);
	t->run(fl, t);
}

// dequeued records that a worker took t's queue entry (and ran it, or
// found it run by a helper). The scope's join waits for this too, so no
// queue holds a task the scope frees.
static void dequeued(xo_task *t) {
	xo_scope *s = t->scope;
	if (xo_x_task_dequeued(XO_ADDR(t), XO_ADDR(s))) {
		park_signal(s->joiner);
	}
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
	int64_t done; // read by sched.xo (decision 0212)
	void *tsan;
	struct xo_fiber *next; // the cache
	void *asan_fake;       // AddressSanitizer: its fake stack while switched out
	int64_t hooks;         // its worker's hooks (sched.xo, decision 0212)
} xo_fiber;

// xo_fiber_switch saves the callee saved registers on the current stack,
// stores the stack pointer in *save, and continues on the stack to (saved
// by an earlier switch, or made by fiber_new). xo_fiber_start is where a
// new fiber starts: it calls the function in the second register slot
// with the first as its argument.
//
// Decision 0212: the compiler emits these sequences in the program's own
// code (rt.stack_switch and rt.fiber_start_addr: xo_stack_switch and
// xo_fiber_entry, from selfhost/arm64 sched.xo, selfhost/llvm sched.xo,
// and their Go mirrors), and the fiber code that uses them is Xo
// (std/native/llrt/sched.xo).

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

#if defined(__has_attribute)
#if __has_attribute(disable_sanitizer_instrumentation)
#define XO_NO_SANITIZE __attribute__((disable_sanitizer_instrumentation))
#endif
#endif
#ifndef XO_NO_SANITIZE
#define XO_NO_SANITIZE
#endif

// sched_hooks tells whether the fibers w runs call xo_sched_hook around
// their switches (decision 0212): in sanitizer builds, and with the
// statistics on (XO_RTSTATS).
static void sched_hooks(xo_worker *w) {
#if defined(XO_TSAN) || defined(XO_ASAN)
	w->hooks = 1;
#else
	w->hooks = __atomic_load_n(&rtstats, __ATOMIC_RELAXED) > 0;
#endif
}

#ifdef MAP_NORESERVE
#define XO_MAP_NORESERVE MAP_NORESERVE
#else
#define XO_MAP_NORESERVE 0
#endif

#define XO_FIBER_CACHE 64

// the part of a fiber's stack committed up front on Windows
#define XO_WIN_FIBER_COMMIT ((size_t)64 << 10)

// The fiber cache's record (words, in the layout std/native/llrt/pool.xo
// reads: decision 0212 stage 2, asserted below).
static struct {
	xo_lock mu;
	xo_fiber *list;
	int64_t n;
	int64_t stack; // bytes, without the guard page
	int64_t page;  // the page size (the guard page)
	int64_t main;  // where a fiber starts (pool.xo: xo_x_fiber_main)
} fibers;

// The fiber stacks' settings are poolsetup.xo's fibers_setup (B20),
// through the services record (the fiber cache's record and the entry
// points, set here at start).
void xo_x_fiber_main(int64_t f);
static int64_t svc_addr(void) {
	svc.fibers = &fibers;
	svc.fiber_main = XO_ADDR(xo_x_fiber_main);
	svc.worker_main = XO_ADDR(xo_x_worker_main);
	return XO_ADDR(&svc);
}

// fiber_counted counts a fiber stack mapped (d 1) or unmapped (-1), and
// the most at once with the statistics on.
static void fiber_counted(int64_t d) {
	int64_t live = __atomic_add_fetch(&st.fiber_live, d, __ATOMIC_RELAXED);
	if (d > 0 && __builtin_expect(__atomic_load_n(&rtstats, __ATOMIC_RELAXED) > 0, 0)) {
		int64_t peak = __atomic_load_n(&st.fiber_peak, __ATOMIC_RELAXED);
		while (live > peak && !__atomic_compare_exchange_n(&st.fiber_peak, &peak, live, 1, __ATOMIC_RELAXED, __ATOMIC_RELAXED)) {
		}
	}
}

// fiber_map_failed reports that a fiber stack's mapping failed with
// errno en (0: unknown), and exits: out of address space more often than
// of memory (stacks are reserved, not committed), so it says how much the
// stacks took (gotchas/linux-tsan-fiber-stacks.md).
XO_NORETURN static void fiber_map_failed(int en) {
#ifndef _WIN32
	fprintf(stderr, "xo: fiber stack mmap failed (%s) with %lld stacks of %lld MiB mapped\n", strerror(en),
	        (long long)__atomic_load_n(&st.fiber_live, __ATOMIC_RELAXED), (long long)(fibers.stack >> 20));
#else
	(void)en;
#endif
	xo_oom();
}

// The hooks the Xo fiber code calls around its switches when the
// worker's hooks word is set (sched_hooks): the sanitizers' fiber calls
// and the statistics. kind (sched.xo HOOK_*): 0 a fiber starts (on its
// stack), 1 it ends, 2 it parks, 3 it was resumed after parking, 4 a
// worker switches to it, 5 the worker is back. Not instrumented by the
// sanitizers: its __tsan_switch_to_fiber changes ThreadSanitizer's
// shadow stack, so an instrumented function's exit here would pop the
// other fiber's entry (gotchas/tsan-fiber-switch-in-callee.md); the
// callers' entries and exits pair up.
XO_NO_SANITIZE void xo_sched_hook(int64_t kind, int64_t wa, int64_t fa) {
	xo_worker *w = (xo_worker *)(intptr_t)wa;
	xo_fiber *f = (xo_fiber *)(intptr_t)fa;
	(void)w, (void)f;
	switch (kind) {
	case 0:
		ASAN_FINISH(NULL, &w->asan_bottom, &w->asan_size);
		break;
	case 1:
		TSAN_SWITCH(w->tsan);
		ASAN_START(NULL, w->asan_bottom, w->asan_size); // for good
		break;
	case 2:
		STAT(fiber_park);
		TSAN_SWITCH(w->tsan);
		ASAN_START(&f->asan_fake, w->asan_bottom, w->asan_size);
		break;
	case 3:
		ASAN_FINISH(f->asan_fake, &w->asan_bottom, &w->asan_size);
		break;
	case 4:
		TSAN_SWITCH(f->tsan);
		ASAN_START(&w->asan_fake, f->mem + (f->size - fibers.stack), fibers.stack);
		break;
	case 5:
		ASAN_FINISH(w->asan_fake, NULL, NULL);
		break;
	}
}

// xo_sched_run_task runs the task of the fiber that just started (Xo's
// fiber_main): it came from a queue, so its entry and its run end here.
void xo_sched_run_task(int64_t fa) {
	xo_task *t = ((xo_fiber *)(intptr_t)fa)->task;
	TSAN_ACQUIRE(t); // its spawn released it (the queues are Xo, not instrumented)
	run_claimed(t, 4);
}

// xo_sched_rt is the C runtime's services for std/native/llrt/pool.xo
// (rt.sched_rt, decision 0212 stage 2): op is pool.xo's RT_*.
static int64_t select_timer_op(int64_t s);
static void select_untimer_op(int64_t t);

int64_t xo_sched_rt(int64_t op, int64_t a, int64_t b) {
	switch (op) {
	case 0: // a statistic: st's counter a
		if (__builtin_expect(__atomic_load_n(&rtstats, __ATOMIC_RELAXED) > 0, 0)) {
			__atomic_fetch_add(&((int64_t *)&st)[a], 1, __ATOMIC_RELAXED);
		}
		return 0;
	case 2:
		tw_kick();
		return 0;
	case 4: { // fiber a got a new stack
		xo_fiber *f = (xo_fiber *)(intptr_t)a;
		(void)f;
		fiber_counted(1);
#ifdef XO_TSAN
		f->tsan = __tsan_create_fiber(0);
#endif
		return 0;
	}
	case 5: { // fiber a's stack is unmapped
		xo_fiber *f = (xo_fiber *)(intptr_t)a;
		(void)f;
#ifdef XO_TSAN
		__tsan_destroy_fiber(f->tsan);
#endif
		fiber_counted(-1);
		return 0;
	}
	case 6:
		fiber_map_failed((int)a);
	case 7: { // worker thread a starts
		xo_worker *w = (xo_worker *)(intptr_t)a;
		tsan_worker(w);
		sched_hooks(w);
		return 0;
	}
	case 8: { // a thread running the C function a with b (the C library's)
		pthread_attr_t attr;
		pthread_attr_init(&attr);
		pthread_attr_setstacksize(&attr, (size_t)64 << 20);
		pthread_attr_setdetachstate(&attr, PTHREAD_CREATE_DETACHED);
		pthread_t th;
		int ok = pthread_create(&th, &attr, (void *(*)(void *))(intptr_t)a, (void *)(intptr_t)b) == 0;
		pthread_attr_destroy(&attr);
		return ok;
	}
	case 9:
		xo_oom();
	case 10: // a worker took the queue entry of task a, which a helper ran
		dequeued((xo_task *)(intptr_t)a);
		return 0;
#ifndef _WIN32
	case 11: { // a fiber stack of a bytes, a guard page of b, from the C library's mmap
		void *m = mmap(NULL, (size_t)a, PROT_READ | PROT_WRITE, MAP_PRIVATE | MAP_ANON | XO_MAP_NORESERVE, -1, 0);
		if (m == MAP_FAILED) {
			fiber_map_failed(errno);
		}
		mprotect(m, (size_t)b, PROT_NONE);
		return XO_ADDR(m);
	}
	case 12:
		munmap((void *)(intptr_t)a, (size_t)b);
		return 0;
#endif
	case 16: // task a, claimed, runs here (its queue entry still queued)
		TSAN_ACQUIRE((void *)(intptr_t)a);
		run_claimed((xo_task *)(intptr_t)a, b);
		return 0;
	case 17: // context a waits on its park (b bit 0: cancelable, bit 1: not counted as blocked)
		park_wait_n((xo_ctx *)(intptr_t)a, (int)(b & 1), !(b & 2));
		return 0;
	case 30: // the test world's record (pool.xo TW_*)
		return XO_ADDR(&tw);
	case 31: // the program is deadlocked (pool.xo dl_check)
		xo_x_deadlock();
	case 29: // task a's entry function, with closure b (tasks.xo run_claimed)
		run_entry((xo_task *)(intptr_t)a, (xo_rlist *)(intptr_t)b);
		return 0;
	case 18: // the list a canceled operation returns
		return XO_ADDR(&empty_list);
	case 19: // drop a reference to handle a (h_release)
		h_release((void *)(intptr_t)a);
		return 0;
	case 20: // the runtime switches to several threads (pool.xo's setup)
		xo_rt_threads_begin();
		return 0;
	case 27: // select a's `after` arm: its timer (selects.xo wait)
		return select_timer_op(a);
	case 28: // timer a of a select's `after` arm goes
		select_untimer_op(a);
		return 0;
	case 22:
		TSAN_RELEASE((void *)(intptr_t)a);
		return 0;
	case 23:
		TSAN_ACQUIRE((void *)(intptr_t)a);
		return 0;
	case 24: { // scope a closes: its deadline's timer goes
		xo_scope *s = (xo_scope *)(intptr_t)a;
		timer_del(s->deadline);
		free(s->deadline);
		s->deadline = NULL;
		return 0;
	}
	case 25: // free handle a, whose last reference went (tasks.xo h_release)
		xo_h_free((void *)(intptr_t)a);
		return 0;
	}
	(void)b;
	return 0;
}

// The layout std/native/llrt/sched.xo reads and writes (decision 0212:
// its constants are these offsets).
_Static_assert(offsetof(xo_runq, head) == 0 && offsetof(xo_runq, tail) == 128 && offsetof(xo_runq, slot) == 136, "sched.xo RQ_*");
_Static_assert(offsetof(xo_worker, q) == 0 && offsetof(xo_worker, sched_sp) == 2208 && offsetof(xo_worker, hooks) == 2216, "sched.xo W_*");
_Static_assert(offsetof(xo_fiber, sp) == 0 && offsetof(xo_fiber, task) == 24 && offsetof(xo_fiber, cur) == 32 && offsetof(xo_fiber, state) == 40 && offsetof(xo_fiber, done) == 48 && offsetof(xo_fiber, hooks) == 80, "sched.xo F_*");
_Static_assert(offsetof(xo_task, state) == 80 && offsetof(xo_task, gone) == 176, "sched.xo T_*");
_Static_assert(offsetof(xo_scope, pending) == 240, "sched.xo S_PENDING");
_Static_assert(offsetof(xo_scope, body) == 16 && offsetof(xo_scope, bud) == 456, "budget.xo S_BODY, S_BUD");
_Static_assert(F_RUNNING == 0 && F_PARKING == 1 && F_PARKED == 2 && F_WOKEN == 3 && T_QUEUED == 0 && T_RUNNING == 1, "sched.xo states");
// and std/native/llrt/pool.xo (decision 0212 stage 2)
_Static_assert(sizeof(xo_lock) == 8, "pool.xo's locks are words");
_Static_assert(offsetof(xo_park, signaled) == 8 && offsetof(xo_park, sleeping) == 16 && offsetof(xo_park, counted) == 20 && offsetof(xo_park, fiber) == 24, "pool.xo PK_*");
_Static_assert(offsetof(xo_node, parent) == 0 && offsetof(xo_node, child) == 8 && offsetof(xo_node, prev) == 16 && offsetof(xo_node, next) == 24 &&
                   offsetof(xo_node, canceled) == 32 && offsetof(xo_node, waiter) == 40 && offsetof(xo_node, mu) == 48,
               "pool.xo N_*");
_Static_assert(offsetof(xo_ctx, node) == 0 && offsetof(xo_ctx, shield) == 12 && offsetof(xo_ctx, park) == 16, "pool.xo C_*");
_Static_assert(offsetof(xo_scope, tasks) == 72 && offsetof(xo_scope, mu) == 104 && offsetof(xo_scope, joiner) == 248 && offsetof(xo_scope, list) == 384 &&
                   offsetof(xo_scope, closed) == 400 && offsetof(xo_scope, nlist) == 432 && offsetof(xo_scope, prune_at) == 440 && offsetof(xo_scope, closing) == 448,
               "pool.xo S_*");
_Static_assert(offsetof(xo_task, h) == 0 && offsetof(xo_task, node) == 16 && offsetof(xo_task, scope) == 72 && offsetof(xo_task, snext) == 168 && T_DONE == 2, "pool.xo T_*");
_Static_assert(offsetof(__typeof__(tw), vpending) == 16 && offsetof(__typeof__(tw), vepoch) == 24 && offsetof(__typeof__(tw), virt) == 40, "pool.xo TW_*");
_Static_assert(offsetof(__typeof__(dlk), blocked) == 8 && offsetof(__typeof__(dlk), uncounted) == 16 && offsetof(__typeof__(pool), dlk) == 120, "pool.xo DLK_*");
_Static_assert(offsetof(xo_worker, next) == 2184 && offsetof(xo_worker, next_at) == 2192 && offsetof(xo_worker, live) == 2200 && offsetof(xo_worker, wakew) == 2232 &&
                   offsetof(xo_worker, idle_next) == 2240 && offsetof(xo_worker, rng) == 2248 && offsetof(xo_worker, nonext) == 2256 &&
                   offsetof(xo_worker, fcache) == 2264 && offsetof(xo_worker, nfcache) == 2328 && offsetof(xo_worker, pool) == 2360 &&
                   offsetof(xo_worker, batch) == 2368 && sizeof(xo_worker) == 3400,
               "pool.xo W_*");
_Static_assert(offsetof(__typeof__(pool), mu) == 0 && offsetof(__typeof__(pool), ncpu) == 8 && offsetof(__typeof__(pool), spin_rounds) == 16 &&
                   offsetof(__typeof__(pool), nw) == 24 && offsetof(__typeof__(pool), alive) == 32 && offsetof(__typeof__(pool), nidle) == 40 &&
                   offsetof(__typeof__(pool), spinning) == 48 && offsetof(__typeof__(pool), idle) == 56 && offsetof(__typeof__(pool), imu) == 64 &&
                   offsetof(__typeof__(pool), ibuf) == 72 && offsetof(__typeof__(pool), ihead) == 80 && offsetof(__typeof__(pool), ilen) == 88 &&
                   offsetof(__typeof__(pool), icap) == 96 && offsetof(__typeof__(pool), fibers) == 104 && offsetof(__typeof__(pool), worker_main) == 112 &&
                   offsetof(__typeof__(pool), ws) == 128 && XO_MAXW == 4096,
               "pool.xo P_*");
_Static_assert(offsetof(__typeof__(fibers), mu) == 0 && offsetof(__typeof__(fibers), list) == 8 && offsetof(__typeof__(fibers), n) == 16 &&
                   offsetof(__typeof__(fibers), stack) == 24 && offsetof(__typeof__(fibers), page) == 32 && offsetof(__typeof__(fibers), main) == 40,
               "pool.xo FS_*");
_Static_assert(offsetof(xo_fiber, mem) == 8 && offsetof(xo_fiber, size) == 16 && offsetof(xo_fiber, next) == 64 && sizeof(xo_fiber) == 88 &&
                   XO_FCACHE == 8 && XO_FIBER_CACHE == 64 && XO_WIN_FIBER_COMMIT == 65536,
               "pool.xo F_*");

// The spawn and the scope's open and close are tasks.xo's (B18 part 2),
// which generated code calls through its exports; these forward for
// older xo's code.
int64_t xo_x_task_spawn(int64_t s, int64_t fl, int64_t run, int64_t drop, int64_t fdrop, int64_t flags, int64_t site, int64_t lp, int64_t ln);
int64_t xo_x_scope_open(void);
int64_t xo_x_scope_close(int64_t s, int64_t mode);

xo_task *xo_task_spawn(xo_scope *s, xo_rlist *fl, int64_t run, int64_t drop, int64_t fdrop, int64_t flags, int64_t site, const char *loc, int64_t n) {
	return (xo_task *)(intptr_t)xo_x_task_spawn(XO_ADDR(s), XO_ADDR(fl), run, drop, fdrop, flags, site, XO_ADDR(loc), n);
}

xo_scope *xo_scope_open(void) { return (xo_scope *)(intptr_t)xo_x_scope_open(); }

int64_t xo_scope_close(xo_scope *s, int64_t mode) { return xo_x_scope_close(XO_ADDR(s), mode); }

_Static_assert(offsetof(xo_task, run) == 88 && offsetof(xo_task, drop) == 96 && offsetof(xo_task, fl) == 104 &&
                   offsetof(xo_task, background) == 124 && offsetof(xo_task, snext) == 168 && offsetof(xo_task, bud) == 208 &&
                   sizeof(xo_task) == 216 && offsetof(xo_hdr, free) == 8,
               "tasks.xo T_*, H_FREE");
_Static_assert(offsetof(xo_scope, parent_node) == 96 && offsetof(xo_scope, list) == 384 && offsetof(xo_scope, closed) == 400 &&
                   offsetof(xo_scope, deadline) == 408 && offsetof(xo_scope, prune_at) == 440 && offsetof(xo_scope, closing) == 448 &&
                   sizeof(xo_scope) == 464 && offsetof(__typeof__(pool), ncpu) == 8,
               "tasks.xo S_*, P_NCPU");

// The task's end, await, and the scope's failed task are tasks.xo's
// (B18), which generated code calls through its exports; these forward
// to them for older xo's code.
void xo_x_task_done(int64_t t, int64_t rl, int64_t failed);
void xo_x_task_unwound(int64_t t);
int64_t xo_x_task_await(int64_t t, int64_t lp, int64_t ln);
int64_t xo_x_task_result(int64_t t);
int64_t xo_x_scope_fail_task(int64_t s);
int64_t xo_x_scope_fail_site(int64_t s);
int64_t xo_x_scope_group(int64_t s);
void xo_x_fault_rethrow(int64_t s);

void xo_task_done(xo_task *t, xo_rlist *rl, int64_t failed) { xo_x_task_done(XO_ADDR(t), XO_ADDR(rl), failed); }

void xo_task_unwound(xo_task *t) { xo_x_task_unwound(XO_ADDR(t)); }

XO_NORETURN void xo_fault_rethrow(xo_scope *s) {
	xo_x_fault_rethrow(XO_ADDR(s));
	__builtin_unreachable();
}

xo_rlist *xo_task_await(xo_task *t, const char *loc, int64_t n) { return (xo_rlist *)(intptr_t)xo_x_task_await(XO_ADDR(t), XO_ADDR(loc), n); }

xo_rlist *xo_task_result(xo_task *t) { return (xo_rlist *)(intptr_t)xo_x_task_result(XO_ADDR(t)); }

xo_task *xo_scope_fail_task(xo_scope *s) { return (xo_task *)(intptr_t)xo_x_scope_fail_task(XO_ADDR(s)); }

int64_t xo_scope_fail_site(xo_scope *s) { return xo_x_scope_fail_site(XO_ADDR(s)); }

_Static_assert(offsetof(xo_task, scope) == 72 && offsetof(xo_task, result) == 112 && offsetof(xo_task, unwound) == 120 &&
                   offsetof(xo_task, consumed) == 128 && offsetof(xo_task, site) == 136 && offsetof(xo_task, mu) == 144 &&
                   offsetof(xo_task, waiters) == 152 && offsetof(xo_task, fault_msg) == 184 && offsetof(xo_task, fault_loc) == 192 &&
                   offsetof(xo_task, fault_detail) == 200,
               "tasks.xo T_*");
_Static_assert(offsetof(xo_scope, mu) == 104 && offsetof(xo_scope, fail) == 392 && offsetof(xo_scope, fault) == 424, "tasks.xo S_*");


// ---------------------------------------------------------------------------
// Channels (core 7.3)

// The queue entry (xo_cent) is above, with xo_ctx: a waiting context's
// is its own (xo_ctx.cent), a select's arms' the select's.

typedef struct {
	xo_cent *head, *tail;
} xo_cq;

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

// The channel operations are chan.xo's (B19), which generated code calls
// through its exports; these forward for older xo's code. chan_free stays
// C (the free function the handle names).
int64_t xo_x_chan_new(int64_t cap, int64_t drop, int64_t lp, int64_t ln);
void xo_x_chan_send(int64_t c, int64_t l, int64_t lp, int64_t ln);
int64_t xo_x_chan_recv(int64_t c);
int64_t xo_x_chan_try_send(int64_t c, int64_t l);
int64_t xo_x_chan_try_recv(int64_t c);
void xo_x_chan_close(int64_t c);
int64_t xo_x_chan_len(int64_t c);

xo_chan *xo_chan_new(int64_t cap, int64_t drop, const char *loc, int64_t n) { return (xo_chan *)(intptr_t)xo_x_chan_new(cap, drop, XO_ADDR(loc), n); }

void xo_chan_send(xo_chan *c, xo_rlist *l, const char *loc, int64_t n) { xo_x_chan_send(XO_ADDR(c), XO_ADDR(l), XO_ADDR(loc), n); }

xo_rlist *xo_chan_recv(xo_chan *c) { return (xo_rlist *)(intptr_t)xo_x_chan_recv(XO_ADDR(c)); }

int64_t xo_chan_try_send(xo_chan *c, xo_rlist *l) { return xo_x_chan_try_send(XO_ADDR(c), XO_ADDR(l)); }

xo_rlist *xo_chan_try_recv(xo_chan *c) { return (xo_rlist *)(intptr_t)xo_x_chan_try_recv(XO_ADDR(c)); }

void xo_chan_close(xo_chan *c) { xo_x_chan_close(XO_ADDR(c)); }

int64_t xo_chan_len(xo_chan *c) { return xo_x_chan_len(XO_ADDR(c)); }

_Static_assert(offsetof(xo_cent, fired) == 8 && offsetof(xo_cent, item) == 16 && offsetof(xo_cent, shut) == 24 && offsetof(xo_cent, idx) == 32 &&
                   offsetof(xo_cent, chosen) == 40 && offsetof(xo_cent, prev) == 48 && offsetof(xo_cent, next) == 56 && sizeof(xo_cent) == 64,
               "chan.xo E_*");
_Static_assert(offsetof(xo_chan, mu) == 16 && offsetof(xo_chan, cap) == 24 && offsetof(xo_chan, head) == 32 && offsetof(xo_chan, len) == 40 &&
                   offsetof(xo_chan, buf) == 48 && offsetof(xo_chan, closed) == 56 && offsetof(xo_chan, ext) == 60 && offsetof(xo_chan, drop) == 64 &&
                   offsetof(xo_chan, recvq) == 72 && offsetof(xo_chan, sendq) == 88 && sizeof(xo_chan) == 104,
               "chan.xo CH_*");

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

// The select's operations are selects.xo's (B19), which generated code
// calls through its exports; these forward for older xo's code.
// select_free stays C (the free function the handle names), and the
// `after` arm's timer (select_timer, select_untimer: rt.sched_rt 27, 28).
int64_t xo_x_select_new(void);
void xo_x_select_recv(int64_t s, int64_t c);
void xo_x_select_send(int64_t s, int64_t c, int64_t l);
int64_t xo_x_select_wait(int64_t s, int64_t lp, int64_t ln);
int64_t xo_x_select_take(int64_t s, int64_t i);

xo_select *xo_select_new(void) { return (xo_select *)(intptr_t)xo_x_select_new(); }

void xo_select_recv(xo_select *s, xo_chan *c) { xo_x_select_recv(XO_ADDR(s), XO_ADDR(c)); }

void xo_select_send(xo_select *s, xo_chan *c, xo_rlist *l) { xo_x_select_send(XO_ADDR(s), XO_ADDR(c), XO_ADDR(l)); }

int64_t xo_select_wait(xo_select *s, const char *loc, int64_t n) { return xo_x_select_wait(XO_ADDR(s), XO_ADDR(loc), n); }

xo_rlist *xo_select_take(xo_select *s, int64_t i) { return (xo_rlist *)(intptr_t)xo_x_select_take(XO_ADDR(s), i); }

_Static_assert(offsetof(xo_sarm, send) == 8 && offsetof(xo_sarm, item) == 16 && offsetof(xo_sarm, e) == 24 && sizeof(xo_sarm) == 88 &&
                   offsetof(xo_select, arms) == 16 && offsetof(xo_select, n) == 24 && offsetof(xo_select, cap) == 28 &&
                   offsetof(xo_select, fired) == 32 && offsetof(xo_select, chosen) == 40 && offsetof(xo_select, has_after) == 48 &&
                   offsetof(xo_select, after) == 56 && offsetof(xo_select, park) == 64 && sizeof(xo_select) == 72,
               "selects.xo A_*, SE_*");

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

// The Mutex's operations are mutex.xo's (B19), which generated code calls
// through its exports; these forward for older xo's code. mutex_free
// stays C (the free function the handle names, through the services
// record).
int64_t xo_x_mutex_new(int64_t l, int64_t drop);
int64_t xo_x_mutex_lock(int64_t m, int64_t lp, int64_t ln);
void xo_x_mutex_unlock(int64_t m, int64_t l);
int64_t xo_x_mutex_rlock(int64_t m, int64_t lp, int64_t ln);
void xo_x_mutex_runlock(int64_t m, int64_t l);

xo_mutex *xo_mutex_new(xo_rlist *l, int64_t drop) { return (xo_mutex *)(intptr_t)xo_x_mutex_new(XO_ADDR(l), drop); }

xo_rlist *xo_mutex_lock(xo_mutex *m, const char *loc, int64_t n) { return (xo_rlist *)(intptr_t)xo_x_mutex_lock(XO_ADDR(m), XO_ADDR(loc), n); }

void xo_mutex_unlock(xo_mutex *m, xo_rlist *l) { xo_x_mutex_unlock(XO_ADDR(m), XO_ADDR(l)); }

xo_rlist *xo_mutex_rlock(xo_mutex *m, const char *loc, int64_t n) { return (xo_rlist *)(intptr_t)xo_x_mutex_rlock(XO_ADDR(m), XO_ADDR(loc), n); }

void xo_mutex_runlock(xo_mutex *m, xo_rlist *l) { xo_x_mutex_runlock(XO_ADDR(m), XO_ADDR(l)); }

_Static_assert(offsetof(xo_mutex, mu) == 16 && offsetof(xo_mutex, writer) == 24 && offsetof(xo_mutex, readers) == 32 &&
                   offsetof(xo_mutex, wwait) == 40 && offsetof(xo_mutex, rwait) == 48 && offsetof(xo_mutex, slot) == 56 &&
                   offsetof(xo_mutex, drop) == 64 && offsetof(xo_mutex, waiters) == 72 && sizeof(xo_mutex) == 88 &&
                   offsetof(xo_went, granted) == 28,
               "mutex.xo M_*");

// ---------------------------------------------------------------------------
// Atomic[T] for Int and Bool (a Bool is 0 or 1)
//
// The generated code reads and writes the value itself (decision 0200):
// get, set, swap, and compare_and_swap are one inline atomic each on
// byte 128 (slot 13 of rt.slot_load and friends, which count a list's
// three word header), and add and max are std/native/llrt/atomic.xo.
// The C keeps the allocation (decision 0198 deleted the C path).

typedef struct {
	xo_hdr h;
	// the value on a cache line apart from the count, which every closure
	// that captures the Atomic retains and releases (decision 0077)
	char pad[128 - sizeof(xo_hdr)];
	int64_t v;
} xo_atomic;

_Static_assert(__builtin_offsetof(xo_atomic, v) == 24 + 8 * 13, "Atomic's value is slot 13 (decision 0200)");


// xo_atomic_new is tasks.xo atomic_new (B19), which generated code calls
// through its export; this forwards for older xo's code.
int64_t xo_x_atomic_new(int64_t v);
xo_atomic *xo_atomic_new(int64_t v) { return (xo_atomic *)(intptr_t)xo_x_atomic_new(v); }

_Static_assert(sizeof(xo_atomic) == 136, "tasks.xo atomic_new");

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

#if defined(_WIN32)
// Windows (decision 0197): the poller waits on an I/O completion port.
// Socket readiness comes from AFD polls (IOCTL_AFD_POLL on \Device\Afd,
// the driver under Winsock's select and WSAPoll, as wepoll and mio use
// it): a waiter arms a one shot poll of its socket for the directions
// waited in, and its completion hands the readiness over (stage 5). When
// the AFD device cannot be opened the poller falls back to WSAPoll over
// every descriptor someone waits on (stage 1), with a loopback socket
// pair as the self pipe; XO_WINPOLL=wsapoll forces that (to compare).
#define XO_WINPOLL 1
#include <winternl.h>
#pragma comment(lib, "ntdll")
typedef struct {
	HANDLE handle;
	ULONG events;
	NTSTATUS status;
} xo_afd_handle;
typedef struct {
	LARGE_INTEGER timeout;
	ULONG n;
	ULONG exclusive;
	xo_afd_handle h[1];
} xo_afd_poll;
#define XO_IOCTL_AFD_POLL 0x00012024
#define XO_AFD_RECEIVE 0x0001
#define XO_AFD_RECEIVE_EXPEDITED 0x0002
#define XO_AFD_SEND 0x0004
#define XO_AFD_DISCONNECT 0x0008
#define XO_AFD_ABORT 0x0010
#define XO_AFD_LOCAL_CLOSE 0x0020
#define XO_AFD_ACCEPT 0x0080
#define XO_AFD_CONNECT_FAIL 0x0100
#define XO_AFD_READ (XO_AFD_RECEIVE | XO_AFD_RECEIVE_EXPEDITED | XO_AFD_DISCONNECT | XO_AFD_ABORT | XO_AFD_LOCAL_CLOSE | XO_AFD_ACCEPT | XO_AFD_CONNECT_FAIL)
#define XO_AFD_WRITE (XO_AFD_SEND | XO_AFD_ABORT | XO_AFD_LOCAL_CLOSE | XO_AFD_CONNECT_FAIL)
#ifndef STATUS_PENDING
#define STATUS_PENDING ((NTSTATUS)0x00000103L)
#endif
#ifndef SIO_BASE_HANDLE
#define SIO_BASE_HANDLE _WSAIORW(IOC_WS2, 34) // mswsock.h
#endif
#elif defined(__APPLE__) || defined(__FreeBSD__)
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
_Static_assert(offsetof(struct xo_timer, when) == 0 && offsetof(struct xo_timer, idx) == 8, "poll.xo's timer offsets");

// The timer heap is std/native/llrt/poll.xo's (zero-C audit B22 stage 1),
// ev.mu held around each call.
int64_t xo_x_poll_timer_push(int64_t t);
void xo_x_poll_timer_remove(int64_t t);
int64_t xo_x_poll_timer_first(void);
#define th_first() ((xo_timer *)(intptr_t)xo_x_poll_timer_first())
#define th_remove(t) xo_x_poll_timer_remove((int64_t)(intptr_t)(t))

typedef struct xo_pd {
	xo_lock mu; // a lock word (std/native/llrt/poll.xo takes it too)
	int fd;
	int closed; // close was called
	int ops;    // operations running (the socket closes when none runs)
	int rready, wready;
	xo_park *rwait, *wwait;
	struct xo_pd *next; // ev.dead
#ifdef XO_WINPOLL
	// AFD polling (pd_arm): the poll's status block and buffer, the
	// socket's base handle, the events of the poll in flight (0 none),
	// whether to poll again once it ends (it was cancelled for more
	// events), and whether the poller frees it once it ends (pd_free ran)
	IO_STATUS_BLOCK iosb;
	xo_afd_poll info;
	SOCKET base;
	ULONG armed;
	int rearm, dying;
#endif
} xo_pd;

static struct {
	pthread_once_t once;
	xo_lock mu; // the timer heap and ev.dead (a lock word: poll.xo takes it too)
	int pfd[2]; // the self pipe
	int poll;   // the kqueue or epoll descriptor
	xo_pd *dead;
	int64_t sigs; // signals not handled yet (atomic; a word: poll.xo swaps it)
#ifdef XO_WINPOLL
	HANDLE iocp, afd; // the completion port and the AFD device (NULL: WSAPoll)
	int wr, ww;     // the self pipe: a connected loopback socket pair
	xo_pd **regs;   // the registered descriptors (ev.mu)
	int nreg, creg; //
	WSAPOLLFD *pfds; // the poller thread's WSAPoll array and its owners
	xo_pd **pwho;
	int cpoll;
#endif
} ev ={.once = PTHREAD_ONCE_INIT};
_Static_assert(offsetof(xo_pd, mu) == 0 && offsetof(xo_pd, fd) == 8 && offsetof(xo_pd, closed) == 12 && offsetof(xo_pd, ops) == 16 &&
                   offsetof(xo_pd, rready) == 20 && offsetof(xo_pd, wready) == 24 && offsetof(xo_pd, rwait) == 32 &&
                   offsetof(xo_pd, wwait) == 40 && offsetof(xo_pd, next) == 48,
               "poll.xo's descriptor offsets");
_Static_assert(offsetof(struct xo_timer, park) == 16 && offsetof(struct xo_timer, fn) == 24 && offsetof(struct xo_timer, fired) == 40,
               "poll.xo's timer offsets");

__attribute__((noinline, unused)) static int64_t x_ret(int64_t r) {
	if (r < 0) {
		int e = errno;
		if (e == EAGAIN || e == EWOULDBLOCK) {
			STAT(net_again);
		}
		return -(int64_t)e;
	}
	return r;
}

// The network's system calls and name resolution from Xo on Linux
// (decision 0206 stage 3): the exports of std/native/llrt/cabi_net.xo
// (over sock.xo and dns.xo), which a program carries when it dials or
// listens or uses std/http. They are weak references, so a program
// without them, or whose net_raw says 0 (XO_LLRT_C=net), calls the C
// library as before; a program linked with no C library (XO_NOLIBC) has
// no C path, and they are strong references there. What a program
// without sockets reaches too (the poller's epoll, its pipe made
// nonblocking, a descriptor's shutdown and close) goes through the
// exports (cabi_poll.xo), which every program has. Each nx_
// function keeps the C library's convention (-1 and errno). macOS takes
// the same path (sock.xo's darwin arms over libSystem, getaddrinfo in
// cabi_net.xo; the zero-C audit's B23), and Windows (its arms over
// ws2_32, B23 stage 2: UNTESTED on windows).
#if defined(__linux__) || defined(__APPLE__) || defined(_WIN32)
#ifdef XO_NOLIBC
#define XO_NX_REF
#define NX_LIBC(call) (void)0
#elif defined(__APPLE__) || defined(_WIN32)
// Mach-O takes no undefined weak reference (and PE/COFF's are weak
// externals with a default): weak definitions below stand in (net_raw
// 0: the C library) until cabi_net.xo's exports replace them
#define XO_NX_REF
#define NX_LIBC(call)                                                                                                         \
	if (!nx_on()) {                                                                                                           \
		return call;                                                                                                          \
	}
#else
#define XO_NX_REF __attribute__((weak))
#define NX_LIBC(call)                                                                                                         \
	if (!nx_on()) {                                                                                                           \
		return call;                                                                                                          \
	}
#endif
XO_NX_REF int64_t xo_x_net_raw(void);
XO_NX_REF int64_t xo_x_net_socket(int64_t family, int64_t ty, int64_t proto);
XO_NX_REF int64_t xo_x_net_bind(int64_t fd, int64_t sa, int64_t n);
XO_NX_REF int64_t xo_x_net_listen(int64_t fd, int64_t backlog);
XO_NX_REF int64_t xo_x_net_accept(int64_t fd, int64_t flags);
XO_NX_REF int64_t xo_x_net_connect(int64_t fd, int64_t sa, int64_t n);
XO_NX_REF int64_t xo_x_net_sock_err(int64_t fd);
XO_NX_REF int64_t xo_x_net_setsockopt(int64_t fd, int64_t level, int64_t opt, int64_t v);
XO_NX_REF int64_t xo_x_net_sockname(int64_t fd, int64_t sa, int64_t lenp);
XO_NX_REF int64_t xo_x_net_peername(int64_t fd, int64_t sa, int64_t lenp);
XO_NX_REF int64_t xo_x_net_read(int64_t fd, int64_t p, int64_t n);
XO_NX_REF int64_t xo_x_net_send(int64_t fd, int64_t p, int64_t n);
XO_NX_REF int64_t xo_x_net_resolve(int64_t hp, int64_t hn, int64_t pp, int64_t pn, int64_t family, int64_t passive, int64_t state,
	int64_t out, int64_t cap);
XO_NX_REF int64_t xo_x_net_ntop(int64_t family, int64_t src, int64_t buf, int64_t bn);
XO_NX_REF int64_t xo_x_net_pton6(int64_t p, int64_t dst);
XO_NX_REF int64_t xo_x_net_gai_text(int64_t code, int64_t buf, int64_t bn);
#if (defined(__APPLE__) || defined(_WIN32)) && !defined(XO_NOLIBC)
#define XO_NX_STUB(name, ...)                                                                                                 \
	__attribute__((weak)) int64_t name(__VA_ARGS__) { return -ENOSYS; }
__attribute__((weak)) int64_t xo_x_net_raw(void) { return 0; }
XO_NX_STUB(xo_x_net_socket, int64_t a, int64_t b, int64_t c)
XO_NX_STUB(xo_x_net_bind, int64_t a, int64_t b, int64_t c)
XO_NX_STUB(xo_x_net_listen, int64_t a, int64_t b)
XO_NX_STUB(xo_x_net_accept, int64_t a, int64_t b)
XO_NX_STUB(xo_x_net_connect, int64_t a, int64_t b, int64_t c)
XO_NX_STUB(xo_x_net_sock_err, int64_t a)
XO_NX_STUB(xo_x_net_setsockopt, int64_t a, int64_t b, int64_t c, int64_t d)
XO_NX_STUB(xo_x_net_sockname, int64_t a, int64_t b, int64_t c)
XO_NX_STUB(xo_x_net_peername, int64_t a, int64_t b, int64_t c)
XO_NX_STUB(xo_x_net_read, int64_t a, int64_t b, int64_t c)
XO_NX_STUB(xo_x_net_send, int64_t a, int64_t b, int64_t c)
XO_NX_STUB(xo_x_net_resolve, int64_t a, int64_t b, int64_t c, int64_t d, int64_t e, int64_t f, int64_t g, int64_t h, int64_t i)
XO_NX_STUB(xo_x_net_ntop, int64_t a, int64_t b, int64_t c, int64_t d)
XO_NX_STUB(xo_x_net_pton6, int64_t a, int64_t b)
XO_NX_STUB(xo_x_net_gai_text, int64_t a, int64_t b, int64_t c)
#endif
int64_t xo_x_fd_close(int64_t fd);
int64_t xo_x_fd_nonblock(int64_t fd);
int64_t xo_x_fd_shutdown(int64_t fd, int64_t how);
int64_t xo_x_poll_create(void);
int64_t xo_x_poll_ctl(int64_t ep, int64_t op, int64_t fd, int64_t ev);
int64_t xo_x_poll_wait(int64_t ep, int64_t evs, int64_t n, int64_t ms);

// nx_on reports whether the network goes through Xo (computed once).
static int nx_on(void) {
#ifdef XO_NOLIBC
	return 1;
#else
	static int on = -1;
	int v = __atomic_load_n(&on, __ATOMIC_RELAXED);
	if (v < 0) {
#if defined(__APPLE__) || defined(_WIN32)
		v = xo_x_net_raw() == 1;
#else
		v = xo_x_net_raw && xo_x_net_raw() == 1;
#endif
		__atomic_store_n(&on, v, __ATOMIC_RELAXED);
	}
	return v;
#endif
}

// nx_ret is a system call's result in the C library's convention.
static int nx_ret(int64_t r) {
	if (r < 0) {
		errno = (int)-r;
		return -1;
	}
	return (int)r;
}

static int nx_socket(int family, int ty, int proto) {
	NX_LIBC(socket(family, ty, proto));
	return nx_ret(xo_x_net_socket(family, ty, proto));
}

static int nx_bind(int fd, const struct sockaddr *sa, socklen_t n) {
	NX_LIBC(bind(fd, sa, n));
	return nx_ret(xo_x_net_bind(fd, XO_ADDR(sa), n));
}

static int nx_listen(int fd, int backlog) {
	NX_LIBC(listen(fd, backlog));
	return nx_ret(xo_x_net_listen(fd, backlog));
}

// nx_setsockopt_int sets the int option opt to v.
static int nx_setsockopt_int(int fd, int level, int opt, int v) {
	NX_LIBC(setsockopt(fd, level, opt, &v, sizeof v));
	return nx_ret(xo_x_net_setsockopt(fd, level, opt, v));
}

static int nx_getsockname(int fd, struct sockaddr *sa, socklen_t *n) {
	NX_LIBC(getsockname(fd, sa, n));
	return nx_ret(xo_x_net_sockname(fd, XO_ADDR(sa), XO_ADDR(n)));
}

static int nx_getpeername(int fd, struct sockaddr *sa, socklen_t *n) {
	NX_LIBC(getpeername(fd, sa, n));
	return nx_ret(xo_x_net_peername(fd, XO_ADDR(sa), XO_ADDR(n)));
}

static int nx_shutdown(int fd, int how) {
	NX_LIBC(shutdown(fd, how));
	return nx_ret(xo_x_fd_shutdown(fd, how));
}

// nx_close closes a socket (or the poller's pipe ends).
static int nx_close(int fd) {
	NX_LIBC(close(fd));
#ifdef _WIN32
	if (!xo_w_is_sock(fd)) {
		// a C runtime descriptor (xo_win.c's tag)
		return close(fd);
	}
#endif
	return nx_ret(xo_x_fd_close(fd));
}

#if defined(__linux__)
static int nx_epoll_create(void) {
	NX_LIBC(epoll_create1(EPOLL_CLOEXEC));
	return nx_ret(xo_x_poll_create());
}

// nx_poll_sync stands for the epoll instance in ThreadSanitizer builds.
// A registration publishes its xo_pd (calloc'd and initialized on the
// registering thread) through the kernel, and the poller reads it from
// the event epoll_wait returns: the kernel orders the two, and the C
// library's epoll_ctl and epoll_wait interceptors tell ThreadSanitizer so
// (a release of the epoll descriptor, an acquire when events return).
// The Xo system calls (cabi_poll.xo, decision 0206 stage 3) bypass those
// interceptors, so without the same pair here it reported pd_new's calloc
// against the poller's lock of the descriptor as a race (Linux,
// 2026-10-13, gotcha tsan-raw-epoll-no-edge).
static char nx_poll_sync;

static int nx_epoll_ctl(int ep, int op, int fd, struct epoll_event *e) {
	NX_LIBC(epoll_ctl(ep, op, fd, e));
	TSAN_RELEASE(&nx_poll_sync);
	return nx_ret(xo_x_poll_ctl(ep, op, fd, XO_ADDR(e)));
}

static int nx_epoll_wait(int ep, struct epoll_event *evs, int n, int ms) {
	NX_LIBC(epoll_wait(ep, evs, n, ms));
	int r = nx_ret(xo_x_poll_wait(ep, XO_ADDR(evs), n, ms));
	if (r > 0) {
		TSAN_ACQUIRE(&nx_poll_sync);
	}
	return r;
}
#endif

// nx_getaddrinfo is getaddrinfo(host, port, hints) for the runtime's
// hints (a numeric port, AI_PASSIVE or not, AF_UNSPEC or one family):
// dns.xo's lookup, at most 32 addresses in one block that
// nx_freeaddrinfo frees. nx_dns is the resolver's state: resolv.conf's
// rotate counter and the order of nsswitch.conf with its file's key.
static int nx_getaddrinfo(const char *host, const char *port, const struct addrinfo *hints, struct addrinfo **res) {
	NX_LIBC(getaddrinfo(host, port, hints, res));
	enum { NX_MAX = 32, NX_SA = 32 };
	static int64_t nx_dns[2];
	unsigned char buf[NX_MAX * NX_SA];
	int64_t n = xo_x_net_resolve(XO_ADDR(host ? host : ""), host ? (int64_t)strlen(host) : 0, XO_ADDR(port), (int64_t)strlen(port),
		hints->ai_family, (hints->ai_flags & AI_PASSIVE) ? 1 : 0, XO_ADDR(nx_dns), XO_ADDR(buf), NX_MAX);
	if (n <= 0) {
		return n < 0 ? (int)n : EAI_NONAME;
	}
	struct addrinfo *a = calloc((size_t)n, sizeof(struct addrinfo) + NX_SA);
	if (!a) {
		xo_oom();
	}
	unsigned char *sas = (unsigned char *)(a + n);
	memcpy(sas, buf, (size_t)n * NX_SA);
	for (int64_t i = 0; i < n; i++) {
		struct sockaddr *sa = (struct sockaddr *)(sas + NX_SA * i);
		a[i].ai_family = sa->sa_family;
		a[i].ai_socktype = hints->ai_socktype;
		a[i].ai_protocol = hints->ai_socktype == SOCK_STREAM ? IPPROTO_TCP : 0;
		a[i].ai_addrlen = sa->sa_family == AF_INET ? sizeof(struct sockaddr_in) : sizeof(struct sockaddr_in6);
		a[i].ai_addr = sa;
		a[i].ai_next = i + 1 < n ? &a[i + 1] : NULL;
	}
	*res = a;
	return 0;
}

static void nx_freeaddrinfo(struct addrinfo *a) {
#ifndef XO_NOLIBC
	if (!nx_on()) {
		freeaddrinfo(a);
		return;
	}
#endif
	free(a);
}

// nx_gai_strerror is gai_strerror(g): glibc's texts.
static const char *nx_gai_strerror(int g) {
	NX_LIBC(gai_strerror(g));
	static __thread char buf[64];
	xo_x_net_gai_text(g, XO_ADDR(buf), sizeof buf);
	return buf;
}

// nx_ntop is inet_ntop(family, src, dst, n).
static const char *nx_ntop(int family, const void *src, char *dst, socklen_t n) {
	NX_LIBC(inet_ntop(family, src, dst, n));
	return xo_x_net_ntop(family, XO_ADDR(src), XO_ADDR(dst), n) < 0 ? NULL : dst;
}

// nx_pton6 is inet_pton(AF_INET6, src, dst).
static int nx_pton6(const char *src, void *dst) {
	NX_LIBC(inet_pton(AF_INET6, src, dst));
	return (int)xo_x_net_pton6(XO_ADDR(src), XO_ADDR(dst));
}
#else
#define nx_socket socket
#define nx_bind bind
#define nx_listen listen
#define nx_getsockname getsockname
#define nx_getpeername getpeername
#define nx_shutdown shutdown
#define nx_getaddrinfo getaddrinfo
#define nx_freeaddrinfo freeaddrinfo
#define nx_gai_strerror gai_strerror
#define nx_ntop inet_ntop
#define nx_pton6(src, dst) inet_pton(AF_INET6, src, dst)
#define nx_close close
#define nx_setsockopt_int(fd, level, opt, v)                                                                                  \
	({                                                                                                                        \
		int nx_v_ = (v);                                                                                                      \
		setsockopt((fd), (level), (opt), &nx_v_, sizeof nx_v_);                                                               \
	})
#endif

#ifdef _WIN32
// Winsock reports errors in WSAGetLastError, not errno
__attribute__((noinline)) static int64_t x_wsa(int64_t r) { return r < 0 ? -(int64_t)xo_wsa_errno() : r; }

__attribute__((noinline)) static int64_t x_read(int fd, void *b, size_t n) {
	STAT(net_read);
	if (nx_on()) {
		return xo_x_net_read(fd, XO_ADDR(b), (int64_t)n);
	}
	return x_wsa(recv((SOCKET)fd, b, (int)n, 0));
}

__attribute__((noinline)) static int64_t x_write(int fd, const void *b, size_t n) {
	STAT(net_write);
	if (nx_on()) {
		return xo_x_net_send(fd, XO_ADDR(b), (int64_t)n);
	}
	return x_wsa(send((SOCKET)fd, b, (int)n, 0));
}
#else
// x_nx counts a would block result of the Xo path as x_ret does.
__attribute__((unused)) static int64_t x_nx(int64_t r) {
	if (r == -EAGAIN || r == -EWOULDBLOCK) {
		STAT(net_again);
	}
	return r;
}

__attribute__((noinline)) static int64_t x_read(int fd, void *b, size_t n) {
	STAT(net_read);
#if defined(XO_NOLIBC)
	return x_nx(xo_x_net_read(fd, XO_ADDR(b), (int64_t)n));
#else
#if defined(__linux__) || defined(__APPLE__)
	if (nx_on()) {
		return x_nx(xo_x_net_read(fd, XO_ADDR(b), (int64_t)n));
	}
#endif
	return x_ret(read(fd, b, n));
#endif
}

__attribute__((noinline)) static int64_t x_write(int fd, const void *b, size_t n) {
	STAT(net_write);
#if defined(XO_NOLIBC)
	return x_nx(xo_x_net_send(fd, XO_ADDR(b), (int64_t)n));
#else
#if defined(__linux__) || defined(__APPLE__)
	if (nx_on()) {
		return x_nx(xo_x_net_send(fd, XO_ADDR(b), (int64_t)n));
	}
#endif
#ifdef MSG_NOSIGNAL
	return x_ret(send(fd, b, n, MSG_NOSIGNAL));
#else
	return x_ret(write(fd, b, n));
#endif
#endif
}
#endif

// Descriptors are close-on-exec from birth on Linux (SOCK_CLOEXEC,
// accept4, pipe2): with FD_CLOEXEC set just after, a process spawned by
// another thread in between inherited the socket or pipe and held it open
// for its life (2026-10-12, gotcha linux-epoll-inherited-socket). macOS has
// neither accept4 nor pipe2, and kqueue drops a closed descriptor's events
// anyway.
#if defined(__linux__)
#if !defined(_GNU_SOURCE)
int accept4(int fd, struct sockaddr *addr, socklen_t *len, int flags);
int pipe2(int fds[2], int flags);
#endif
#define XO_SOCK_CLOEXEC SOCK_CLOEXEC
#else
#define XO_SOCK_CLOEXEC 0
#endif

// xo_pipe is pipe(2) with both ends close-on-exec.
static int xo_pipe(int p[2]) {
#if defined(__linux__)
	return pipe2(p, O_CLOEXEC);
#else
	if (pipe(p) < 0) {
		return -1;
	}
	fcntl(p[0], F_SETFD, FD_CLOEXEC);
	fcntl(p[1], F_SETFD, FD_CLOEXEC);
	return 0;
#endif
}

#if defined(XO_NOLIBC)
__attribute__((noinline)) static int64_t x_accept(int fd) { return xo_x_net_accept(fd, SOCK_CLOEXEC); }
#elif defined(__linux__)
__attribute__((noinline)) static int64_t x_accept(int fd) {
	if (nx_on()) {
		return xo_x_net_accept(fd, SOCK_CLOEXEC);
	}
	return x_ret(accept4(fd, NULL, NULL, SOCK_CLOEXEC));
}
#elif defined(__APPLE__)
__attribute__((noinline)) static int64_t x_accept(int fd) {
	if (nx_on()) {
		return xo_x_net_accept(fd, 0);
	}
	return x_ret(accept(fd, NULL, NULL));
}
#elif defined(_WIN32)
__attribute__((noinline)) static int64_t x_accept(int fd) {
	if (nx_on()) {
		return xo_x_net_accept(fd, 0);
	}
	SOCKET s = accept((SOCKET)fd, NULL, NULL);
	return s == INVALID_SOCKET ? -(int64_t)xo_wsa_errno() : (int64_t)(int)s;
}
#else
__attribute__((noinline)) static int64_t x_accept(int fd) { return x_ret(accept(fd, NULL, NULL)); }
#endif

#ifdef _WIN32
// a nonblocking connect in progress is WSAEWOULDBLOCK
__attribute__((noinline)) static int64_t x_connect(int fd, const struct sockaddr *a, socklen_t n) {
	if (nx_on()) {
		return xo_x_net_connect(fd, XO_ADDR(a), n);
	}
	int64_t r = x_wsa(connect((SOCKET)fd, a, n));
	return r == -EWOULDBLOCK ? -EINPROGRESS : r;
}
#elif defined(XO_NOLIBC)
__attribute__((noinline)) static int64_t x_connect(int fd, const struct sockaddr *a, socklen_t n) {
	return xo_x_net_connect(fd, XO_ADDR(a), n);
}
#else
__attribute__((noinline)) static int64_t x_connect(int fd, const struct sockaddr *a, socklen_t n) {
#if defined(__linux__) || defined(__APPLE__)
	if (nx_on()) {
		return xo_x_net_connect(fd, XO_ADDR(a), n);
	}
#endif
	return x_ret(connect(fd, a, n));
}
#endif

__attribute__((noinline)) static int64_t x_sockerr(int fd) {
#if defined(XO_NOLIBC)
	return xo_x_net_sock_err(fd);
#else
#if defined(__linux__) || defined(__APPLE__) || defined(_WIN32)
	if (nx_on()) {
		return xo_x_net_sock_err(fd);
	}
#endif
	int e = 0;
	socklen_t n = sizeof e;
	if (getsockopt(fd, SOL_SOCKET, SO_ERROR, &e, &n) < 0) {
		return -(int64_t)errno;
	}
#ifdef _WIN32
	return e ? -(int64_t)xo_wsa_map(e) : 0; // a Winsock error code
#else
	return -(int64_t)e;
#endif
#endif
}

// x_errno is errno now (for calls that report errors in it).
__attribute__((noinline)) static int x_errno(void) { return errno; }

static void set_nonblock(int fd) {
#if defined(__linux__) || defined(__APPLE__) || defined(_WIN32)
	if (nx_on()) {
		xo_x_fd_nonblock(fd);
		return;
	}
#endif
	fcntl(fd, F_SETFL, fcntl(fd, F_GETFL) | O_NONBLOCK);
	fcntl(fd, F_SETFD, FD_CLOEXEC);
}

// ev_poke wakes the poller (a new first timer).
static void ev_poke(void) {
#ifdef XO_WINPOLL
	if (ev.afd) {
		PostQueuedCompletionStatus(ev.iocp, 0, 1, NULL);
	} else {
		send((SOCKET)ev.ww, "w", 1, 0);
	}
	return;
#endif
	char b = 'w';
	ssize_t r = write(ev.pfd[1], &b, 1);
	(void)r; // a full pipe wakes it anyway
}

static void *poller_main(void *arg);

#ifdef XO_WINPOLL
// afd_open opens the AFD device for polls, its completions on iocp; NULL
// when it cannot (the poller uses WSAPoll then).
static HANDLE afd_open(HANDLE iocp) {
	const char *e = getenv("XO_WINPOLL");
	if (e && strcmp(e, "wsapoll") == 0) {
		return NULL;
	}
	xo_wsa_init();
	static WCHAR name[] = L"\\Device\\Afd\\Xo";
	UNICODE_STRING u = {(USHORT)(sizeof name - sizeof(WCHAR)), (USHORT)sizeof name, name};
	OBJECT_ATTRIBUTES a;
	InitializeObjectAttributes(&a, &u, 0, NULL, NULL);
	IO_STATUS_BLOCK iosb;
	HANDLE h = NULL;
	NTSTATUS st = NtCreateFile(&h, SYNCHRONIZE, &a, &iosb, NULL, 0, FILE_SHARE_READ | FILE_SHARE_WRITE, FILE_OPEN, 0, NULL, 0);
	if (st != 0) {
		return NULL;
	}
	if (!CreateIoCompletionPort(h, iocp, 0, 0)) {
		CloseHandle(h);
		return NULL;
	}
	SetFileCompletionNotificationModes(h, FILE_SKIP_SET_EVENT_ON_HANDLE);
	return h;
}
#endif

static void ev_init(void) {
#ifdef XO_WINPOLL
	ev.iocp = CreateIoCompletionPort(INVALID_HANDLE_VALUE, NULL, 0, 1);
	ev.afd = ev.iocp ? afd_open(ev.iocp) : NULL;
	if (!ev.afd) {
		int sv[2];
		if (xo_w_socketpair(sv) < 0) {
			xo_oom();
		}
		ev.wr = sv[0];
		ev.ww = sv[1];
	}
#else
	if (xo_pipe(ev.pfd) < 0) {
		xo_oom();
	}
	set_nonblock(ev.pfd[0]);
	set_nonblock(ev.pfd[1]);
#endif
#ifdef XO_WINPOLL
#elif defined(XO_KQUEUE)
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
	ev.poll = nx_epoll_create();
	if (ev.poll < 0) {
		xo_oom();
	}
	struct epoll_event e = {.events = EPOLLIN};
	e.data.ptr = NULL;
	if (nx_epoll_ctl(ev.poll, EPOLL_CTL_ADD, ev.pfd[0], &e) < 0) {
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

// timer_add arms t (when, and park or fn, set).
static void timer_add(xo_timer *t) {
	if (!tw.virt) {
		// the test world has no poller: its timers fire when the
		// virtual clock moves (tw_kick)
		ev_start();
	}
	lk(&ev.mu);
	int first = (int)xo_x_poll_timer_push((int64_t)(intptr_t)t);
	unlk(&ev.mu);
	if (first && !tw.virt) {
		ev_poke();
	}
}

// timer_del disarms t; after it returns t's function does not run.
static void timer_del(xo_timer *t) {
	lk(&ev.mu);
	if (t->idx >= 0) {
		th_remove(t);
	}
	unlk(&ev.mu);
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

// pd_ready hands readiness mode (1 read, 2 write) to pd's waiters, or
// keeps it for the next wait: std/native/llrt/poll.xo's (B22).
void xo_x_poll_pd_ready(int64_t pd, int64_t mode);
static void pd_ready(xo_pd *pd, int mode) { xo_x_poll_pd_ready(XO_ADDR(pd), mode); }

// pd_new registers socket fd (nonblocking) with the poller; write tells
// to watch write readiness too. NULL when the poller refuses it (errno).
static xo_pd *pd_new(int fd, int write) {
	ev_start();
	xo_pd *pd = calloc(1, sizeof(xo_pd));
	if (!pd) {
		xo_oom();
	}
	pd->fd = fd;
#if defined(XO_WINPOLL)
	(void)write;
	if (ev.afd) {
		// polls go to the socket's base handle (below any layered
		// service provider)
		DWORD got = 0;
		pd->base = (SOCKET)fd;
		SOCKET b = INVALID_SOCKET;
		if (WSAIoctl((SOCKET)fd, SIO_BASE_HANDLE, NULL, 0, &b, sizeof b, &got, NULL, NULL) == 0 && b != INVALID_SOCKET) {
			pd->base = b;
		}
		return pd;
	}
	// WSAPoll is level triggered: the poller polls a descriptor only in
	// the directions someone waits in (pd_wait_until pokes it)
	lk(&ev.mu);
	if (ev.nreg == ev.creg) {
		ev.creg = ev.creg ? ev.creg * 2 : 64;
		ev.regs = realloc(ev.regs, (size_t)ev.creg * sizeof(xo_pd *));
		if (!ev.regs) {
			xo_oom();
		}
	}
	ev.regs[ev.nreg++] = pd;
	unlk(&ev.mu);
	int r = 0;
#elif defined(XO_KQUEUE)
	struct kevent k[2];
	EV_SET(&k[0], fd, EVFILT_READ, EV_ADD | EV_CLEAR, 0, 0, pd);
	EV_SET(&k[1], fd, EVFILT_WRITE, EV_ADD | EV_CLEAR, 0, 0, pd);
	int r = kevent(ev.poll, k, write ? 2 : 1, NULL, 0, NULL);
#else
	struct epoll_event e = {.events = EPOLLIN | EPOLLRDHUP | EPOLLET | (write ? EPOLLOUT : 0)};
	e.data.ptr = pd;
	int r = nx_epoll_ctl(ev.poll, EPOLL_CTL_ADD, fd, &e);
#endif
	if (r < 0) {
		free(pd);
		return NULL;
	}
	return pd;
}

// A descriptor's operations, close, and waits are
// std/native/llrt/poll.xo's (B22 stage 5: pd_begin, pd_end, pd_close,
// pd_free, pd_wait_ctx, which other Xo code calls too; on Windows since
// stage 4, with the AFD poll's arm, pd_arm); the C keeps pd_new (the
// registration). poll.xo calls the services record pd_svc for the C's
// parts.
int64_t xo_x_poll_pd_begin(int64_t pd);
void xo_x_poll_pd_end(int64_t pd);
void xo_x_poll_pd_close(int64_t pd);
void xo_x_poll_pd_free(int64_t pd);
int64_t xo_x_poll_pd_wait(int64_t pd, int64_t write, int64_t c, int64_t when);
#define pd_begin(pd) ((int)xo_x_poll_pd_begin(XO_ADDR(pd)))
#define pd_end(pd) xo_x_poll_pd_end(XO_ADDR(pd))
#define pd_close(pd) xo_x_poll_pd_close(XO_ADDR(pd))
#define pd_free(pd) xo_x_poll_pd_free(XO_ADDR(pd))
#define pd_wait_until(pd, write, c, when) ((int)xo_x_poll_pd_wait(XO_ADDR(pd), (write), XO_ADDR(c), (when)))
#ifdef XO_WINPOLL
void xo_x_poll_pd_arm(int64_t pd);
#define pd_arm(pd) xo_x_poll_pd_arm(XO_ADDR(pd))
_Static_assert(offsetof(xo_pd, iosb) == 56 && offsetof(xo_pd, info) == 72 && offsetof(xo_pd, base) == 104 && offsetof(xo_pd, armed) == 112 &&
                   offsetof(xo_pd, rearm) == 116 && offsetof(xo_pd, dying) == 120 && sizeof(xo_afd_poll) == 32 &&
                   offsetof(xo_afd_poll, h) == 16 && offsetof(IO_STATUS_BLOCK, Status) == 0 && sizeof(OVERLAPPED_ENTRY) == 32 &&
                   offsetof(OVERLAPPED_ENTRY, lpOverlapped) == 8,
               "poll.xo's Windows D_* and OVERLAPPED_ENTRY");
#endif

enum { PD_READY, PD_CANCELED, PD_CLOSED, PD_TIMEOUT };

// pd_wait is pd_wait_until without a deadline.
static int pd_wait(xo_pd *pd, int write, xo_ctx *c) { return pd_wait_until(pd, write, c, 0); }

// The poller thread

static void stop_signal(void);

// pd_svc is poll.xo's services record (its S_ words): the C functions it
// calls through rt.call_c2 for a descriptor, the event loop's lock word
// and dead list, and the offset of a context's park.
static int64_t poll_signal_cb(int64_t p, int64_t b);
static int64_t pdsvc_open(int64_t a, int64_t b) {
	(void)b;
	return XO_ADDR(pd_new((int)(a & 0xffffffff), (int)(a >> 32)));
}
static int64_t pdsvc_close(int64_t fd, int64_t b) {
	(void)b;
	nx_close((int)fd);
	return 0;
}
static int64_t pdsvc_shut(int64_t fd, int64_t b) {
	(void)b;
#if !defined(XO_KQUEUE) && !defined(XO_WINPOLL)
	// unregister now: epoll keys a registration by the open file, not the
	// descriptor, so close alone leaves it while a child process still
	// holds the file, and its next event would name this xo_pd after the
	// poller freed it (2026-10-12, gotcha linux-epoll-inherited-socket)
	nx_epoll_ctl(ev.poll, EPOLL_CTL_DEL, (int)fd, NULL);
#endif
#ifdef XO_WINPOLL
	// Windows resets a connection shut down for receiving while input is
	// unread, and the reset discards what the peer has not read of ours
	// yet (a 400 just written): drop the input first (the socket is
	// nonblocking)
	char drop[4096];
	while (recv((SOCKET)fd, drop, sizeof drop, 0) > 0) {
	}
#endif
	nx_shutdown((int)fd, SHUT_RDWR);
	return 0;
}
static int64_t pdsvc_canceled(int64_t c, int64_t b) {
	(void)b;
	return ctx_canceled((xo_ctx *)(intptr_t)c);
}
static int64_t pdsvc_wait(int64_t c, int64_t b) {
	(void)b;
	park_wait_n((xo_ctx *)(intptr_t)c, 1, 0);
	return 0;
}
static int64_t pdsvc_tadd(int64_t t, int64_t b) {
	(void)b;
	timer_add((xo_timer *)(intptr_t)t);
	return 0;
}
static int64_t pdsvc_tdel(int64_t t, int64_t b) {
	(void)b;
	timer_del((xo_timer *)(intptr_t)t);
	return 0;
}
#ifdef XO_WINPOLL
static int64_t pdsvc_poke(int64_t a, int64_t b) {
	(void)a;
	(void)b;
	ev_poke();
	return 0;
}
#endif
static int64_t pd_svc[13];
void xo_x_poll_svc(int64_t rec);
static void pd_svc_init(void) {
	pd_svc[0] = XO_ADDR(&pdsvc_open);
	pd_svc[1] = XO_ADDR(&pdsvc_close);
	pd_svc[2] = XO_ADDR(&pdsvc_shut);
	pd_svc[3] = XO_ADDR(&pdsvc_canceled);
	pd_svc[4] = XO_ADDR(&pdsvc_wait);
	pd_svc[5] = XO_ADDR(&pdsvc_tadd);
	pd_svc[6] = XO_ADDR(&pdsvc_tdel);
	pd_svc[7] = XO_ADDR(&poll_signal_cb);
	pd_svc[8] = XO_ADDR(&ev.mu);
	pd_svc[9] = XO_ADDR(&ev.dead);
	pd_svc[10] = (int64_t)offsetof(xo_ctx, park);
#ifdef XO_WINPOLL
	pd_svc[11] = XO_ADDR(&ev.afd);
	pd_svc[12] = XO_ADDR(&pdsvc_poke);
#endif
	xo_x_poll_svc(XO_ADDR(pd_svc));
}
_Static_assert(sizeof(xo_timer) <= 64, "poll.xo's T_SIZE");

#if defined(XO_WINPOLL)
typedef struct {
	xo_pd *pd;
	int mode;
} xo_event;
#endif

// The poller thread's loop is std/native/llrt/poll.xo's on Linux and
// macOS (poll_main, B22): it frees the dead descriptors, fires the timers
// that are due, waits in kqueue or epoll, hands readiness to the
// descriptors, and drains the self pipe and its signals. The C keeps
// creating the poll descriptor, the self pipe, and the thread (ev_init),
// and Windows' loop below (B22 stage 4); poll.xo calls back into these.
void xo_x_poll_main(int64_t poll, int64_t pipe_r, int64_t mu, int64_t dead, int64_t sigs, int64_t signal_fn, int64_t stop_fn, int64_t free_fn);
#ifdef XO_WINPOLL
void xo_x_poll_main_win(int64_t iocp, int64_t mu, int64_t dead, int64_t sigs, int64_t signal_fn, int64_t stop_fn, int64_t free_fn);
#endif
static int64_t poll_signal_cb(int64_t p, int64_t b) {
	(void)b;
	park_signal((xo_park *)(intptr_t)p);
	return 0;
}
static int64_t poll_stop_cb(int64_t a, int64_t b) {
	(void)a;
	(void)b;
	stop_signal();
	return 0;
}
static int64_t poll_free_cb(int64_t pd, int64_t b) {
	(void)b;
	free((void *)(intptr_t)pd);
	return 0;
}

static void *poller_main(void *arg) {
	(void)arg;
#ifndef XO_WINPOLL
	xo_x_poll_main(ev.poll, ev.pfd[0], XO_ADDR(&ev.mu), XO_ADDR(&ev.dead), XO_ADDR(&ev.sigs), XO_ADDR(&poll_signal_cb), XO_ADDR(&poll_stop_cb),
	               XO_ADDR(&poll_free_cb));
	return NULL;
#else
	if (ev.afd) {
		// AFD polls through the completion port: poll.xo's loop (B22
		// stage 4)
		xo_x_poll_main_win((int64_t)(intptr_t)ev.iocp, XO_ADDR(&ev.mu), XO_ADDR(&ev.dead), XO_ADDR(&ev.sigs), XO_ADDR(&poll_signal_cb),
		                   XO_ADDR(&poll_stop_cb), XO_ADDR(&poll_free_cb));
		return NULL;
	}
	// WSAPoll (no AFD device, or XO_WINPOLL=wsapoll)
	xo_event evs[64];
	for (;;) {
		lk(&ev.mu);
		while (ev.dead) {
			xo_pd *pd = ev.dead;
			ev.dead = pd->next;
#ifdef XO_WINPOLL
			for (int i = 0; i < ev.nreg; i++) {
				if (ev.regs[i] == pd) {
					ev.regs[i] = ev.regs[--ev.nreg];
					break;
				}
			}
#endif
			free(pd);
		}
		int64_t now = mono_ns();
		for (xo_timer *t = th_first(); t && t->when <= now; t = th_first()) {
			th_remove(t);
			__atomic_store_n(&t->fired, 1, __ATOMIC_RELEASE);
			if (t->park) {
				park_signal(t->park);
			} else {
				t->fn(t);
			}
		}
		xo_timer *first = th_first();
		int64_t wait = first ? first->when - now : -1;
		unlk(&ev.mu);
#if defined(XO_WINPOLL)
		DWORD ms = wait < 0 ? INFINITE : (DWORD)((wait + 999999) / 1000000);
		if (wait > (int64_t)1 << 40) {
			ms = 1 << 30;
		}
		// the descriptors someone waits on, in the directions waited for
		lk(&ev.mu);
		if (ev.cpoll < ev.nreg + 1) {
			ev.cpoll = (ev.nreg + 1) * 2;
			ev.pfds = realloc(ev.pfds, (size_t)ev.cpoll * sizeof(WSAPOLLFD));
			ev.pwho = realloc(ev.pwho, (size_t)ev.cpoll * sizeof(xo_pd *));
			if (!ev.pfds || !ev.pwho) {
				xo_oom();
			}
		}
		int nf = 0;
		ev.pfds[nf].fd = (SOCKET)ev.wr;
		ev.pfds[nf].events = POLLRDNORM;
		ev.pfds[nf].revents = 0;
		ev.pwho[nf++] = NULL;
		for (int i = 0; i < ev.nreg; i++) {
			xo_pd *pd = ev.regs[i];
			lk(&pd->mu);
			short want = 0;
			if (!pd->closed && pd->fd >= 0) {
				want = (short)((pd->rwait ? POLLRDNORM : 0) | (pd->wwait ? POLLWRNORM : 0));
			}
			if (want) {
				ev.pfds[nf].fd = (SOCKET)pd->fd;
				ev.pfds[nf].events = want;
				ev.pfds[nf].revents = 0;
				ev.pwho[nf++] = pd;
			}
			unlk(&pd->mu);
		}
		unlk(&ev.mu);
		int got = WSAPoll(ev.pfds, (ULONG)nf, (INT)(ms == INFINITE ? -1 : (int)ms));
		STAT(poll_return);
		(void)evs;
		for (int i = 1; got > 0 && i < nf; i++) {
			short r = ev.pfds[i].revents;
			if (!r) {
				continue;
			}
			int mode = ((r & (POLLRDNORM | POLLHUP | POLLERR | POLLNVAL)) ? 1 : 0) | ((r & (POLLWRNORM | POLLHUP | POLLERR | POLLNVAL)) ? 2 : 0);
			pd_ready(ev.pwho[i], mode);
		}
		if (got > 0 && ev.pfds[0].revents) {
			char b[64];
			while (recv((SOCKET)ev.wr, b, sizeof b, 0) > 0) {
			}
		}
		for (int k = __atomic_exchange_n(&ev.sigs, 0, __ATOMIC_ACQ_REL); k > 0; k--) {
			stop_signal();
		}
#endif
	}
	return NULL;
#endif
}

// ---------------------------------------------------------------------------
// Clock (std.md 3.3): Time is wall clock ns since the Unix epoch, Duration
// ns (the Go backend's representation)

static void tw_sleep_record(int64_t d);

// xo_clock_now is the test world's virtual clock while it runs, else the
// wall clock read in Xo (sysio.xo real_ns through cabi.xo; work item
// B01). Clock.now itself is sysio.xo's clock_now, which calls this only
// for the virtual clock.
int64_t xo_x_real_ns(void);
int64_t xo_clock_now(void) {
	if (tw.virt) {
		return __atomic_load_n(&tw.vnow, __ATOMIC_ACQUIRE);
	}
	return xo_x_real_ns();
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
	int64_t z = 0;
	if (__atomic_compare_exchange_n(&s->fired, &z, 1, 0, __ATOMIC_ACQ_REL, __ATOMIC_ACQUIRE)) {
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

// xo_budget_open and xo_budget_end are budget.xo open and end (which
// arm64 code calls directly).
void xo_budget_open(xo_scope *s, int64_t cpu, int64_t alloc, int64_t steps, const char *loc, int64_t n) {
	xo_x_budget_open(XO_ADDR(s), cpu, alloc, steps, XO_ADDR(loc), n);
}

int64_t xo_budget_end(xo_scope *s) { return xo_x_budget_end(XO_ADDR(s)); }

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
	int unlink_path; // a Unix domain socket whose file close removes (decision 0190)
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

// The address policy of Net.only (decisions 0156, 0175, 0190; std/rt
// attenuate.go onlyNet) is Xo (std/native/llrt/netpolicy.xo, through
// cabi.xo, decision 0207): splitting "host:port", "[v6]:port", or
// ":port"; a numeric port above 65535 (Darwin's getaddrinfo rejects one,
// glibc's truncates it: gotchas/glibc-getaddrinfo-port-truncation.md);
// whether the lists admit a host and port, or a dial or listen address.

static int split_hostport(const char *a, char *host, size_t hn, char *port, size_t pn) {
	return (int)xo_x_split_hostport(XO_ADDR(a), XO_ADDR(host), (int64_t)hn, XO_ADDR(port), (int64_t)pn);
}

static int port_out_of_range(const char *port) { return (int)xo_x_port_out_of_range(XO_ADDR(port)); }

static int net_admits_addr(const char *lp, int64_t ln, const char *a, int64_t n) {
	return (int)xo_x_net_admits_addr(XO_ADDR(lp), ln, XO_ADDR(a), n);
}

// net_denied records NetErr.Denied(addr) (tag 6) for the n bytes at a.
static void net_denied(xo_ctx *c, const char *a, int64_t n) {
	free(c->net_msg);
	c->net_msg = cstr_of(a, n);
	c->net_status = 6;
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
			nx_ntop(AF_INET, &a->sin_addr, ip, sizeof ip);
		}
		sprintf(out, "%s:%d", ip, ntohs(a->sin_port));
	} else {
		const struct sockaddr_in6 *a = (const struct sockaddr_in6 *)sa;
		if (IN6_IS_ADDR_UNSPECIFIED(&a->sin6_addr)) {
			strcpy(ip, "::1");
		} else {
			nx_ntop(AF_INET6, &a->sin6_addr, ip, sizeof ip);
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
	case ENOENT:
		return "no such file or directory";
	case EINVAL:
		return "invalid argument";
	case ENOTDIR:
		return "not a directory";
	case EPROTOTYPE:
		return "protocol wrong type for socket";
	}
	return strerror(e);
}

static const char closed_text[] = "use of closed network connection";

static xo_conn *conn_new(xo_ctx *c, int fd, xo_pd *pd);

// unix_addr fills a sockaddr_un with the path of a `unix:PATH` address:
// -1 when it does not fit (Go's "invalid argument").
static int unix_addr(const char *addr, struct sockaddr_un *sa) {
	const char *path = addr + 5;
	memset(sa, 0, sizeof *sa);
	sa->sun_family = AF_UNIX;
	if (strlen(path) >= sizeof sa->sun_path) {
		return -1;
	}
	strcpy(sa->sun_path, path);
	return 0;
}

// unix_listen is `net.listen("unix:PATH")` (decision 0190): Go's
// net.Listen("unix", PATH), its messages, and the address as given.
static xo_listener *unix_listen(xo_ctx *c, char *addr) {
	const char *path = addr + 5;
	struct sockaddr_un sa;
	if (unix_addr(addr, &sa) < 0) {
		net_fail(c, 4, "listen unix %s: bind: invalid argument", path);
		free(addr);
		return xo_h_zero();
	}
	int fd = nx_socket(AF_UNIX, SOCK_STREAM | XO_SOCK_CLOEXEC, 0);
	if (fd < 0) {
		net_fail(c, 4, "listen unix %s: socket: %s", path, err_text(x_errno()));
		free(addr);
		return xo_h_zero();
	}
	if (nx_bind(fd, (struct sockaddr *)&sa, sizeof sa) != 0) {
		int e = x_errno();
		nx_close(fd);
		net_fail(c, 4, "listen unix %s: bind: %s", path, err_text(e));
		free(addr);
		return xo_h_zero();
	}
	if (nx_listen(fd, SOMAXCONN) != 0) {
		int e = x_errno();
		nx_close(fd);
		unlink(path);
		net_fail(c, 4, "listen unix %s: listen: %s", path, err_text(e));
		free(addr);
		return xo_h_zero();
	}
	set_nonblock(fd);
	xo_pd *pd = pd_new(fd, 0);
	if (!pd) {
		int e = x_errno();
		nx_close(fd);
		unlink(path);
		net_fail(c, 4, "listen unix: %s", err_text(e));
		free(addr);
		return xo_h_zero();
	}
	xo_listener *l = xo_alloc(sizeof(xo_listener));
	h_init(l, listener_free);
	l->pd = pd;
	l->mem = NULL;
	l->addr = addr;
	l->unlink_path = 1;
	net_ok(c);
	return l;
}

// unix_dial is `net.dial("unix:PATH")` (decision 0190): a socket file
// with no listener is Refused, as in Go.
static xo_conn *unix_dial(xo_ctx *c, const char *addr, int64_t when) {
	const char *path = addr + 5;
	struct sockaddr_un sa;
	if (unix_addr(addr, &sa) < 0) {
		net_fail(c, 4, "dial unix %s: connect: invalid argument", path);
		return xo_h_zero();
	}
	int fd = nx_socket(AF_UNIX, SOCK_STREAM | XO_SOCK_CLOEXEC, 0);
	if (fd < 0) {
		net_fail(c, 4, "dial unix %s: socket: %s", path, err_text(x_errno()));
		return xo_h_zero();
	}
	set_nonblock(fd);
	xo_pd *pd = pd_new(fd, 1);
	if (!pd) {
		int e = x_errno();
		nx_close(fd);
		net_fail(c, 4, "dial unix %s: %s", path, err_text(e));
		return xo_h_zero();
	}
	pd_begin(pd);
	int64_t e = x_connect(fd, (struct sockaddr *)&sa, sizeof sa);
	while (e == -EINTR) {
		e = x_connect(fd, (struct sockaddr *)&sa, sizeof sa);
	}
	if (e == -EINPROGRESS || e == -EAGAIN) {
		int w = pd_wait_until(pd, 1, c, when);
		e = w == PD_READY ? x_sockerr(fd) : w == PD_CANCELED ? 1 : w == PD_TIMEOUT ? 2 : -ECONNABORTED;
	}
	pd_end(pd);
	if (e == 0) {
		return conn_new(c, fd, pd);
	}
	pd_free(pd);
#ifdef _WIN32
	// Windows refuses a connect to a missing socket file; POSIX (and the
	// Fs view of it) says it does not exist
	if (e == -ECONNREFUSED && xo_w_attrs(path) == INVALID_FILE_ATTRIBUTES) {
		e = -ENOENT;
	}
#endif
	if (e == 1) {
		net_fail(c, 5, "");
	} else if (e == 2) {
		net_fail(c, 4, "dial unix %s: i/o timeout", path);
	} else if (e == -ECONNREFUSED) {
		net_fail(c, 2, "");
	} else {
		net_fail(c, 4, "dial unix %s: connect: %s", path, err_text((int)-e));
	}
	return xo_h_zero();
}

// xo_net_listen is `net.listen(addr)` (std.md 3.2) on a Net of the
// attenuation lists lp (decision 0175): an address they do not admit is
// NetErr.Denied before anything else.
xo_listener *xo_net_listen(const char *lp, int64_t ln, const char *p, int64_t n) {
	xo_ctx *c = ctx();
	if (!net_admits_addr(lp, ln, p, n)) {
		net_denied(c, p, n);
		return xo_h_zero();
	}
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
	if (strncmp(addr, "unix:", 5) == 0) {
		return unix_listen(c, addr);
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
	int g = nx_getaddrinfo(host[0] ? host : NULL, port, &hints, &res);
	if (g != 0) {
		net_fail(c, 4, "listen tcp %s: %s", addr, nx_gai_strerror(g));
		free(addr);
		return xo_h_zero();
	}
	int fd = -1, e = 0;
	for (struct addrinfo *a = res; a; a = a->ai_next) {
		fd = nx_socket(a->ai_family, a->ai_socktype | XO_SOCK_CLOEXEC, a->ai_protocol);
		if (fd < 0) {
			e = x_errno();
			continue;
		}
		nx_setsockopt_int(fd, SOL_SOCKET, SO_REUSEADDR, 1);
		if (a->ai_family == AF_INET6 && !host[0]) {
			nx_setsockopt_int(fd, IPPROTO_IPV6, IPV6_V6ONLY, 0);
		}
		if (nx_bind(fd, a->ai_addr, a->ai_addrlen) == 0 && nx_listen(fd, SOMAXCONN) == 0) {
			break;
		}
		e = x_errno();
		nx_close(fd);
		fd = -1;
	}
	nx_freeaddrinfo(res);
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
		nx_close(fd);
		net_fail(c, 4, "listen tcp: %s", err_text(e));
		return xo_h_zero();
	}
	struct sockaddr_storage ss;
	socklen_t sl = sizeof ss;
	nx_getsockname(fd, (struct sockaddr *)&ss, &sl);
	xo_listener *l = xo_alloc(sizeof(xo_listener));
	h_init(l, listener_free);
	l->pd = pd;
	l->mem = NULL;
	l->addr = fmt_addr((struct sockaddr *)&ss);
	l->unlink_path = 0;
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
	if (l->unlink_path) {
		// as Go's UnixListener.Close: the socket file goes, once
		l->unlink_path = 0;
		unlink(l->addr + 5);
	}
}

// conn_new makes a connection handle of a connected socket.
static xo_conn *conn_new(xo_ctx *c, int fd, xo_pd *pd) {
	nx_setsockopt_int(fd, IPPROTO_TCP, TCP_NODELAY, 1);
#ifdef SO_NOSIGPIPE
	nx_setsockopt_int(fd, SOL_SOCKET, SO_NOSIGPIPE, 1);
#endif
	if (!pd) {
		pd = pd_new(fd, 1);
		if (!pd) {
			int e = x_errno();
			nx_close(fd);
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
		net_fail(c, 4, "accept %s %s: %s", strncmp(l->addr, "unix:", 5) == 0 ? "unix" : "tcp", strncmp(l->addr, "unix:", 5) == 0 ? l->addr + 5 : l->addr, closed_text);
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
				net_fail(c, 4, "accept %s %s: %s", strncmp(l->addr, "unix:", 5) == 0 ? "unix" : "tcp", strncmp(l->addr, "unix:", 5) == 0 ? l->addr + 5 : l->addr, closed_text);
			}
			return xo_h_zero();
		}
		pd_end(pd);
		net_fail(c, 4, "accept %s %s: %s", strncmp(l->addr, "unix:", 5) == 0 ? "unix" : "tcp", strncmp(l->addr, "unix:", 5) == 0 ? l->addr + 5 : l->addr, err_text((int)-fd));
		return xo_h_zero();
	}
}

// xo_net_dial is `net.dial(addr)` (std.md 3.2): connection refused is
// NetErr.Refused; it gives up after 30 s like the Go backend's
// net.DialTimeout (NetErr.Io "i/o timeout").
xo_conn *xo_net_dial_timeout(const char *p, int64_t n, int64_t d);

xo_conn *xo_net_dial(const char *lp, int64_t ln, const char *p, int64_t n) {
	if (!net_admits_addr(lp, ln, p, n)) {
		// Net.only (decision 0175): denied before anything else
		net_denied(ctx(), p, n);
		return xo_h_zero();
	}
	return xo_net_dial_timeout(p, n, (int64_t)30 * 1000000000);
}

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
	if (strncmp(addr, "unix:", 5) == 0) {
		xo_conn *u = unix_dial(c, addr, when);
		free(addr);
		return u;
	}
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
	int g = nx_getaddrinfo(host[0] ? host : "localhost", port, &hints, &res);
	if (g != 0) {
		net_fail(c, 4, "dial tcp %s: %s", addr, nx_gai_strerror(g));
		free(addr);
		return xo_h_zero();
	}
	int64_t e = 0;
	xo_conn *out = NULL;
	for (struct addrinfo *a = res; a && !out; a = a->ai_next) {
		int fd = nx_socket(a->ai_family, a->ai_socktype | XO_SOCK_CLOEXEC, a->ai_protocol);
		if (fd < 0) {
			e = -x_errno();
			continue;
		}
		set_nonblock(fd);
		xo_pd *pd = pd_new(fd, 1);
		if (!pd) {
			e = -x_errno();
			nx_close(fd);
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
	nx_freeaddrinfo(res);
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

// The process environment, which C main hands to process.xo
// (xo_x_proc_start below).
#ifdef _WIN32
#undef environ // the C runtime's (stdlib.h)
#define environ (xo_w_environ()) // UTF-8 (xo_win.c)
#else
extern char **environ;
#endif

// ---------------------------------------------------------------------------
// Signals and two phase shutdown (core 7.5, decision 0042): the state
// (the signals counted, the stop request, the grace timer, task.stopping's
// channel, main's root context) is std/native/llrt/stop.xo's (the zero-C
// audit's B26, part 1). The C keeps the handlers, which count a signal
// and wake the poller (whose thread calls stop_signal), and stop_svc, the
// services record stop.xo calls through rt.call_c2 (its S_ words).

void xo_x_stop_init(int64_t svc);
void xo_x_stop_signal(void);
void xo_x_stop_cancel(void);
int64_t xo_x_stop_requested(void);
int64_t xo_x_stop_canceled(void);
int64_t xo_x_stop_stopping(int64_t drop);
void xo_x_stop_set_grace(int64_t d);
int64_t xo_x_stop_root(void);
void xo_x_stop_set_root(int64_t n);
void xo_x_stop_end(void);
void xo_x_stop_reset(int64_t n);
int64_t xo_x_stop_unroot(int64_t n);

// stop_signal handles SIGINT or SIGTERM (on the poller thread).
static void stop_signal(void) { xo_x_stop_signal(); }

static int64_t stopsvc_cancel(int64_t n, int64_t b) {
	(void)b;
	node_cancel((xo_node *)(intptr_t)n);
	return 0;
}
static int64_t stopsvc_chan(int64_t drop, int64_t b) {
	(void)b;
	static const char loc[] = "task.stopping";
	xo_chan *ch = xo_chan_new(0, drop, loc, (int64_t)sizeof loc - 1);
	ch->ext = 1;
	return XO_ADDR(ch);
}
static int64_t stopsvc_close(int64_t ch, int64_t b) {
	(void)b;
	xo_chan_close((xo_chan *)(intptr_t)ch);
	return 0;
}
static int64_t stopsvc_retain(int64_t ch, int64_t b) {
	(void)b;
	h_retain((void *)(intptr_t)ch);
	return 0;
}
static int64_t stopsvc_release(int64_t ch, int64_t b) {
	(void)b;
	h_release((void *)(intptr_t)ch);
	return 0;
}
static int64_t stopsvc_exit(int64_t a, int64_t b) {
	(void)a;
	(void)b;
	xo_flush();
	_exit(130);
}
static int64_t stopsvc_now(int64_t a, int64_t b) {
	(void)a;
	(void)b;
	return tnow();
}
static void stop_grace_fire(xo_timer *t) {
	(void)t;
	xo_x_stop_cancel();
}
#ifndef _WIN32
static void on_signal(int sig) {
	(void)sig;
	int e = errno;
	__atomic_fetch_add(&ev.sigs, 1, __ATOMIC_ACQ_REL);
	char b = 's';
	ssize_t r = write(ev.pfd[1], &b, 1);
	(void)r;
	errno = e;
}
#endif

#ifdef _WIN32
// Ctrl-C, Ctrl-Break, and closing the console are Windows' SIGINT and
// SIGTERM: the handler runs on a thread of its own and wakes the poller.
static BOOL WINAPI on_console(DWORD kind) {
	switch (kind) {
	case CTRL_C_EVENT:
	case CTRL_BREAK_EVENT:
	case CTRL_CLOSE_EVENT:
	case CTRL_SHUTDOWN_EVENT:
		__atomic_fetch_add(&ev.sigs, 1, __ATOMIC_ACQ_REL);
		ev_poke();
		return TRUE;
	}
	return FALSE;
}

// on_overflow reports a stack overflow (decision 0197, stage 5): not a
// fault (decision 0085), so nothing unwinds; the program ends as the Go
// backend's does, with "fatal error: stack overflow" and status 2, where
// Windows would end it silently (0xC00000FD). It runs on what is left of
// the overflowed stack (the guarantee SetThreadStackGuarantee keeps for
// the thread), so it only writes and exits.
static LONG CALLBACK on_overflow(EXCEPTION_POINTERS *e) {
	if (e->ExceptionRecord->ExceptionCode != EXCEPTION_STACK_OVERFLOW) {
		return EXCEPTION_CONTINUE_SEARCH;
	}
	static const char m[] = "fatal error: stack overflow\n";
	DWORD w;
	// the program's buffered output first, unless a writer holds it
	xo_x_flush_try();
	WriteFile(GetStdHandle(STD_ERROR_HANDLE), m, sizeof m - 1, &w, NULL);
	TerminateProcess(GetCurrentProcess(), 2);
	return EXCEPTION_CONTINUE_SEARCH;
}
#endif

// The entry's services (std/native/llrt/entry.xo E_ words): functions
// that take no arguments are called through rt.call_c2 as they are (the
// two arguments it passes are ignored on every target's C convention).
static void tw_init(void);
void xo_consts_free(void);
#ifdef XO_TIME
void xo_x_tz_free(void);
#endif
static int64_t entrysvc_mid_max(int64_t m, int64_t b) {
	(void)b;
	xo_mid_max = m;
	main_heap.mid_max = m;
	return 0;
}

static int64_t stop_svc[20];
static void stop_svc_init(void) {
	stop_svc[0] = XO_ADDR(&pdsvc_tadd);
	stop_svc[1] = XO_ADDR(&pdsvc_tdel);
	stop_svc[2] = XO_ADDR(&stopsvc_cancel);
	stop_svc[3] = XO_ADDR(&stopsvc_chan);
	stop_svc[4] = XO_ADDR(&stopsvc_close);
	stop_svc[5] = XO_ADDR(&stopsvc_retain);
	stop_svc[6] = XO_ADDR(&stopsvc_release);
	stop_svc[7] = XO_ADDR(&stopsvc_exit);
	stop_svc[8] = XO_ADDR(&stopsvc_now);
	stop_svc[9] = XO_ADDR(&stop_grace_fire);
	stop_svc[10] = XO_ADDR(&tw.in_main);
	stop_svc[11] = XO_ADDR(&tw_init);
	stop_svc[12] = XO_ADDR(&ev_start);
#ifdef _WIN32
	stop_svc[13] = XO_ADDR(&on_console);
	stop_svc[14] = XO_ADDR(&on_overflow);
#else
	stop_svc[13] = XO_ADDR(&on_signal);
#endif
	stop_svc[15] = XO_ADDR(&entrysvc_mid_max);
	stop_svc[16] = XO_ADDR(&xo_consts_free);
#ifdef XO_TIME
	stop_svc[17] = XO_ADDR(&xo_x_tz_free);
#endif
	stop_svc[18] = XO_ADDR(&st);
	stop_svc[19] = XO_ADDR(&rtstats);
	xo_x_stop_init(XO_ADDR(stop_svc));
}

int64_t xo_stop_requested(void) { return xo_x_stop_requested(); }

// xo_task_stopping is `task.stopping()`: a channel closed when a stop is
// requested (drop releases values someone sends on it).
xo_chan *xo_task_stopping(int64_t drop) { return (xo_chan *)(intptr_t)xo_x_stop_stopping(drop); }

// xo_set_grace is `proc.set_grace(d)`: after a stop request the deadline
// moves to the stop time plus d.
void xo_set_grace(int64_t d) { xo_x_stop_set_grace(d); }

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

// selects.xo's ops for the `after` arm's timer (rt.sched_rt 27, 28).
static int64_t select_timer_op(int64_t s) {
	struct xo_timer *tm = NULL;
	select_timer((xo_select *)(intptr_t)s, &tm);
	return XO_ADDR(tm);
}

static void select_untimer_op(int64_t t) { select_untimer((struct xo_timer *)(intptr_t)t); }

// ---------------------------------------------------------------------------
// Entry (left out with -DXO_RT_NO_MAIN: the C tests of std/llrt/testdata
// include this file and bring their own main)

#ifndef XO_RT_NO_MAIN
extern void xo_entry(void);

static void *run_main(void *arg) {
	(void)arg;
	// main's root context: the second shutdown signal cancels it
	xo_x_stop_set_root(XO_ADDR(ctx()->node));
	// a fault that unwinds main reaches this frame: reported, status 70
	// (decision 0085)
	ctx()->boundary = (uintptr_t)__builtin_frame_address(0);
	xo_entry();
	return NULL;
}

static void tw_cleanup(void);
static void xo_streams_free(void);

// the program's start and end (std/native/llrt/entry.xo)
void xo_x_entry_start(int64_t argc, int64_t argv, int64_t envp);
void xo_x_entry_finish(void);
void xo_x_rt_stats(void);

int main(int argc, char **argv) {
	obj_heap_init(&main_heap);
	fault_init();
#ifdef _WIN32
	// standard streams in binary mode (no \n to \r\n)
	_setmode(0, _O_BINARY);
	_setmode(1, _O_BINARY);
	_setmode(2, _O_BINARY);
#endif
	// the arguments and the environment (env.args, env.program, env.get,
	// and env.vars read them in Xo; `environ` is xo_w_environ() on
	// Windows), XO_HEAP_MID_MAX, then the test world (XO_TEST) or the
	// signal handlers
	xo_x_entry_start(argc, (int64_t)(intptr_t)argv, (int64_t)(intptr_t)environ);
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
	xo_x_stop_end();
	xo_x_rt_stats();
	xo_streams_free();
	if (tw.on) {
		tw_cleanup();
	}
	// XO_LEAKCHECK's and XO_HEAPSTATS' reports
	xo_x_entry_finish();
	return 0;
}
#endif

// The intrinsics of the native std implementation (decision 0074).
#include "xo_std.c"

// The C library functions of a Linux program linked with no C library
// (XO_NOLIBC=1 builds, decisions 0203 and 0206).
#ifdef XO_NOLIBC
#include "xo_nolibc.c"
#endif
