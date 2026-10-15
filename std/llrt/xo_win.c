// xo_win.c: the POSIX subset the runtime uses, for Windows (decision
// 0197). xo_rt.c includes it instead of the POSIX headers when _WIN32 is
// defined (clang --target=x86_64-pc-windows-msvc or aarch64-pc-windows-msvc,
// with the MSVC C runtime and the Windows SDK). It maps:
//
//   - threads: pthread_create/join on _beginthreadex, mutexes on SRW
//     locks, pthread_once on InitOnceExecuteOnce;
//   - the futex: WaitOnAddress and WakeByAddressSingle (xo_rt.c);
//   - clocks: QueryPerformanceCounter (monotonic) and
//     GetSystemTimePreciseAsFileTime (wall clock);
//   - sockets: Winsock 2 (initialized on first use); readiness comes
//     from AFD polls on the poller's completion port (xo_rt.c);
//   - descriptors: the C runtime's _read/_write/_close, in binary mode.
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
typedef SRWLOCK pthread_mutex_t;
#define PTHREAD_MUTEX_INITIALIZER SRWLOCK_INIT
typedef INIT_ONCE pthread_once_t;
#define PTHREAD_ONCE_INIT INIT_ONCE_STATIC_INIT
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

static inline int pthread_mutex_init(pthread_mutex_t *m, const void *a) {
	(void)a;
	InitializeSRWLock(m);
	return 0;
}
static inline int pthread_mutex_destroy(pthread_mutex_t *m) {
	(void)m;
	return 0;
}
static inline int pthread_mutex_lock(pthread_mutex_t *m) {
	AcquireSRWLockExclusive(m);
	return 0;
}
static inline int pthread_mutex_unlock(pthread_mutex_t *m) {
	ReleaseSRWLockExclusive(m);
	return 0;
}

static inline int pthread_mutex_trylock(pthread_mutex_t *m) { return TryAcquireSRWLockExclusive(m) ? 0 : EBUSY; }

// condition variables (the TLS library's threading, which then skips
// third_party/mbedtls/xo/win/pthread.h: the same subset)
#define XO_WIN_PTHREAD_H 1
typedef CONDITION_VARIABLE pthread_cond_t;
#define PTHREAD_COND_INITIALIZER CONDITION_VARIABLE_INIT
static inline int pthread_cond_init(pthread_cond_t *c, const void *a) {
	(void)a;
	InitializeConditionVariable(c);
	return 0;
}
static inline int pthread_cond_destroy(pthread_cond_t *c) {
	(void)c;
	return 0;
}
static inline int pthread_cond_signal(pthread_cond_t *c) {
	WakeConditionVariable(c);
	return 0;
}
static inline int pthread_cond_broadcast(pthread_cond_t *c) {
	WakeAllConditionVariable(c);
	return 0;
}
static inline int pthread_cond_wait(pthread_cond_t *c, pthread_mutex_t *m) { return SleepConditionVariableSRW(c, m, INFINITE, 0) ? 0 : EINVAL; }

