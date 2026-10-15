// xo_rt.c: the C left of the native runtime (the runtime is Xo:
// std/native/llrt, decisions 0206 and 0233). What stays needs C: main
// (the C library's start calls it), the fault raise's frame and the
// personality addresses, Windows' stack overflow handler and binary
// streams, the services record's C words, ThreadSanitizer's annotation,
// the immortal placeholders generated code takes the address of, and
// forwarders that older compilers' code still names. It is compiled with
// the generated code (clang, -flto on LLVM); its interface is scalars
// and pointers only.

#ifdef _WIN32
// Windows (decision 0197): the Win32 and Winsock headers, with the MSVC
// C runtime and the Windows SDK
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
// xo_nolibc.c's FILE, va_list and exit
#include <stdarg.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#endif
#include <stddef.h>

#ifdef _WIN32
// the Windows constants and layouts std/native/llrt sock.xo and
// cabi_net.xo hard code
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
// Memory: the heap is Xo (alloc.xo, obj.xo, heapos.xo through cabi.xo's
// exports); heap_start makes the main thread's record before anything
// allocates.

void xo_x_heap_start(void);

// The C tests of internal/llvm (XO_RT_NO_MAIN) bring their own main: a
// constructor does main's setup there (xo's own ELF linker takes no
// .init_array, so main does it otherwise:
// gotchas/runtime-c-no-constructors.md).
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

// rt.heap_large and friends are libcmem.xo's (cabi.xo heap_large);
// these forward for code of compilers whose emitters named the C (the
// 0.13.0 seed's).
int64_t xo_x_heap_large(int64_t n);
int64_t xo_x_heap_large_resize(int64_t b, int64_t n);
void xo_x_heap_release(int64_t b);
int64_t xo_heap_large(int64_t n) { return xo_x_heap_large(n); }
int64_t xo_heap_large_resize(int64_t b, int64_t n) { return xo_x_heap_large_resize(b, n); }
void xo_heap_release(int64_t b) { xo_x_heap_release(b); }

// the chunks mapped so far (heapos.xo), which tasks.c and
// alloc_threads.c check
int64_t xo_x_heap_chunks(void);

#define xo_chunks_now() xo_x_heap_chunks()

// ---------------------------------------------------------------------------
// Reference counts (decisions 0046 and 0058) are rc.xo's: a count c > 0 is
// thread local, -n is shared by n, XO_RC_IMMORTAL marks a literal (never
// written). rc_inc and rc_dec are the C tests'.

#define XO_RC_IMMORTAL INT64_MIN

static inline int64_t rc_load(int64_t *p) { return __atomic_load_n(p, __ATOMIC_RELAXED); }

void xo_x_rc_inc(int64_t p);
int64_t xo_x_rc_dec(int64_t p);

static inline __attribute__((always_inline)) void rc_inc(int64_t *p) { xo_x_rc_inc((int64_t)(intptr_t)p); }
static inline __attribute__((always_inline)) int rc_dec(int64_t *p) { return (int)xo_x_rc_dec((int64_t)(intptr_t)p); }

// ---------------------------------------------------------------------------
// Output is stdio.xo's; xo_flush is the C name std's rt.flush lowers to.

void xo_x_flush(void);
void xo_x_flush_try(void);

void xo_flush(void) { xo_x_flush(); }

// an address as the int64_t the C ABI exports (cabi.xo) take
#define XO_ADDR(p) ((int64_t)(intptr_t)(p))

// ---------------------------------------------------------------------------
// Faults (core 8): fault.xo writes the message and raises through
// cabi.xo fault_raise_msg; fault_init (below the personality routines)
// has fault.xo, ctx.xo, poll.xo and stop.xo start.
static void fault_init(void);

// ---------------------------------------------------------------------------
// Strings and lists: the layouts the C tests use; a list's slow paths
// (obj.xo's) for LLVM generated code, which LTO folds into the caller.

typedef struct {
	int64_t rc, cap;
	char data[];
} xo_rstr;

typedef struct {
	int64_t rc, len, cap;
} xo_rlist;

typedef void (*xo_elem_fn)(void *);

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
// Tasks (decisions 0068, 0069, 0077, 0212): scopes, tasks, channels,
// select, Mutex, Atomic, the worker pool, fibers and the scheduler are
// Xo (tasks.xo, chan.xo, selects.xo, mutex.xo, pool.xo, sched.xo), their
// records laid out there only. A handle is {count, free, ...}.

typedef void (*xo_free_fn)(void *);

typedef struct {
	int64_t rc;
	xo_free_fn free;
} xo_hdr;

// the immortal placeholders a canceled operation returns; generated code
// takes their addresses (rt.rl_empty, rt.h_zero)
xo_hdr xo_zero_handle = {XO_RC_IMMORTAL, NULL};
xo_rlist xo_empty_list = {XO_RC_IMMORTAL, 0, 0};
#define zero_handle xo_zero_handle
#define empty_list xo_empty_list

