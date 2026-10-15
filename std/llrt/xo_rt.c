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
// Windows (decision 0197): the Win32 and Winsock headers (clang
// --target=x86_64-pc-windows-msvc or aarch64-pc-windows-msvc, with the
// MSVC C runtime and the Windows SDK; xo_win.c's POSIX subset was ported
// to Xo and deleted, the zero-C audit's B32, 2026-10-15)
#ifndef WIN32_LEAN_AND_MEAN
#define WIN32_LEAN_AND_MEAN
#endif
#ifndef NOMINMAX
#define NOMINMAX
#endif
#ifndef _CRT_SECURE_NO_WARNINGS
#define _CRT_SECURE_NO_WARNINGS
#endif
#ifndef _CRT_NONSTDC_NO_DEPRECATE
#define _CRT_NONSTDC_NO_DEPRECATE
#endif
#ifndef _WINSOCK_DEPRECATED_NO_WARNINGS
#define _WINSOCK_DEPRECATED_NO_WARNINGS
#endif

#include <winsock2.h>
#include <ws2tcpip.h>
#include <afunix.h>
#include <windows.h>
#include <shellapi.h>
#include <winioctl.h>
#include <wincrypt.h>
#include <bcrypt.h>
#include <io.h>
#include <fcntl.h>
#include <process.h>
#include <direct.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <errno.h>
#include <math.h>
#include <signal.h>
#include <stdarg.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

#pragma comment(lib, "ws2_32")
#pragma comment(lib, "bcrypt")
#pragma comment(lib, "synchronization")
#pragma comment(lib, "kernel32")
#pragma comment(lib, "shell32")
#pragma comment(lib, "crypt32")
#else
#include <math.h>
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

#ifdef _WIN32
// std/native/llrt/sock.xo's wsa_errno returns these numbers (the Windows
// C runtime's errno.h) for Winsock's codes, and sock.xo and cabi_net.xo
// take Winsock's constants and layouts below
_Static_assert(EWOULDBLOCK == 140 && EINPROGRESS == 112 && ECONNREFUSED == 107 && ECONNRESET == 108 && ECONNABORTED == 106 &&
                   EPIPE == 32 && ETIMEDOUT == 138 && EADDRINUSE == 100 && EADDRNOTAVAIL == 101 && EACCES == 13 && EINVAL == 22 &&
                   EINTR == 4 && ENOTSOCK == 128 && ENETUNREACH == 118 && EHOSTUNREACH == 110 && ENOTCONN == 126 &&
                   EAFNOSUPPORT == 102 && EIO == 5,
               "sock.xo wsa_errno");
_Static_assert(WSAEWOULDBLOCK == 10035 && WSAEINPROGRESS == 10036 && WSAEALREADY == 10037 && WSAECONNREFUSED == 10061 &&
                   WSAECONNRESET == 10054 && WSAECONNABORTED == 10053 && WSAESHUTDOWN == 10058 && WSAETIMEDOUT == 10060 &&
                   WSAEADDRINUSE == 10048 && WSAEADDRNOTAVAIL == 10049 && WSAEACCES == 10013 && WSAEINVAL == 10022 &&
                   WSAEINTR == 10004 && WSAENOTSOCK == 10038 && WSAENETUNREACH == 10051 && WSAEHOSTUNREACH == 10065 &&
                   WSAENOTCONN == 10057 && WSAEAFNOSUPPORT == 10047,
               "sock.xo wsa_errno");
_Static_assert(WSAHOST_NOT_FOUND == 11001 && WSATRY_AGAIN == 11002 && WSANO_RECOVERY == 11003 && WSANO_DATA == 11004 &&
                   WSATYPE_NOT_FOUND == 10109 && AI_NUMERICSERV == 8 && FIONBIO == (long)0x8004667E && SOL_SOCKET == 0xffff &&
                   SO_ERROR == 0x1007 && AF_INET6 == 23 && sizeof(WSADATA) <= 512 && offsetof(struct addrinfo, ai_addrlen) == 16 &&
                   offsetof(struct addrinfo, ai_addr) == 32 && offsetof(struct addrinfo, ai_next) == 40,
               "sock.xo and cabi_net.xo Windows constants");
#endif

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
// thread every call uses main_heap (obj.xo's static word HEAP), so
// single threaded code pays one load and a predicted branch.
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

// A free deferred by a real-time section (decision 0133,
// std/native/llrt/rtsec.xo; the queue is obj.xo's rt_defer and rt_drain):
// what reached a count of zero and how to free it.
#define XO_RTQ 256
typedef struct {
	int kind; // obj.xo RT_STR, RT_LIST, RT_DEFER, RT_HANDLE
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
	// unused (they carried a real-time section's list free to the C)
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
_Static_assert(offsetof(xo_heap, rt_n) == 2144 && offsetof(xo_heap, rt_q) == 2152 && sizeof(xo_rtdef) == 32 && offsetof(xo_rtdef, obj) == 8 && offsetof(xo_rtdef, esize) == 16 && offsetof(xo_rtdef, fn) == 24, "obj.xo's RT_N, RT_Q");

// The central lists: per class, a stack of magazines (free lists of
// about central_batch(c) blocks) linked by their first block's second
// word (every block has at least 16 bytes).
static void *central[XO_NCLASS + 1];

static xo_heap main_heap = {.mid_max = XO_MID_MAX, .central = central};
static int xo_threaded; // set once, before a second thread runs Xo code
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
void xo_x_obj_heap_init(int64_t h, int64_t slow, int64_t hsize);

// hw_slow_c is the calling thread's heap record for obj.xo's heap_word
// (called through rt.call_c2: two Int arguments, unused).
static int64_t hw_slow_c(int64_t a, int64_t b) {
	(void)a;
	(void)b;
	return (int64_t)(intptr_t)heap_slow();
}

static void obj_heap_init(xo_heap *h) { xo_x_obj_heap_init((int64_t)(intptr_t)h, (int64_t)(intptr_t)&hw_slow_c, (int64_t)sizeof(xo_heap)); }

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
	obj_heap_init(NULL);
	__atomic_store_n(&xo_threaded, 1, __ATOMIC_RELAXED);
	// standard output's buffer takes its lock from now on (stdio.xo)
	xo_x_stdio_threads();
}

