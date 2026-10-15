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
// The heap is Xo (decisions 0058, 0118, 0198, 0207; decision 0233 step 2
// moved the last of it): std/native/llrt/alloc.xo (the size classes,
// free lists, magazines and central lists), obj.xo (the heap words, the
// deferred frees) and heapos.xo (the records, one per thread, and their
// mappings), through cabi.xo's exports. heap_start makes the main record
// before anything allocates; heap_threads switches to one record per
// thread before a second thread runs Xo code.

void xo_x_heap_start(void);

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
	xo_x_heap_start();
	// the environment for the runtime's settings (poolsetup.xo reads
	// XO_THREADS)
	xo_x_proc_start(0, 0, (int64_t)(intptr_t)environ);
	fault_init();
}
#endif

// The runtime's records (std/native rt.heap_large) are libcmem.xo's
// (cabi.xo heap_large; the zero-C inventory's rank 10, 2026-10-15), not
// the C library's malloc: these forward for code of compilers before
// the emitters named the exports (seed 0.12.0).
int64_t xo_x_heap_large(int64_t n);
int64_t xo_x_heap_large_resize(int64_t b, int64_t n);
void xo_x_heap_release(int64_t b);
int64_t xo_heap_large(int64_t n) { return xo_x_heap_large(n); }
int64_t xo_heap_large_resize(int64_t b, int64_t n) { return xo_x_heap_large_resize(b, n); }
void xo_heap_release(int64_t b) { xo_x_heap_release(b); }

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
// Parks, nodes, contexts, locks: their records are Xo's (pool.xo PK_*
// and N_*, ctx.xo C_*, tasks.xo, chan.xo, selects.xo, budget.xo), laid
// out and read there only; the C keeps no copy of their layouts (the
// zero-C inventory's rank 13, 2026-10-15). What stays: ThreadSanitizer's
// annotation of a claimed task (xo_sched_rt).

// ThreadSanitizer does not see the synchronization of the Xo code (it is
// not instrumented): the C tells it that a task claimed from a queue
// acquires its spawn's release (decision 0212 stage 2).
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

// Raising a fault is cabi.xo's fault_raise_cur (the record, the
// context's flags, unwind.xo's walk, the report: the zero-C inventory's
// rank 13); fault_raise_msg, whose address fault.xo's raisers call, gives
// it the personality routines' addresses: xo_x_personality, which the
// emitters name, and on Windows the language handler xo_personality.
void xo_x_fault_raise_cur(int64_t m, int64_t loc, int64_t pers, int64_t pers_old);
int64_t xo_x_personality(int64_t version, int64_t actions, int64_t cls, int64_t exc, int64_t uc);
#if defined(_WIN32)
EXCEPTION_DISPOSITION xo_personality(PEXCEPTION_RECORD rec, PVOID frame, PCONTEXT cx, PVOID disp);
#define XO_PERS_OLD XO_ADDR(xo_personality)
#else
#define XO_PERS_OLD XO_ADDR(xo_x_personality)
#endif

// fault_raise_msg records the fault msg at loc in the current context and
// raises it; with a null msg it raises the fault already recorded (fault.xo
// cont, a landing pad handing it on).
XO_NORETURN static void fault_raise_msg(const char *msg, const char *loc) {
	xo_x_fault_raise_cur(XO_ADDR(msg), XO_ADDR(loc), XO_ADDR(xo_x_personality), XO_PERS_OLD);
	__builtin_unreachable();
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

// The budget record is std/native/llrt/budget.xo's (B_*).

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
// through the C ABI exports of cabi.xo, over records that are Xo's too;
// the Xo code calls back for the services in xo_sched_rt.

// netconn.xo's (cabi_net.xo): weak, as the other network exports (see
// XO_NX_REF below)
#if defined(__APPLE__) || defined(_WIN32)
#define XO_NC_REF
#else
#define XO_NC_REF __attribute__((weak))
#endif

// The deadlock check is pool.xo dl_check; the test world's clock move
// (twrun.xo kick) checks again after it.



// ---------------------------------------------------------------------------
// Scopes and tasks

// A scope and a task are tasks.xo's records (S_*, T_*); the C never
// reads one.

// The task states in Xo (decision 0212, std/native/llrt/sched.xo).

void xo_x_run_claimed(int64_t t, int64_t units);

// run_claimed runs t, which the caller claimed, on this stack (a fiber,
// or a thread that helps): tasks.xo run_claimed (B18), with its entry
// call (tasks.xo run_entry, decision 0233).
static void run_claimed(int64_t t, int64_t units) { xo_x_run_claimed(t, units); }

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
		run_claimed(a, b);
		return 0;
	case 18: // the list a canceled operation returns
		return XO_ADDR(&empty_list);
	}
	(void)b;
	return 0;
}

// The spawn, the scope's open and close, the task's end and await are
// tasks.xo's (B18), which generated code calls through its exports.