// ThreadSanitizer does not see the Xo code's synchronization (it is not
// instrumented): a task claimed from a queue acquires its spawn's release
// (decision 0212 stage 2).
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
#define TSAN_ACQUIRE(a) __tsan_acquire(a)
#else
#define TSAN_ACQUIRE(a) ((void)(a))
#endif

// ---------------------------------------------------------------------------
// Fault unwinding (decisions 0085, 0221): cabi.xo's fault_raise_msg
// and fault_raise_cur record the fault and unwind with unwind.xo up to
// the context's boundary, with two personality routines: xo_x_personality,
// which the emitters name, and on Windows the language handler below.
int64_t xo_x_personality(int64_t version, int64_t actions, int64_t cls, int64_t exc, int64_t uc);
#if defined(_WIN32)
EXCEPTION_DISPOSITION xo_personality(PEXCEPTION_RECORD rec, PVOID frame, PCONTEXT cx, PVOID disp);
#define XO_PERS_OLD XO_ADDR(xo_personality)
#else
#define XO_PERS_OLD XO_ADDR(xo_x_personality)
#endif

// fault_init starts fault.xo and ctx.xo (cabi.xo rt_start) with the
// second personality routine, then the services records of poll.xo and
// stop.xo.
void xo_x_rt_start(int64_t pers_old);
static void pd_svc_init(void);
static void stop_svc_init(void);
static void fault_init(void) {
	xo_x_rt_start(XO_PERS_OLD);
	pd_svc_init();
	stop_svc_init();
}

#if defined(_WIN32)
// Windows' exception dispatcher calls it for frames of generated code;
// faults never go through the dispatcher, so it passes every exception on
EXCEPTION_DISPOSITION xo_personality(PEXCEPTION_RECORD rec, PVOID frame, PCONTEXT cx, PVOID disp) {
	(void)rec;
	(void)frame;
	(void)cx;
	(void)disp;
	return ExceptionContinueSearch;
}
#endif

// ---------------------------------------------------------------------------
// The scheduler's C services (rt.sched_rt, pool.xo RT_*): a claimed
// task's run with ThreadSanitizer's annotation, and the empty list.

void xo_x_run_claimed(int64_t t, int64_t units);

static void run_claimed(int64_t t, int64_t units) { xo_x_run_claimed(t, units); }

// the C name of cabi.xo fiber_run_task for code of compilers before the
// emitters named the export (the 0.13.0 seed's)
void xo_x_fiber_run_task(int64_t f);
void xo_sched_run_task(int64_t f) { xo_x_fiber_run_task(f); }

int64_t xo_sched_rt(int64_t op, int64_t a, int64_t b) {
	switch (op) {
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

// an Atomic's value: byte 128 of tasks.xo's record (decision 0200; the
// C tests read it)
#define XO_ATOMIC_V 128

// ---------------------------------------------------------------------------
// The event loop is poll.xo's (B22): pd_svc_init has it make its record;
// ev_poke and ev_start are the C tests' and the entry's.

#if defined(_WIN32)
// the AFD polls' NtDeviceIoControlFile and NtCreateFile (poll.xo)
#pragma comment(lib, "ntdll")
#endif

void xo_x_poll_poke(void);
static void ev_poke(void) { xo_x_poll_poke(); }

void xo_x_poll_ev_start(void);
static void ev_start(void) { xo_x_poll_ev_start(); }

#ifdef _WIN32
// poll.xo reads the completions GetQueuedCompletionStatusEx returns
_Static_assert(sizeof(OVERLAPPED_ENTRY) == 32 && offsetof(OVERLAPPED_ENTRY, lpOverlapped) == 8, "poll.xo's OVERLAPPED_ENTRY");
#endif

int64_t xo_x_poll_svc(void);
static void pd_svc_init(void) {
	(void)xo_x_poll_svc();
}

// ---------------------------------------------------------------------------
// The network is netconn.xo's (cabi_net.xo); these records are the
// evloop.c test's view of its handles.

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

// the environment, which main hands to process.xo (on Windows process.xo
// reads Windows' own)
#ifdef _WIN32
#undef environ // the C runtime's (stdlib.h): process.xo reads Windows' own
#define environ ((char **)0)
#else
extern char **environ;
#endif

// ---------------------------------------------------------------------------
// Two phase shutdown (core 7.5) is stop.xo's; stop_svc holds the C words
// entry.xo reads (its E_ words, 88 on), and the handlers below.

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

// functions that take no arguments are called through rt.call_c2 as they
// are (its two arguments are ignored on every target's C convention)
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
	// word 11 (the test world's start) is unused: a test binary's main
	// starts it (the native prelude's llrt_tw_start)
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