// The runtime's records from malloc (std/native rt.heap_large): the C
// frees some of them with free (tasks.xo task_free), so these stay
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
// system and locks the central lists.
int64_t xo_x_heap_chunks(void);

// xo_chunks is the chunks mapped so far (std/llrt/testdata/tasks.c and
// alloc_threads.c check that blocks freed on another thread come back
// instead, decision 0118).
#define xo_chunks_now() xo_x_heap_chunks()

// The heap, strings and lists in Xo (decisions 0198 and 0207, zero-C
// audit B13): std/native/llrt/alloc.xo and obj.xo through cabi.xo's
// exports.

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

static inline __attribute__((always_inline)) void rc_inc(int64_t *p) { xo_x_rc_inc((int64_t)(intptr_t)p); }
static inline __attribute__((always_inline)) int rc_dec(int64_t *p) { return (int)xo_x_rc_dec((int64_t)(intptr_t)p); }

// ---------------------------------------------------------------------------
// The test world (decision 0083): a test binary run with XO_TEST set runs
// one test against fakes. The test world is Xo (std/native/llrt/
// testworld.xo, twrun.xo, and twflags.xo, which keeps its record: on, the
// virtual clock, in_main, the effects made real), reached through
// cabi_tw.xo's exports (xo_x_tw_*), which do nothing outside it.

// The exports of std/native/llrt/cabi_tw.xo, which test binaries carry;
// the weak definitions keep other programs linking.
#define TW_WEAK __attribute__((weak))
TW_WEAK void xo_x_tw_start(void) {}
TW_WEAK void xo_x_tw_end(void) {}
TW_WEAK void xo_x_tw_kick(void) {}
TW_WEAK void xo_x_tw_sleep(int64_t d) { (void)d; }

// ---------------------------------------------------------------------------
// Output: standard output's buffer is Xo (std/native/llrt/stdio.xo, the
// zero-C audit's work item B03); the C flushes it through cabi.xo's
// exports before a wait, at exit, and before a fault's message.

void xo_x_flush(void);
void xo_x_flush_try(void);

void xo_flush(void) { xo_x_flush(); }

// Addresses pass to the C ABI exports of std/native/llrt/cabi.xo
// (decisions 0204, 0207) as int64_t.
#define XO_ADDR(p) ((int64_t)(intptr_t)(p))

// ---------------------------------------------------------------------------
// Faults (core 8): the same messages as std/rt, exit status 70

// A fault unwinds the task that raised it (decision 0085, "Fault
// unwinding" below): fault_raise_msg records the message in the running
// context and raises.
XO_NORETURN static void fault_raise_msg(const char *msg, const char *loc);

// The fault entry points of generated code are cabi.xo's fault_raise_*
// exports (decision 0228: fault.xo writes the message and calls
// fault_raise_msg, whose address fault_init gives it), which the
// emitters name.
void xo_x_fault_init(int64_t raise_msg);

// fault_init gives fault.xo the address of fault_raise_msg and ctx.xo
// that of ctx_root (with the heap words: first in main, or in
// XO_RT_NO_MAIN's constructor).
void xo_x_ctx_init(int64_t tsan);
static void ctx_init(void);
static void pd_svc_init(void);
static void stop_svc_init(void);
static void fault_init(void) {
	xo_x_fault_init(XO_ADDR(fault_raise_msg));
	ctx_init();
	pd_svc_init();
	stop_svc_init();
}

// ---------------------------------------------------------------------------
// Strings

// ---------------------------------------------------------------------------
// Reference counted strings and lists (code lowered from internal/ssa)

typedef struct {
	int64_t rc, cap;
	char data[];
} xo_rstr;

typedef struct {
	int64_t rc, len, cap;
} xo_rlist;

typedef void (*xo_elem_fn)(void *);

// The entries LLVM generated code calls (arm64 generated code calls the
// obj_*_n exports directly): they forward to obj.xo's entries, which find
// the heap themselves, so LTO folds this level into the caller.
int64_t xo_x_obj_rl_grow_n(int64_t l, int64_t esize);
int64_t xo_x_obj_rl_copy_n(int64_t l, int64_t esize, int64_t dup, int64_t drop);

#define XO_A(p) ((int64_t)(intptr_t)(p))

// xo_rl_copy is the slow path of `reuse`: l's count is not 1 (obj.xo
// rl_copy).
__attribute__((noinline)) xo_rlist *xo_rl_copy(xo_rlist *l, int64_t esize, xo_elem_fn dup, xo_elem_fn drop) {
	return (xo_rlist *)(intptr_t)xo_x_obj_rl_copy_n(XO_A(l), esize, XO_A(dup), XO_A(drop));
}