static BOOL CALLBACK xo_once_tramp(PINIT_ONCE o, PVOID fn, PVOID *ctx) {
	(void)o;
	(void)ctx;
	((void (*)(void))fn)();
	return TRUE;
}
static inline int pthread_once(pthread_once_t *o, void (*fn)(void)) {
	InitOnceExecuteOnce(o, xo_once_tramp, (PVOID)fn, NULL);
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
// Clocks

static inline struct tm *localtime_r(const time_t *t, struct tm *out) { return localtime_s(out, t) == 0 ? out : NULL; }

// ---------------------------------------------------------------------------
// Descriptors and sockets

#ifndef SHUT_RDWR
#define SHUT_RD SD_RECEIVE
#define SHUT_WR SD_SEND
#define SHUT_RDWR SD_BOTH
#endif


// xo_wsa_init starts Winsock once.
static void xo_wsa_once(void) {
	WSADATA d;
	WSAStartup(MAKEWORD(2, 2), &d);
}
static INIT_ONCE xo_wsa_o = INIT_ONCE_STATIC_INIT;
static BOOL CALLBACK xo_wsa_tramp(PINIT_ONCE o, PVOID p, PVOID *c) {
	(void)o;
	(void)p;
	(void)c;
	xo_wsa_once();
	return TRUE;
}
static inline void xo_wsa_init(void) { InitOnceExecuteOnce(&xo_wsa_o, xo_wsa_tramp, NULL, NULL); }

// errno for a Winsock error code
static int xo_wsa_map(int code) {
	switch (code) {
	case WSAEWOULDBLOCK:
		return EWOULDBLOCK;
	case WSAEINPROGRESS:
	case WSAEALREADY:
		return EINPROGRESS;
	case WSAECONNREFUSED:
		return ECONNREFUSED;
	case WSAECONNRESET:
		return ECONNRESET;
	case WSAECONNABORTED:
		return ECONNABORTED;
	case WSAESHUTDOWN:
		return EPIPE;
	case WSAETIMEDOUT:
		return ETIMEDOUT;
	case WSAEADDRINUSE:
		return EADDRINUSE;
	case WSAEADDRNOTAVAIL:
		return EADDRNOTAVAIL;
	case WSAEACCES:
		return EACCES;
	case WSAEINVAL:
		return EINVAL;
	case WSAEINTR:
		return EINTR;
	case WSAENOTSOCK:
		return ENOTSOCK;
	case WSAENETUNREACH:
		return ENETUNREACH;
	case WSAEHOSTUNREACH:
		return EHOSTUNREACH;
	case WSAENOTCONN:
		return ENOTCONN;
	case WSAEAFNOSUPPORT:
		return EAFNOSUPPORT;
	default:
		return EIO;
	}
}

// std/native/llrt/sock.xo's wsa_errno returns these numbers (the Windows
// C runtime's errno.h) for Winsock's codes, as xo_wsa_map does, and
// sock.xo and cabi_net.xo take Winsock's constants and layouts below
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

// errno for the last Winsock error
static int xo_wsa_errno(void) { return xo_wsa_map(WSAGetLastError()); }

// the Winsock calls the runtime makes on int descriptors, failing with
// errno set
#define XO_WSA_INT(call)                                                                                                   \
	do {                                                                                                                   \
		int r_ = (call);                                                                                                   \
		if (r_ == SOCKET_ERROR) {                                                                                          \
			errno = xo_wsa_errno();                                                                                        \
			return -1;                                                                                                     \
		}                                                                                                                  \
		return r_;                                                                                                         \
	} while (0)
static int xo_w_bind(int fd, const struct sockaddr *a, int n) { XO_WSA_INT(bind((SOCKET)fd, a, n)); }
static int xo_w_listen(int fd, int n) { XO_WSA_INT(listen((SOCKET)fd, n)); }
static int xo_w_getsockname(int fd, struct sockaddr *a, int *n) { XO_WSA_INT(getsockname((SOCKET)fd, a, n)); }
#define bind(fd, a, n) xo_w_bind((fd), (a), (int)(n))
#define listen(fd, n) xo_w_listen((fd), (n))
#define getsockname(fd, a, n) xo_w_getsockname((fd), (a), (n))

// Descriptors and sockets are both ints (stage 5): a socket is its
// SOCKET handle's value (a multiple of 4), and a C runtime descriptor above
// the standard three carries XO_W_FD, so the two never share a number. The
// runtime's descriptors (0 to 2 untagged) only reach the C runtime through
// these shims, which take the tag off. Before, both were plain small
// numbers, and with about 550 files open a file's descriptor equaled a
// socket's handle: closing the file closed the socket.
#define XO_W_FD 0x40000000

// xo_w_raw is the C runtime's number of a descriptor.
static inline int xo_w_raw(int fd) { return fd > 2 && (fd & XO_W_FD) ? fd & ~XO_W_FD : fd; }

// xo_w_is_sock reports whether fd names a socket.
static inline int xo_w_is_sock(int fd) { return fd > 2 && !(fd & XO_W_FD); }

// xo_w_socketpair connects two nonblocking loopback TCP sockets (the
// poller's self pipe: WSAPoll waits on sockets only)
static int xo_w_socketpair(int sv[2]) {
	xo_wsa_init();
	SOCKET l = WSASocketW(AF_INET, SOCK_STREAM, IPPROTO_TCP, NULL, 0, WSA_FLAG_NO_HANDLE_INHERIT);
	if (l == INVALID_SOCKET) {
		return -1;
	}
	struct sockaddr_in a;
	memset(&a, 0, sizeof a);
	a.sin_family = AF_INET;
	a.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
	int n = sizeof a;
	SOCKET c = INVALID_SOCKET, s = INVALID_SOCKET;
	if (bind(l, (struct sockaddr *)&a, n) == 0 && listen(l, 1) == 0 && getsockname(l, (struct sockaddr *)&a, &n) == 0) {
		c = WSASocketW(AF_INET, SOCK_STREAM, IPPROTO_TCP, NULL, 0, WSA_FLAG_NO_HANDLE_INHERIT);
		if (c != INVALID_SOCKET && connect(c, (struct sockaddr *)&a, n) == 0) {
			s = accept(l, NULL, NULL);
		}
	}
	closesocket(l);
	if (s == INVALID_SOCKET) {
		if (c != INVALID_SOCKET) {
			closesocket(c);
		}
		return -1;
	}
	u_long one = 1;
	ioctlsocket(s, FIONBIO, &one);
	ioctlsocket(c, FIONBIO, &one);
	sv[0] = (int)s;
	sv[1] = (int)c;
	return 0;
}

// close closes a socket or a C runtime descriptor
static int xo_w_close(int fd) {
	if (fd < 0) {
		errno = EBADF;
		return -1;
	}
	if (xo_w_is_sock(fd)) {
		if (closesocket((SOCKET)(unsigned)fd) != 0) {
			errno = xo_wsa_errno();
			return -1;
		}
		return 0;
	}
	return _close(xo_w_raw(fd));
}
#define close(fd) xo_w_close(fd)

#define read(fd, p, n) _read(xo_w_raw(fd), (p), (unsigned)(n))
#define write(fd, p, n) _write(xo_w_raw(fd), (p), (unsigned)(n))

#endif