// ---------------------------------------------------------------------------
// Channels (core 7.3)

// A channel and its queue entries are chan.xo's records (CH_*, E_*);
// its operations are chan.xo's (B19), which generated code calls through
// its exports (task.stopping's channel too: stop.xo through cabi.xo's
// stop_chan_new; the C tests through xcalls.h).

// select (core 7.3): arms are added in source order, then
// selects.xo select_wait performs the first arm that can proceed, or waits for
// one (a cancellation point that unwinds). While it decides, and while
// it takes its arms off the queues, it holds the locks of all its
// channels (taken in address order), so no waker sees a half registered
// select.

// A select and its arms are selects.xo's records (SE_*, A_*); its
// operations are selects.xo's (B19), which generated code calls through
// its exports; the `after` arm's timer is poll.xo's (FN_SELECT).

// ---------------------------------------------------------------------------
// Mutex[T] (core 5.4, std.md 2.7): a readers and writer lock with
// writers preferred, holding its value in a shared list of one.

// A Mutex is mutex.xo's record (M_*), its operations mutex.xo's (B19),
// which generated code calls through its exports.

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
#define XO_ATOMIC_V 128

// ---------------------------------------------------------------------------
// The event loop (decision 0069): timers, sockets, and signals
//
// The event loop is std/native/llrt/poll.xo's (B22): its record, the
// timers, the descriptors, the poller thread on every OS (kqueue on
// Darwin, epoll on Linux, AFD polls through a completion port or the
// WSAPoll fallback on Windows), and the signal handlers (poll.xo
// on_signal and on_console). The records' layouts are poll.xo's own (EV_*, D_*, T_*;
// asserted in C until 2026-10-15).

#if defined(_WIN32)
// the AFD polls' NtDeviceIoControlFile and NtCreateFile (poll.xo)
#pragma comment(lib, "ntdll")
#endif

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
// tests call this).
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

// pd_svc_init has poll.xo make the event loop's record (poll_svc); poll.xo's services record went in
// decision 0233 step 2 (its wait, a context's park offset, the poller
// thread's start, cancellation, park signals and the shutdown's steps
// are Xo).
int64_t xo_x_poll_svc(void);
static void pd_svc_init(void) {
	(void)xo_x_poll_svc();
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


#ifndef _WIN32
// SIGINT and SIGTERM's handler is poll.xo's (cabi.xo on_signal).
void xo_x_on_signal(int64_t sig);
#endif

#ifdef _WIN32
// Ctrl-C, Ctrl-Break, and closing the console are Windows' SIGINT and
// SIGTERM: poll.xo on_console counts them and wakes the poller.
int64_t xo_x_on_console(int64_t kind);

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
void xo_x_heap_set_mid_max(int64_t m);
static int64_t entrysvc_mid_max(int64_t m, int64_t b) {
	(void)b;
	xo_x_heap_set_mid_max(m);
	return 0;
}

int64_t xo_x_rtstats_counts(void);
int64_t xo_x_rtstats_switch(void);
static int64_t stop_svc[20];
static void stop_svc_init(void) {
	stop_svc[11] = XO_ADDR(&xo_x_tw_start);
	stop_svc[12] = XO_ADDR(&ev_start);
#ifdef _WIN32
	stop_svc[13] = XO_ADDR(&xo_x_on_console);
	stop_svc[14] = XO_ADDR(&on_overflow);
#else
	stop_svc[13] = XO_ADDR(&xo_x_on_signal);
#endif
	stop_svc[15] = XO_ADDR(&entrysvc_mid_max);
	stop_svc[16] = XO_ADDR(&xo_x_consts_free);
	// word 17 (std/time's tz_free) is atexit.xo's since decision 0233
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

// the program (std/native/llrt/entry.xo main: the start, main's thread,
// the end, decision 0233)
int64_t xo_x_entry_main(int64_t argc, int64_t argv, int64_t envp, int64_t entry, int64_t n);

#ifdef XO_RT_TEST_THREADS
#define XO_MAIN_THREADS XO_RT_TEST_THREADS
#else
#define XO_MAIN_THREADS 1
#endif

int main(int argc, char **argv) {
	xo_x_heap_start();
	fault_init();
#ifdef _WIN32
	// standard streams in binary mode (no \n to \r\n)
	_setmode(0, _O_BINARY);
	_setmode(1, _O_BINARY);
	_setmode(2, _O_BINARY);
#endif
	return (int)xo_x_entry_main(argc, (int64_t)(intptr_t)argv, (int64_t)(intptr_t)environ, XO_ADDR(xo_entry), XO_MAIN_THREADS);
}
#endif

// The C library functions of a Linux program linked with no C library
// (XO_NOLIBC=1 builds, decisions 0203 and 0206).
#ifdef XO_NOLIBC
#include "xo_nolibc.c"
#endif