// xo_rl_grow doubles the capacity of a unique full list (obj.xo rl_grow).
__attribute__((noinline)) xo_rlist *xo_rl_grow(xo_rlist *l, int64_t esize) { return (xo_rlist *)(intptr_t)xo_x_obj_rl_grow_n(XO_A(l), esize); }

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
//     code retains and releases them like lists and frees them at zero
//     (obj.xo's h_free).
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

// The placeholders a canceled operation returns while it unwinds (never
// written: their counts are immortal).
// Generated code takes their addresses directly (rt.rl_empty, rt.h_zero;
// the generated-code entries' batch E3).
xo_hdr xo_zero_handle = {XO_RC_IMMORTAL, NULL};
xo_rlist xo_empty_list = {XO_RC_IMMORTAL, 0, 0};
#define zero_handle xo_zero_handle
#define empty_list xo_empty_list

// ---------------------------------------------------------------------------
// Parks, nodes, contexts

// Runtime statistics (decision 0077): std/native/llrt/rtstats.xo.

// ---------------------------------------------------------------------------
// Real-time sections (core 6.5, decision 0133): std/native/llrt/rtsec.xo,
// through cabi.xo's exports, which generated code calls.

void xo_x_rt_fault_leave(void);

// rt_fault_leave ends the thread's real-time sections when a fault
// unwinds out of them.
static void rt_fault_leave(void) { xo_x_rt_fault_leave(); }

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

// Deadlock detection (core 11.4, fault XO-F001), decision 0077 (pool.xo
// dl_check, its counts in the pool record's P_DLK): the
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

