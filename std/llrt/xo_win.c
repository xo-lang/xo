// xo_win.c: the POSIX subset the runtime uses, for Windows (decision
// 0197). xo_rt.c includes it instead of the POSIX headers when _WIN32 is
// defined (clang --target=x86_64-pc-windows-msvc or aarch64-pc-windows-msvc,
// with the MSVC C runtime and the Windows SDK). What is left (the zero-C
// audit's B32, 2026-10-15):
//
//   - threads: pthread_create/join on _beginthreadex (main's thread in
//     xo_rt.c run_main and sanhook.xo's seed shims);
//   - Winsock's constants and layouts, asserted for sock.xo and
//     cabi_net.xo (the sockets, the poller, and their errors are Xo:
//     sock.xo, poll.xo).
//
// Faults unwind with the Windows unwinder (RtlVirtualUnwind, xo_rt.c)
// instead of _Unwind_ForcedUnwind, which the MSVC environment does not
// provide.
#ifndef XO_WIN_C
#define XO_WIN_C

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

typedef intptr_t ssize_t;

// ---------------------------------------------------------------------------
// Threads

typedef HANDLE pthread_t;
#define PTHREAD_CREATE_DETACHED 1
#define PTHREAD_CREATE_JOINABLE 0

typedef struct {
	size_t stack;
	int detached;
} pthread_attr_t;

static inline int pthread_attr_init(pthread_attr_t *a) {
	a->stack = 0;
	a->detached = 0;
	return 0;
}
static inline int pthread_attr_destroy(pthread_attr_t *a) {
	(void)a;
	return 0;
}
static inline int pthread_attr_setstacksize(pthread_attr_t *a, size_t n) {
	a->stack = n;
	return 0;
}
static inline int pthread_attr_setdetachstate(pthread_attr_t *a, int d) {
	a->detached = d;
	return 0;
}

typedef struct {
	void *(*fn)(void *);
	void *arg;
} xo_win_start;

// the stack a thread keeps for its stack overflow handler (on_overflow,
// xo_rt.c), past the guard page
#define XO_W_GUARANTEE (64 << 10)

static unsigned __stdcall xo_thread_tramp(void *p) {
	xo_win_start s = *(xo_win_start *)p;
	free(p);
	ULONG g = XO_W_GUARANTEE;
	SetThreadStackGuarantee(&g);
	s.fn(s.arg);
	return 0;
}

static int pthread_create(pthread_t *t, const pthread_attr_t *a, void *(*fn)(void *), void *arg) {
	xo_win_start *s = malloc(sizeof *s);
	if (!s) {
		return ENOMEM;
	}
	s->fn = fn;
	s->arg = arg;
	size_t stack = a ? a->stack : 0;
	// the size reserves address space; Windows commits a thread's stack
	// as it grows (guard page)
	uintptr_t h = _beginthreadex(NULL, (unsigned)(stack > 0xFFFFFFFFu ? 0xFFFFFFFFu : stack), xo_thread_tramp, s,
		stack ? STACK_SIZE_PARAM_IS_A_RESERVATION : 0, NULL);
	if (h == 0) {
		free(s);
		return EAGAIN;
	}
	if (a && a->detached) {
		CloseHandle((HANDLE)h);
		*t = NULL;
	} else {
		*t = (HANDLE)h;
	}
	return 0;
}

static inline int pthread_join(pthread_t t, void **ret) {
	if (ret) {
		*ret = NULL;
	}
	WaitForSingleObject(t, INFINITE);
	CloseHandle(t);
	return 0;
}

// ---------------------------------------------------------------------------
// Winsock's constants

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