// A park is where one context waits. signaled is also the futex word of
// a thread (a root context) that sleeps on it; a fiber that waits parks
// instead (decision 0069). All zero is a fresh park.
typedef struct {
	xo_lock mu;
	int64_t signaled; // written under mu, read atomically (the futex word: its low half)
	int sleeping;     // a thread sleeps on signaled
	int counted;      // a root asleep here counts in the blocked roots (pool.xo DLK_BLOCKED)
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

// The parks and the cancellation tree in Xo (decision 0212 stage 2):
// std/native/llrt/pool.xo through cabi.xo, wrapped below the pool's
// record.
static void park_reset(xo_park *p);

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

static void node_link(xo_node *n, xo_node *parent);
static void node_unlink(xo_node *n);

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
	// __tsan_func_exit on unwind; gotchas/tsan-resumes-fault-unwind.md).
	// The unwinder declares it 16 byte aligned, which a context from the
	// Xo heap is not (tasks.xo run_claimed: a block is 8 mod 16, after
	// its header), so this 8 aligned area holds it at its first 16
	// byte aligned address (ctx_exc; a context never moves).
	// gotchas/xo-heap-records-8-aligned.md
	unsigned char fault_exc[sizeof(struct _Unwind_Exception) + 8];
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

// ctx returns the current context; a thread that runs Xo code outside
// any task (the program's main thread) gets a root context from ctx.xo
// root.
int64_t xo_x_ctx_root(void);
static xo_ctx *ctx(void) {
	xo_ctx *c = cur_get();
	if (__builtin_expect(c != NULL, 1)) {
		return c;
	}
	return (xo_ctx *)(intptr_t)xo_x_ctx_root();
}

// The context record's layout, which ctx.xo's C_* name (decision 0229).
_Static_assert(offsetof(xo_ctx, node) == 0 && offsetof(xo_ctx, unwinding) == 8 && offsetof(xo_ctx, shield) == 12 &&
                   offsetof(xo_ctx, park) == 16 && offsetof(xo_ctx, held) == 48 && offsetof(xo_ctx, nheld) == 56 &&
                   offsetof(xo_ctx, capheld) == 60 && offsetof(xo_ctx, net_status) == 64 && offsetof(xo_ctx, net_msg) == 72 &&
                   offsetof(xo_ctx, faulting) == 80 && offsetof(xo_ctx, fault_msg) == 88 && offsetof(xo_ctx, fault_loc) == 96 &&
                   offsetof(xo_ctx, fault_detail) == 104 && offsetof(xo_ctx, boundary) == 112 && offsetof(xo_ctx, fault_exc) == 120,
               "ctx.xo C_*");
_Static_assert(offsetof(xo_ctx, bud) == 160 && offsetof(xo_ctx, bud_left) == 168 && offsetof(xo_ctx, bud_q) == 176 &&
                   offsetof(xo_ctx, bud_run) == 184 && offsetof(xo_ctx, bud_amark) == 192 && offsetof(xo_ctx, bud_heap) == 200 &&
                   offsetof(xo_ctx, bud_paused) == 208 && offsetof(xo_ctx, went) == 216 && offsetof(xo_ctx, cent) == 248 && offsetof(xo_ctx, fired) == 312 && sizeof(xo_ctx) == 320 &&
                   sizeof(xo_went) == 32 && offsetof(xo_went, writer) == 24,
               "ctx.xo C_BUD_*");

// tasks.xo allocates contexts from the Xo heap, 8 aligned
// (gotchas/xo-heap-records-8-aligned.md).
_Static_assert(_Alignof(xo_ctx) <= 8, "xo_ctx from the Xo heap");

// ctx_exc is the fault's exception object in c's fault_exc area.
static inline struct _Unwind_Exception *ctx_exc(xo_ctx *c) {
	return (struct _Unwind_Exception *)(((uintptr_t)c->fault_exc + 15) & ~(uintptr_t)15);
}

// ---------------------------------------------------------------------------
// Fault unwinding (core 8, decision 0085)
//
// A fault is raised as a forced unwind (decision 0221): on Linux and
// macOS, on arm64 and x86_64 (stage 2, 2026-10-15), the Xo unwinder
// walks the frames (std/native/llrt/unwind.xo raise), on Windows the
// system's unwinder (decision 0197).
// Every frame of generated code has unwind tables, and its landing pads (the
// cleanup paths of internal/ssa's checks, landing.go) run the deferred
// code of the blocks the fault leaves, close their scopes, unlock their
// Mutexes, and release what the frame owns, then hand the fault on
// (xo_fault_continue raises it again from there). The fault is caught by
// the entry function of a task (it ends the task as faulted:
// tasks.xo task_unwound), by `task.isolate` (xo_fault_take), or, past the
// boundary of the context (the C function running the program's main),
// reported and the process exits with status 70. unwind.xo reads the
// call site table of the language specific data area that both backends
// emit (the GCC format for cleanups).
// Frames of C with cleanups between a fault and its catch are passed
// over, their cleanups not run (the runtime's C has none).

#define XO_EXC_CLASS 0x584f00004641554cULL // "XO\0\0FAUL"

// The fault record (faulting, fault_msg, fault_loc, fault_detail) is
// kept by std/native/llrt/fault.xo (the zero-C audit's B06).
_Static_assert(offsetof(xo_ctx, unwinding) == 8 && offsetof(xo_ctx, faulting) == 80 && offsetof(xo_ctx, fault_msg) == 88 &&
                   offsetof(xo_ctx, fault_loc) == 96 && offsetof(xo_ctx, fault_detail) == 104,
               "fault.xo C_*");
void xo_x_fault_record_set(int64_t c, int64_t mp, int64_t mn, int64_t lp, int64_t ln);
void xo_x_fault_record_exit(int64_t c);

// fault_exit reports the fault of c that nothing caught and ends the
// process with status 70 (fault.xo exit).
XO_NORETURN static void fault_exit(xo_ctx *c) {
	xo_x_fault_record_exit(XO_ADDR(c));
	__builtin_unreachable();
}

int64_t xo_x_unwind_fault(int64_t exc, int64_t boundary, int64_t pers, int64_t pers_old, int64_t wcx);
int64_t xo_x_personality(int64_t version, int64_t actions, int64_t cls, int64_t exc, int64_t uc);
#if defined(_WIN32)
EXCEPTION_DISPOSITION xo_personality(PEXCEPTION_RECORD rec, PVOID frame, PCONTEXT cx, PVOID disp);
#endif

// fault_raise on Linux, macOS, and Windows (decision 0221; Windows,
// decision 0197 and the zero-C audit's B06): the unwinder is Xo
// (std/native/llrt/unwind.xo over the .eh_frame_hdr tables of the
// program and its shared objects on Linux, the compact unwind of
// __unwind_info and __eh_frame on macOS, the system's RtlVirtualUnwind
// on Windows, win_raise), which runs the first landing pad of
// a frame of generated code (its personality is xo_x_personality; on
// Windows the language handler xo_personality too) below the context's
// boundary, and returns when nothing catches the fault.
XO_NORETURN static void fault_raise(xo_ctx *c) {
	rt_fault_leave();
	c->faulting = 1;
	c->unwinding = 1;
	// on Windows unwind.xo win_raise captures its own CONTEXT
	// (RtlCaptureContext from sys_windows.xo) and walks from there; the
	// language handler xo_personality marks generated code there too
#ifdef _WIN32
	xo_x_unwind_fault(XO_ADDR(ctx_exc(c)), (int64_t)c->boundary, XO_ADDR(xo_x_personality), XO_ADDR(xo_personality), 0);
#else
	xo_x_unwind_fault(XO_ADDR(ctx_exc(c)), (int64_t)c->boundary, XO_ADDR(xo_x_personality), XO_ADDR(xo_x_personality), 0);
#endif
	fault_exit(c);
}

// fault_set records a fault in c (fault.xo set).
static void fault_set(xo_ctx *c, const char *msg, const char *loc) {
	xo_x_fault_record_set(XO_ADDR(c), XO_ADDR(msg), (int64_t)strlen(msg), XO_ADDR(loc), (int64_t)strlen(loc));
}

// fault_raise_msg records the fault msg at loc in the current context and
// raises it; with a null msg it raises the fault already recorded (fault.xo
// cont, a landing pad handing it on).
XO_NORETURN static void fault_raise_msg(const char *msg, const char *loc) {
	xo_ctx *c = ctx();
	if (msg) {
		fault_set(c, msg, loc);
	}
	fault_raise(c);
}

// xo_personality: the Xo unwinder reads the call site tables itself and
// only compares a frame's personality with the address of the export
// xo_x_personality, which the emitters name (seed 0.13.0's too, so the
// C forwarder for older emitters is gone). With no C library the link
// defines xo_personality as the export. On Windows it is the language
// handler below.
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
// point. A task started under a budget works under it (tasks.xo task_spawn).

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

// The shields of deferred code (core 3.5, decision 0022) are ctx.xo's
// (decision 0229), which generated code calls through its exports.

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

// The worker records (xo_worker until decision 0233 step 2) and their
// run queues are std/native/llrt/sched.xo's and pool.xo's (W_*, RQ_*).

// The pool's setup (pool_init until B20) and the root threads' worker
// records are poolsetup.xo's ready and root_worker (tasks.xo
// park_wait_n calls it).

// The worker pool in Xo (decision 0212 stage 2): std/native/llrt/pool.xo
// through the C ABI exports of cabi.xo, over the records above (their
// layout is asserted below). The C keeps the records, their setup, the
// root threads' records, and the deadlock check; the Xo code calls back
// for the services in xo_sched_rt.

void xo_x_park_reset(int64_t p);
void xo_x_node_link(int64_t n, int64_t parent);
void xo_x_node_unlink(int64_t n);

static void park_reset(xo_park *p) { xo_x_park_reset(XO_ADDR(p)); }
static void node_link(xo_node *n, xo_node *parent) { xo_x_node_link(XO_ADDR(n), XO_ADDR(parent)); }
static void node_unlink(xo_node *n) { xo_x_node_unlink(XO_ADDR(n)); }

// netconn.xo's (cabi_net.xo): weak, as the other network exports (see
// XO_NX_REF below)
#if defined(__APPLE__) || defined(_WIN32)
#define XO_NC_REF
#else
#define XO_NC_REF __attribute__((weak))
#endif

// The deadlock check is pool.xo dl_check; the test world's clock move
// (twrun.xo kick) checks again after it.


// park_wait_n blocks the current context until its park is signaled or,
// when cancelable, its node is canceled (tasks.xo park_wait_n): a task on
// a fiber parks, a root thread runs queued work while it waits. counted
// tells that the wait counts as blocked for deadlock detection; a wait
// that a timer, the network, or a signal can end does not.
void xo_x_park_wait_n(int64_t c, int64_t cancelable, int64_t counted);
static void park_wait_n(xo_ctx *c, int cancelable, int counted) { xo_x_park_wait_n(XO_ADDR(c), cancelable, counted); }

// A wait list: the parks of the contexts waiting on an object, signaled
// all at once when its state changes (tasks.xo wl_*). The entry
// (xo_went, above) is the waiting context's own (xo_ctx.went).
typedef struct {
	xo_went *first, *last;
} xo_wl;

// ---------------------------------------------------------------------------
// Scopes and tasks

// A scope is tasks.xo's record (its S_* offsets, budget.xo's and
// poll.xo's copies): the C never reads one.
typedef struct xo_scope xo_scope;

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

// `s.group()`, the program's task group (os.tasks), and s.cancel() are
// tasks.xo's (B18), which generated code calls through its exports.

_Static_assert(sizeof(xo_node) == 56, "tasks.xo: the node's size");

// The task states in Xo (decision 0212, std/native/llrt/sched.xo).

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

// The fiber records (xo_fiber until decision 0233 step 2) are
// std/native/llrt/sched.xo's and pool.xo's (F_*).

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

#if defined(__has_attribute)
#if __has_attribute(disable_sanitizer_instrumentation)
#define XO_NO_SANITIZE __attribute__((disable_sanitizer_instrumentation))
#endif
#endif
#ifndef XO_NO_SANITIZE
#define XO_NO_SANITIZE
#endif


// ctx_init makes ctx.xo's services record (decision 0233 step 2), with
// the fiber cache's record and the statistics' (rtstats.xo).
static void ctx_init(void) {
#ifdef XO_TSAN_ANN
	xo_x_ctx_init(1);
#else
	xo_x_ctx_init(0);
#endif
}

// xo_sched_run_task runs the task of fiber f, which just started: the
// emitters call cabi.xo's fiber_run_task for rt.sched_run_task; this
// forwarder is for code of earlier compilers (delete after the next seed).
void xo_x_fiber_run_task(int64_t f);
void xo_sched_run_task(int64_t f) { xo_x_fiber_run_task(f); }

// xo_sched_rt is the C runtime's services for std/native/llrt/pool.xo
// (rt.sched_rt, decision 0212 stage 2): op is pool.xo's RT_*.

int64_t xo_sched_rt(int64_t op, int64_t a, int64_t b) {
	switch (op) {
	case 2: // the virtual clock's move (twrun.xo kick; pool.xo asks while it is virtual)
		xo_x_tw_kick();
		return 0;
	case 16: // task a, claimed, runs here (its queue entry still queued)
		TSAN_ACQUIRE((void *)(intptr_t)a);
		run_claimed((xo_task *)(intptr_t)a, b);
		return 0;
	case 29: // task a's entry function, with closure b (tasks.xo run_claimed)
		run_entry((xo_task *)(intptr_t)a, (xo_rlist *)(intptr_t)b);
		return 0;
	case 18: // the list a canceled operation returns
		return XO_ADDR(&empty_list);
	case 20: // the runtime switches to several threads (pool.xo's setup)
		xo_rt_threads_begin();
		return 0;
	}
	(void)b;
	return 0;
}

// The layout std/native/llrt/sched.xo reads and writes (decision 0212:
// its constants are these offsets).
_Static_assert(offsetof(xo_task, state) == 80 && offsetof(xo_task, gone) == 176, "sched.xo T_*");
_Static_assert(T_QUEUED == 0 && T_RUNNING == 1, "sched.xo states");
// and std/native/llrt/pool.xo (decision 0212 stage 2)
_Static_assert(sizeof(xo_lock) == 8, "pool.xo's locks are words");
_Static_assert(offsetof(xo_park, signaled) == 8 && offsetof(xo_park, sleeping) == 16 && offsetof(xo_park, counted) == 20 && offsetof(xo_park, fiber) == 24, "pool.xo PK_*");
_Static_assert(offsetof(xo_node, parent) == 0 && offsetof(xo_node, child) == 8 && offsetof(xo_node, prev) == 16 && offsetof(xo_node, next) == 24 &&
                   offsetof(xo_node, canceled) == 32 && offsetof(xo_node, waiter) == 40 && offsetof(xo_node, mu) == 48,
               "pool.xo N_*");
_Static_assert(offsetof(xo_ctx, node) == 0 && offsetof(xo_ctx, shield) == 12 && offsetof(xo_ctx, park) == 16, "pool.xo C_*");
_Static_assert(offsetof(xo_task, h) == 0 && offsetof(xo_task, node) == 16 && offsetof(xo_task, scope) == 72 && offsetof(xo_task, snext) == 168 && T_DONE == 2, "pool.xo T_*");

// The spawn and the scope's open and close are tasks.xo's (B18 part 2),
// which generated code calls through its exports.

_Static_assert(offsetof(xo_task, run) == 88 && offsetof(xo_task, drop) == 96 && offsetof(xo_task, fl) == 104 &&
                   offsetof(xo_task, background) == 124 && offsetof(xo_task, snext) == 168 && offsetof(xo_task, bud) == 208 &&
                   sizeof(xo_task) == 216 && offsetof(xo_hdr, free) == 8,
               "tasks.xo T_*, H_FREE");

// The task's end, await, and the scope's failed task are tasks.xo's
// (B18), which generated code calls through its exports.

_Static_assert(offsetof(xo_task, scope) == 72 && offsetof(xo_task, result) == 112 && offsetof(xo_task, unwound) == 120 &&
                   offsetof(xo_task, consumed) == 128 && offsetof(xo_task, site) == 136 && offsetof(xo_task, mu) == 144 &&
                   offsetof(xo_task, waiters) == 152 && offsetof(xo_task, fault_msg) == 184 && offsetof(xo_task, fault_loc) == 192 &&
                   offsetof(xo_task, fault_detail) == 200,
               "tasks.xo T_*");

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
// through its exports (task.stopping's channel too: stop.xo through
// cabi.xo's stop_chan_new; the C tests through xcalls.h). chan_free stays
// C (the free function the handle names).

_Static_assert(offsetof(xo_cent, fired) == 8 && offsetof(xo_cent, item) == 16 && offsetof(xo_cent, shut) == 24 && offsetof(xo_cent, idx) == 32 &&
                   offsetof(xo_cent, chosen) == 40 && offsetof(xo_cent, prev) == 48 && offsetof(xo_cent, next) == 56 && sizeof(xo_cent) == 64,
               "chan.xo E_*");
_Static_assert(offsetof(xo_chan, mu) == 16 && offsetof(xo_chan, cap) == 24 && offsetof(xo_chan, head) == 32 && offsetof(xo_chan, len) == 40 &&
                   offsetof(xo_chan, buf) == 48 && offsetof(xo_chan, closed) == 56 && offsetof(xo_chan, ext) == 60 && offsetof(xo_chan, drop) == 64 &&
                   offsetof(xo_chan, recvq) == 72 && offsetof(xo_chan, sendq) == 88 && sizeof(xo_chan) == 104,
               "chan.xo CH_*");

// select (core 7.3): arms are added in source order, then
// selects.xo select_wait performs the first arm that can proceed, or waits for
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

// The select's operations are selects.xo's (B19), which generated code
// calls through its exports. select_free stays C (the free function the handle names); the `after`
// arm's timer is poll.xo's (FN_SELECT).

_Static_assert(offsetof(xo_sarm, send) == 8 && offsetof(xo_sarm, item) == 16 && offsetof(xo_sarm, e) == 24 && sizeof(xo_sarm) == 88 &&
                   offsetof(xo_select, arms) == 16 && offsetof(xo_select, n) == 24 && offsetof(xo_select, cap) == 28 &&
                   offsetof(xo_select, fired) == 32 && offsetof(xo_select, chosen) == 40 && offsetof(xo_select, has_after) == 48 &&
                   offsetof(xo_select, after) == 56 && offsetof(xo_select, park) == 64 && sizeof(xo_select) == 72,
               "selects.xo A_*, SE_*; poll.xo SEL_*");

// ---------------------------------------------------------------------------
// Mutex[T] (core 5.4, std.md 2.7): a readers and writer lock with
// writers preferred, holding its value in a shared list of one.

// A Mutex is mutex.xo's record (its M_* offsets): the C never reads one.
typedef struct xo_mutex xo_mutex;

// The Mutex's operations are mutex.xo's (B19), which generated code calls
// through its exports. mutex_free stays C (the free function the handle names, through the services
// record).

_Static_assert(offsetof(xo_went, granted) == 28, "mutex.xo W_GRANTED");

// ---------------------------------------------------------------------------
// Atomic[T] for Int and Bool (a Bool is 0 or 1)
//
// The generated code reads and writes the value itself (decision 0200):
// get, set, swap, and compare_and_swap are one inline atomic each on
// byte 128 (slot 13 of rt.slot_load and friends, which count a list's
// three word header), and add and max are std/native/llrt/atomic.xo.
// The C keeps the allocation (decision 0198 deleted the C path).

// An Atomic is tasks.xo's record (atomic_new: 136 bytes, the value at
// byte 128, XO_ATOMIC_V): the C never reads one.
typedef struct xo_atomic xo_atomic;
#define XO_ATOMIC_V 128

// ---------------------------------------------------------------------------
// The event loop (decision 0069): timers, sockets, and signals
//
// The event loop is std/native/llrt/poll.xo's (B22): its record, the
// timers, the descriptors, the poller thread on every OS (kqueue on
// Darwin, epoll on Linux, AFD polls through a completion port or the
// WSAPoll fallback on Windows). What stays here: the signal handlers
// (on_signal, on_console: async signal safe, they count the signal in
// the record and wake the poller), and the services the poller calls
// back (pd_svc). The records' layouts are poll.xo's own (EV_*, D_*, T_*;
// asserted in C until 2026-10-15).

#if defined(_WIN32)
// the AFD polls' NtDeviceIoControlFile and NtCreateFile (poll.xo)
#pragma comment(lib, "ntdll")
#endif

// ev is the event loop's record, which poll.xo makes (poll_svc): Windows'
// console handler counts a signal in its word EV_SIGS (atomic) and wakes
// the poller (poll.xo on_signal does both on Linux and macOS).
static int64_t *ev;
enum { EV_SIGS_W = 5 };

// The network's system calls (decision 0206 stage 3, the zero-C audit's
// B23): std/native/llrt/netconn.xo makes the network's (listen, dial,
// accept, reads and writes) from Xo, and generated code calls
// cabi_net.xo's exports for them. The handles' free functions, which a
// program carries when it dials, listens, or uses std/http
// (cabi_net.xo), and proc.xo's child free function are weak references
// everywhere (XO_NC_REF, above the services record); Mach-O takes no
// undefined weak reference (and PE/COFF's are weak externals with a
// default), so weak definitions stand in there until the exports
// replace them.
XO_NC_REF void xo_x_nc_listener_free(int64_t l);
XO_NC_REF void xo_x_nc_conn_free(int64_t cn);
#if (defined(__APPLE__) || defined(_WIN32)) && !defined(XO_NOLIBC)
#define XO_NX_STUB(name, ...)                                                                                                 \
	__attribute__((weak)) int64_t name(__VA_ARGS__) { return -ENOSYS; }
#define XO_NX_VSTUB(name, ...)                                                                                                \
	__attribute__((weak)) void name(__VA_ARGS__) {}
XO_NX_VSTUB(xo_x_nc_listener_free, int64_t a)
XO_NX_VSTUB(xo_x_nc_conn_free, int64_t a)
#endif

// ev_poke wakes the poller (a new first timer): poll.xo poke (the C
// tests and Windows' console handler call this).
void xo_x_poll_poke(void);
static void ev_poke(void) { xo_x_poll_poke(); }

// ev_start starts the event loop once (poll.xo ev_start).
void xo_x_poll_ev_start(void);
static void ev_start(void) { xo_x_poll_ev_start(); }


// Socket descriptors are poll.xo's (B22: registration, operations,
// close, waits, and on Windows the AFD poll's arm); the C keeps none.
#ifdef _WIN32
// poll.xo reads the completions GetQueuedCompletionStatusEx returns
_Static_assert(sizeof(OVERLAPPED_ENTRY) == 32 && offsetof(OVERLAPPED_ENTRY, lpOverlapped) == 8, "poll.xo's OVERLAPPED_ENTRY");
#endif

// The poller thread

// pd_svc is poll.xo's services record (its S_ words): the wait it calls
// through rt.call_c2 (park_wait_n), the event loop's lock word and dead list and
// record, the offset of a context's park, and where the poller thread
// starts (Windows' start, registration, and loops are poll.xo's too
// since 2026-10-15).
static int64_t pdsvc_wait(int64_t c, int64_t b) {
	(void)b;
	park_wait_n((xo_ctx *)(intptr_t)c, 1, 0);
	return 0;
}
int64_t xo_x_poll_thread(int64_t a);
static int64_t pd_svc[17];
int64_t xo_x_poll_svc(int64_t rec);
static void pd_svc_init(void) {
	pd_svc[4] = XO_ADDR(&pdsvc_wait);
	pd_svc[10] = (int64_t)offsetof(xo_ctx, park);
	pd_svc[14] = XO_ADDR(&xo_x_poll_thread);
	ev = (int64_t *)(intptr_t)xo_x_poll_svc(XO_ADDR(pd_svc));
}

// The poller thread's loop is std/native/llrt/poll.xo's (poll_thread,
// poll_main on Linux and macOS, B22; poll_main_win over Windows' AFD
// polls, stage 4; poll_main_wsa, Windows' WSAPoll fallback): it frees the
// dead descriptors, fires the timers that are due, waits for readiness,
// hands it to the descriptors, and runs the signals' shutdown steps
// (stop.xo signal).

// ---------------------------------------------------------------------------
// Clock (std.md 3.3): Time is wall clock ns since the Unix epoch, Duration
// ns (the Go backend's representation)

// ---------------------------------------------------------------------------
// Network (std.md 3.2): TCP listeners and connections are
// std/native/llrt/netconn.xo's (the zero-C audit's B22 and B23
// remnants), through cabi_net.xo's exports, which generated code calls
// directly (selfhost/ssa/exports.xo). Each operation records its outcome
// (cabi_net.xo nc_status, nc_msg), which the generated code reads to
// build the NetErr: 1 InvalidAddr, 2 Refused, 3 Timeout, 4 Io(msg),
// 5 Canceled, 6 Denied (the Go backend's tags). A handle is the zero
// handle, and a list the empty one, when netconn.xo returns 0. The
// records below are the evloop.c test's view of the handles.

typedef struct {
	xo_hdr h;
	void *pd; // poll.xo's descriptor record
	char *addr;
	void *mem; // the test world's in memory listener (twrun.xo), or NULL
	int unlink_path; // a Unix domain socket whose file close removes (decision 0190)
} xo_listener;
_Static_assert(offsetof(xo_listener, pd) == 16 && offsetof(xo_listener, addr) == 24 && offsetof(xo_listener, mem) == 32 &&
                   offsetof(xo_listener, unlink_path) == 40 && sizeof(xo_listener) == 48,
               "netconn.xo L_*");

typedef struct {
	xo_hdr h;
	void *pd;
} xo_conn;
_Static_assert(offsetof(xo_conn, pd) == 16 && sizeof(xo_conn) == 24, "netconn.xo C_*");

// The process environment, which C main hands to process.xo
// (xo_x_proc_start below).
#ifdef _WIN32
#undef environ // the C runtime's (stdlib.h): process.xo reads Windows' own
#define environ ((char **)0)
#else
extern char **environ;
#endif

// ---------------------------------------------------------------------------
// Signals and two phase shutdown (core 7.5, decision 0042): the state
// (the signals counted, the stop request, the grace timer, task.stopping's
// channel, main's root context) is std/native/llrt/stop.xo's (the zero-C
// audit's B26, part 1). The C keeps the handlers, which count a signal
// and wake the poller (whose thread calls stop.xo signal), and stop_svc,
// the services record whose words 88 on entry.xo reads (its E_ words).

void xo_x_stop_init(int64_t svc);
void xo_x_stop_set_root(int64_t n);
void xo_x_stop_end(void);


#ifndef _WIN32
// SIGINT and SIGTERM's handler is poll.xo's (cabi.xo on_signal).
void xo_x_on_signal(int64_t sig);
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
		__atomic_fetch_add(&ev[EV_SIGS_W], 1, __ATOMIC_ACQ_REL);
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
void xo_x_consts_free(void);
#ifdef XO_TIME
void xo_x_tz_free(void);
#endif
static int64_t entrysvc_mid_max(int64_t m, int64_t b) {
	(void)b;
	xo_mid_max = m;
	main_heap.mid_max = m;
	return 0;
}

int64_t xo_x_rtstats_counts(void);
int64_t xo_x_rtstats_switch(void);
static int64_t stop_svc[20];
static void stop_svc_init(void) {
	stop_svc[11] = XO_ADDR(&xo_x_tw_start);
	stop_svc[12] = XO_ADDR(&ev_start);
#ifdef _WIN32
	stop_svc[13] = XO_ADDR(&on_console);
	stop_svc[14] = XO_ADDR(&on_overflow);
#else
	stop_svc[13] = XO_ADDR(&xo_x_on_signal);
#endif
	stop_svc[15] = XO_ADDR(&entrysvc_mid_max);
	stop_svc[16] = XO_ADDR(&xo_x_consts_free);
#ifdef XO_TIME
	stop_svc[17] = XO_ADDR(&xo_x_tz_free);
#endif
	stop_svc[18] = xo_x_rtstats_counts();
	stop_svc[19] = xo_x_rtstats_switch();
	xo_x_stop_init(XO_ADDR(stop_svc));
}

// xo_set_grace is `proc.set_grace(d)` (the SSA builders' rt.set_grace):
// after a stop request the deadline moves to the stop time plus d.
void xo_x_stop_set_grace(int64_t d);
void xo_set_grace(int64_t d) { xo_x_stop_set_grace(d); }

// ---------------------------------------------------------------------------
// Entry (left out with -DXO_RT_NO_MAIN: the C tests of std/llrt/testdata
// include this file and bring their own main)

#ifndef XO_RT_NO_MAIN
extern void xo_entry(void);

static void *run_main(void *arg) {
	(void)arg;
#ifdef _WIN32
	// the stack the thread keeps for its stack overflow handler
	// (on_overflow), past the guard page
	ULONG g = 64 << 10;
	SetThreadStackGuarantee(&g);
#endif
	// main's root context: the second shutdown signal cancels it
	xo_x_stop_set_root(XO_ADDR(ctx()->node));
	// a fault that unwinds main reaches this frame: reported, status 70
	// (decision 0085)
	ctx()->boundary = (uintptr_t)__builtin_frame_address(0);
	xo_entry();
	return NULL;
}

// std/http's streamed responses and idle HTTP/2 client connections
// (cabi_http.xo; weak in xo_http.c)
void xo_x_streams_free(void);

// the program's start and end (std/native/llrt/entry.xo)
void xo_x_entry_start(int64_t argc, int64_t argv, int64_t envp);
void xo_x_entry_finish(void);
int64_t xo_x_main_threads(int64_t f, int64_t n);
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
	// and env.vars read them in Xo; on Windows process.xo takes the
	// environment from Windows in UTF-8), XO_HEAP_MID_MAX, then the test
	// world (XO_TEST) or the signal handlers
	xo_x_entry_start(argc, (int64_t)(intptr_t)argv, (int64_t)(intptr_t)environ);
	// main runs on a thread of its own with a 1 GiB stack, which pool.xo
	// thread_run starts and waits for
#ifdef XO_RT_TEST_THREADS
	// N copies of the program at once: every thread has its own heap,
	// literals are shared (and never written)
	xo_rt_threads_begin();
	if (!xo_x_main_threads(XO_ADDR(run_main), XO_RT_TEST_THREADS)) {
		fputs("pthread_create failed\n", stderr);
		return 1;
	}
#else
	if (!xo_x_main_threads(XO_ADDR(run_main), 1)) {
		run_main(NULL);
	}
#endif
	xo_flush();
	xo_x_stop_end();
	xo_x_rt_stats();
	xo_x_streams_free();
	// the values the test world keeps (twrun.xo end; nothing outside it)
	xo_x_tw_end();
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
