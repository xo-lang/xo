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

static inline int sched_yield(void) {
	SwitchToThread();
	return 0;
}

// ---------------------------------------------------------------------------
// System facts and clocks

#define _SC_PAGESIZE 1
#define _SC_NPROCESSORS_ONLN 2

static long xo_sysconf(int name) {
	SYSTEM_INFO si;
	GetSystemInfo(&si);
	switch (name) {
	case _SC_PAGESIZE:
		return (long)si.dwPageSize;
	case _SC_NPROCESSORS_ONLN:
		return (long)GetActiveProcessorCount(ALL_PROCESSOR_GROUPS);
	}
	return -1;
}
#define sysconf(n) xo_sysconf(n)

#define CLOCK_REALTIME 0
#define CLOCK_MONOTONIC 1

static int xo_clock_gettime(int clk, struct timespec *ts) {
	if (clk == CLOCK_MONOTONIC) {
		static LARGE_INTEGER freq;
		LARGE_INTEGER c;
		if (freq.QuadPart == 0) {
			QueryPerformanceFrequency(&freq);
		}
		QueryPerformanceCounter(&c);
		int64_t s = c.QuadPart / freq.QuadPart;
		int64_t r = c.QuadPart % freq.QuadPart;
		ts->tv_sec = (time_t)s;
		ts->tv_nsec = (long)(r * 1000000000 / freq.QuadPart);
		return 0;
	}
	FILETIME ft;
	GetSystemTimePreciseAsFileTime(&ft);
	// 100 ns intervals since 1601-01-01
	int64_t t = (int64_t)(((uint64_t)ft.dwHighDateTime << 32) | ft.dwLowDateTime) - 116444736000000000LL;
	ts->tv_sec = (time_t)(t / 10000000);
	ts->tv_nsec = (long)((t % 10000000) * 100);
	return 0;
}
#define clock_gettime(c, t) xo_clock_gettime((c), (t))

// nanosleep sleeps at least ts (millisecond resolution)
static inline int nanosleep(const struct timespec *ts, struct timespec *rem) {
	(void)rem;
	int64_t ms = (int64_t)ts->tv_sec * 1000 + (ts->tv_nsec + 999999) / 1000000;
	Sleep((DWORD)(ms > 0x7FFFFFFF ? 0x7FFFFFFF : ms));
	return 0;
}

static inline struct tm *gmtime_r(const time_t *t, struct tm *out) { return gmtime_s(out, t) == 0 ? out : NULL; }
static inline struct tm *localtime_r(const time_t *t, struct tm *out) { return localtime_s(out, t) == 0 ? out : NULL; }

// ---------------------------------------------------------------------------
// Descriptors and sockets

#ifndef SHUT_RDWR
#define SHUT_RD SD_RECEIVE
#define SHUT_WR SD_SEND
#define SHUT_RDWR SD_BOTH
#endif

#define F_GETFD 1
#define F_SETFD 2
#define F_GETFL 3
#define F_SETFL 4
#define FD_CLOEXEC 1
#ifndef O_NONBLOCK
#define O_NONBLOCK 0x4000
#endif
#ifndef O_CLOEXEC
#define O_CLOEXEC 0
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
static int xo_w_getpeername(int fd, struct sockaddr *a, int *n) { XO_WSA_INT(getpeername((SOCKET)fd, a, n)); }
static int xo_w_shutdown(int fd, int how) { XO_WSA_INT(shutdown((SOCKET)fd, how)); }
#define bind(fd, a, n) xo_w_bind((fd), (a), (int)(n))
#define listen(fd, n) xo_w_listen((fd), (n))
#define getsockname(fd, a, n) xo_w_getsockname((fd), (a), (n))
#define getpeername(fd, a, n) xo_w_getpeername((fd), (a), (n))
#define shutdown(fd, how) xo_w_shutdown((fd), (how))

// Descriptors and sockets are both ints (stage 5): a socket is its
// SOCKET handle's value (a multiple of 4), and a C runtime descriptor above
// the standard three carries XO_W_FD, so the two never share a number. The
// runtime's descriptors (0 to 2 untagged) only reach the C runtime through
// these shims, which take the tag off. Before, both were plain small
// numbers, and with about 550 files open a file's descriptor equaled a
// socket's handle: closing the file closed the socket.
#define XO_W_FD 0x40000000

// xo_w_fd tags a C runtime descriptor (negative: an error, as it is).
static inline int xo_w_fd(int raw) { return raw > 2 ? raw | XO_W_FD : raw; }

// xo_w_raw is the C runtime's number of a descriptor.
static inline int xo_w_raw(int fd) { return fd > 2 && (fd & XO_W_FD) ? fd & ~XO_W_FD : fd; }

// xo_w_is_sock reports whether fd names a socket.
static inline int xo_w_is_sock(int fd) { return fd > 2 && !(fd & XO_W_FD); }

// sockets are numbered as ints, like descriptors (a SOCKET is a kernel
// handle, a multiple of 4 below 2^32 in practice)
static int xo_w_socket(int af, int type, int proto) {
	xo_wsa_init();
	// not inherited by child processes (CLOEXEC)
	SOCKET s = WSASocketW(af, type, proto, NULL, 0, WSA_FLAG_NO_HANDLE_INHERIT | WSA_FLAG_OVERLAPPED);
	if (s == INVALID_SOCKET) {
		errno = xo_wsa_errno();
		return -1;
	}
	return (int)s;
}
#define socket(a, t, p) xo_w_socket((a), (t), (p))

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

static int xo_w_fcntl(int fd, int cmd, ...) {
	va_list ap;
	va_start(ap, cmd);
	int v = cmd == F_SETFL || cmd == F_SETFD ? va_arg(ap, int) : 0;
	va_end(ap);
	if (cmd == F_SETFL) {
		u_long nb = (v & O_NONBLOCK) ? 1 : 0;
		if (ioctlsocket((SOCKET)fd, FIONBIO, &nb) != 0) {
			errno = xo_wsa_errno();
			return -1;
		}
	}
	return 0;
}
#define fcntl(...) xo_w_fcntl(__VA_ARGS__)

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

// Winsock's option values are char pointers
#define getsockopt(s, l, o, v, n) getsockopt((SOCKET)(s), (l), (o), (char *)(v), (n))
#define setsockopt(s, l, o, v, n) setsockopt((SOCKET)(s), (l), (o), (const char *)(v), (n))

#define strncasecmp(a, b, n) _strnicmp((a), (b), (n))
#define strcasecmp(a, b) _stricmp((a), (b))
#define strtok_r(s, d, p) strtok_s((s), (d), (p))
#define timegm(tm) _mkgmtime(tm)

static int xo_w_pipe(int p[2]) {
	if (_pipe(p, 1 << 16, _O_BINARY) != 0) {
		return -1;
	}
	p[0] = xo_w_fd(p[0]);
	p[1] = xo_w_fd(p[1]);
	return 0;
}
#define pipe(p) xo_w_pipe(p)

static int xo_w_fix(const char *path, int e, int isopen);

// open takes a UTF-8 path (converted to UTF-16) and opens in binary mode
static int xo_w_open(const char *path, int flags, ...) {
	int mode = 0;
	if (flags & _O_CREAT) {
		va_list ap;
		va_start(ap, flags);
		mode = va_arg(ap, int);
		va_end(ap);
	}
	int n = MultiByteToWideChar(CP_UTF8, 0, path, -1, NULL, 0);
	if (n <= 0) {
		errno = ENOENT;
		return -1;
	}
	wchar_t *w = malloc((size_t)n * sizeof(wchar_t));
	if (!w) {
		errno = ENOMEM;
		return -1;
	}
	MultiByteToWideChar(CP_UTF8, 0, path, -1, w, n);
	// the owner's write bit decides the read-only attribute of a new file
	int fd = _wopen(w, flags | _O_BINARY | _O_NOINHERIT, _S_IREAD | ((flags & _O_CREAT) && !(mode & 0200) ? 0 : _S_IWRITE));
	free(w);
	if (fd < 0) {
		errno = xo_w_fix(path, errno, 1);
	}
	return xo_w_fd(fd);
}
#define open(...) xo_w_open(__VA_ARGS__)

// fopen for reading ("rb", the runtime's only mode), through open
static FILE *xo_w_fopen_rb(const char *path) {
	int fd = xo_w_open(path, _O_RDONLY);
	if (fd < 0) {
		return NULL;
	}
	FILE *f = _fdopen(xo_w_raw(fd), "rb");
	if (!f) {
		_close(xo_w_raw(fd));
	}
	return f;
}
#define fopen(p, m) xo_w_fopen_rb(p)

// ---------------------------------------------------------------------------
// Files by path (xo_fs.c, xo_time.c): UTF-8 paths converted to UTF-16 for
// the wide Win32 calls, and the Win32 error as errno.

// xo_w_path converts a UTF-8 path to a malloc'd UTF-16 one (NULL: errno).
static wchar_t *xo_w_path(const char *path) {
	int n = MultiByteToWideChar(CP_UTF8, 0, path, -1, NULL, 0);
	if (n <= 0) {
		errno = ENOENT;
		return NULL;
	}
	wchar_t *w = malloc((size_t)n * sizeof(wchar_t));
	if (!w) {
		errno = ENOMEM;
		return NULL;
	}
	MultiByteToWideChar(CP_UTF8, 0, path, -1, w, n);
	return w;
}

// xo_w_utf8 converts UTF-16 to UTF-8 into buf (size n): the length, -1
// when it does not fit.
static int xo_w_utf8(const wchar_t *w, int wn, char *buf, size_t n) {
	int k = WideCharToMultiByte(CP_UTF8, 0, w, wn, buf, (int)n, NULL, NULL);
	return k > 0 || wn == 0 ? k : -1;
}

// xo_w_errno is errno for a Win32 error
static int xo_w_errno(DWORD e) {
	switch (e) {
	case ERROR_FILE_NOT_FOUND:
	case ERROR_PATH_NOT_FOUND:
	case ERROR_INVALID_NAME:
	case ERROR_BAD_NETPATH:
		return ENOENT;
	case ERROR_ACCESS_DENIED:
	case ERROR_SHARING_VIOLATION:
	case ERROR_LOCK_VIOLATION:
	case ERROR_PRIVILEGE_NOT_HELD:
		return EACCES;
	case ERROR_ALREADY_EXISTS:
	case ERROR_FILE_EXISTS:
		return EEXIST;
	case ERROR_DIR_NOT_EMPTY:
		return ENOTEMPTY;
	case ERROR_DIRECTORY:
		return ENOTDIR;
	case ERROR_NOT_ENOUGH_MEMORY:
	case ERROR_OUTOFMEMORY:
		return ENOMEM;
	case ERROR_NOT_A_REPARSE_POINT:
		return EINVAL;
	default:
		return EIO;
	}
}

// xo_w_attrs is the attributes of a UTF-8 path, INVALID_FILE_ATTRIBUTES
// when it does not exist.
static DWORD xo_w_attrs(const char *path) {
	wchar_t *w = xo_w_path(path);
	if (!w) {
		return INVALID_FILE_ATTRIBUTES;
	}
	DWORD a = GetFileAttributesW(w);
	free(w);
	return a;
}

// xo_w_fix refines the errno e of a call on path as POSIX reports it,
// which is what the Fs errors are defined by: ENOTDIR when a parent of
// path is a file (Windows: path not found), EISDIR when an open of a
// directory was refused (Windows: access denied).
static int xo_w_fix(const char *path, int e, int isopen) {
	if (e == EACCES && isopen) {
		DWORD a = xo_w_attrs(path);
		if (a != INVALID_FILE_ATTRIBUTES && (a & FILE_ATTRIBUTE_DIRECTORY)) {
			return EISDIR;
		}
		return e;
	}
	if (e != ENOENT) {
		return e;
	}
	size_t n = strlen(path);
	char *p = malloc(n + 1);
	if (!p) {
		return e;
	}
	memcpy(p, path, n + 1);
	for (size_t i = 1; i < n; i++) {
		if (p[i] != '/' && p[i] != '\\') {
			continue;
		}
		if (i == 2 && p[1] == ':') {
			continue; // the drive
		}
		char c = p[i];
		p[i] = 0;
		DWORD a = xo_w_attrs(p);
		p[i] = c;
		if (a == INVALID_FILE_ATTRIBUTES) {
			break;
		}
		if (!(a & FILE_ATTRIBUTE_DIRECTORY)) {
			e = ENOTDIR;
			break;
		}
	}
	free(p);
	return e;
}

#ifndef S_IFLNK
#define S_IFLNK 0xA000
#endif
#ifndef S_ISREG
#define S_ISREG(m) (((m) & _S_IFMT) == _S_IFREG)
#define S_ISDIR(m) (((m) & _S_IFMT) == _S_IFDIR)
#endif
#define S_ISLNK(m) (((m) & 0xF000) == S_IFLNK)

// struct xo_wstat replaces the C library's struct stat (the macro renames
// both the tag and the calls): the fields the runtime reads, with
// nanosecond times as on Linux.
struct xo_wstat {
	unsigned st_mode;
	int64_t st_size;
	struct timespec st_mtim;
};

static int64_t xo_w_filetime_ns(FILETIME ft) {
	int64_t t = (int64_t)(((uint64_t)ft.dwHighDateTime << 32) | ft.dwLowDateTime) - 116444736000000000LL;
	return t * 100;
}

static int xo_w_fill_stat(HANDLE h, struct xo_wstat *st, int link) {
	BY_HANDLE_FILE_INFORMATION fi;
	if (!GetFileInformationByHandle(h, &fi)) {
		errno = xo_w_errno(GetLastError());
		return -1;
	}
	unsigned perm = (fi.dwFileAttributes & FILE_ATTRIBUTE_READONLY) ? 0444 : 0666;
	if (fi.dwFileAttributes & FILE_ATTRIBUTE_DIRECTORY) {
		st->st_mode = _S_IFDIR | 0777;
	} else {
		st->st_mode = _S_IFREG | perm;
	}
	if (link && (fi.dwFileAttributes & FILE_ATTRIBUTE_REPARSE_POINT)) {
		st->st_mode = S_IFLNK | 0777;
	}
	st->st_size = (int64_t)(((uint64_t)fi.nFileSizeHigh << 32) | fi.nFileSizeLow);
	int64_t ns = xo_w_filetime_ns(fi.ftLastWriteTime);
	st->st_mtim.tv_sec = (time_t)(ns / 1000000000);
	st->st_mtim.tv_nsec = (long)(ns % 1000000000);
	return 0;
}

// xo_w_open_meta opens path for its attributes (a directory too); link
// opens a symbolic link itself.
static HANDLE xo_w_open_meta(const char *path, DWORD access, int link) {
	wchar_t *w = xo_w_path(path);
	if (!w) {
		return INVALID_HANDLE_VALUE;
	}
	HANDLE h = CreateFileW(w, access, FILE_SHARE_READ | FILE_SHARE_WRITE | FILE_SHARE_DELETE, NULL, OPEN_EXISTING,
		FILE_FLAG_BACKUP_SEMANTICS | (link ? FILE_FLAG_OPEN_REPARSE_POINT : 0), NULL);
	DWORD e = GetLastError();
	if (h == INVALID_HANDLE_VALUE && !link && e == ERROR_CANT_ACCESS_FILE) {
		// a reparse point the system cannot follow (an AF_UNIX socket
		// file): the point itself, as Go's os.Stat
		h = CreateFileW(w, access, FILE_SHARE_READ | FILE_SHARE_WRITE | FILE_SHARE_DELETE, NULL, OPEN_EXISTING,
			FILE_FLAG_BACKUP_SEMANTICS | FILE_FLAG_OPEN_REPARSE_POINT, NULL);
		e = GetLastError();
	}
	free(w);
	if (h == INVALID_HANDLE_VALUE) {
		errno = xo_w_fix(path, xo_w_errno(e), 0);
	}
	return h;
}

static int xo_w_stat_path(const char *path, struct xo_wstat *st, int link) {
	HANDLE h = xo_w_open_meta(path, FILE_READ_ATTRIBUTES, link);
	if (h == INVALID_HANDLE_VALUE) {
		return -1;
	}
	int r = xo_w_fill_stat(h, st, link);
	CloseHandle(h);
	return r;
}

static int xo_w_fstat(int fd, struct xo_wstat *st) {
	HANDLE h = (HANDLE)_get_osfhandle(xo_w_raw(fd));
	if (h == INVALID_HANDLE_VALUE) {
		errno = EBADF;
		return -1;
	}
	if (GetFileType(h) != FILE_TYPE_DISK) {
		// a console or a pipe: neither regular file nor directory
		memset(st, 0, sizeof *st);
		st->st_mode = _S_IFCHR;
		return 0;
	}
	return xo_w_fill_stat(h, st, 0);
}

#define stat xo_wstat
#define xo_wstat(p, st) xo_w_stat_path((p), (st), 0)
#define lstat(p, st) xo_w_stat_path((p), (st), 1)
#define fstat(fd, st) xo_w_fstat((fd), (st))

// directories: FindFirstFileW over path\*
struct dirent {
	char d_name[1024];
};
typedef struct {
	HANDLE h;
	WIN32_FIND_DATAW fd;
	int first;
	struct dirent e;
} DIR;

static DIR *opendir(const char *path) {
	size_t n = strlen(path);
	char *pat = malloc(n + 3);
	if (!pat) {
		errno = ENOMEM;
		return NULL;
	}
	memcpy(pat, path, n);
	memcpy(pat + n, "\\*", 3);
	wchar_t *w = xo_w_path(pat);
	free(pat);
	if (!w) {
		return NULL;
	}
	DIR *d = calloc(1, sizeof(DIR));
	if (!d) {
		free(w);
		errno = ENOMEM;
		return NULL;
	}
	d->h = FindFirstFileExW(w, FindExInfoBasic, &d->fd, FindExSearchNameMatch, NULL, 0);
	DWORD e = GetLastError();
	free(w);
	if (d->h == INVALID_HANDLE_VALUE) {
		free(d);
		// FindFirstFile on a file reports a missing path
		struct xo_wstat st;
		if (e == ERROR_DIRECTORY || (xo_w_stat_path(path, &st, 0) == 0 && !S_ISDIR(st.st_mode))) {
			errno = ENOTDIR;
		} else {
			errno = xo_w_fix(path, xo_w_errno(e), 0);
		}
		return NULL;
	}
	d->first = 1;
	return d;
}

static struct dirent *readdir(DIR *d) {
	if (!d->first && !FindNextFileW(d->h, &d->fd)) {
		return NULL;
	}
	d->first = 0;
	if (xo_w_utf8(d->fd.cFileName, -1, d->e.d_name, sizeof d->e.d_name) < 0) {
		d->e.d_name[0] = 0;
	}
	return &d->e;
}

static int closedir(DIR *d) {
	FindClose(d->h);
	free(d);
	return 0;
}

// xo_w_call1 runs a wide path call that reports a Win32 error by BOOL
#define XO_W_PATHCALL(name, expr)                                                                                          \
	static int name(const char *path) {                                                                                    \
		wchar_t *w = xo_w_path(path);                                                                                      \
		if (!w) {                                                                                                          \
			return -1;                                                                                                     \
		}                                                                                                                  \
		BOOL ok = (expr);                                                                                                  \
		DWORD e = GetLastError();                                                                                          \
		free(w);                                                                                                           \
		if (!ok) {                                                                                                         \
			errno = xo_w_fix(path, xo_w_errno(e), 0);                                                                      \
			return -1;                                                                                                     \
		}                                                                                                                  \
		return 0;                                                                                                          \
	}
XO_W_PATHCALL(xo_w_unlink, DeleteFileW(w))
XO_W_PATHCALL(xo_w_rmdir, RemoveDirectoryW(w))
XO_W_PATHCALL(xo_w_mkdir1, CreateDirectoryW(w, NULL))
#define unlink(p) xo_w_unlink(p)
#define rmdir(p) xo_w_rmdir(p)
#define mkdir(p, mode) xo_w_mkdir1(p)

static int xo_w_rename(const char *a, const char *b) {
	wchar_t *wa = xo_w_path(a), *wb = xo_w_path(b);
	if (!wa || !wb) {
		free(wa);
		free(wb);
		return -1;
	}
	BOOL ok = MoveFileExW(wa, wb, MOVEFILE_REPLACE_EXISTING);
	DWORD e = GetLastError();
	free(wa);
	free(wb);
	if (!ok) {
		errno = xo_w_fix(a, xo_w_errno(e), 0);
		return -1;
	}
	return 0;
}
#define rename(a, b) xo_w_rename((a), (b))

// chmod: Windows keeps only the read-only attribute (the owner's write
// bit), as Go's os.Chmod
static int xo_w_chmod(const char *path, int mode) {
	wchar_t *w = xo_w_path(path);
	if (!w) {
		return -1;
	}
	DWORD a = GetFileAttributesW(w);
	BOOL ok = a != INVALID_FILE_ATTRIBUTES;
	if (ok) {
		a = (mode & 0200) ? (a & ~(DWORD)FILE_ATTRIBUTE_READONLY) : (a | FILE_ATTRIBUTE_READONLY);
		ok = SetFileAttributesW(w, a);
	}
	DWORD e = GetLastError();
	free(w);
	if (!ok) {
		errno = xo_w_errno(e);
		return -1;
	}
	return 0;
}
#define chmod(p, m) xo_w_chmod((p), (int)(m))
typedef int mode_t;

#define AT_FDCWD -100

// utimensat sets the access and modification times (ts[0], ts[1])
static int utimensat(int dirfd, const char *path, const struct timespec ts[2], int flags) {
	(void)dirfd;
	(void)flags;
	HANDLE h = xo_w_open_meta(path, FILE_WRITE_ATTRIBUTES, 0);
	if (h == INVALID_HANDLE_VALUE) {
		return -1;
	}
	FILETIME ft[2];
	for (int i = 0; i < 2; i++) {
		int64_t t = ((int64_t)ts[i].tv_sec * 1000000000 + ts[i].tv_nsec) / 100 + 116444736000000000LL;
		ft[i].dwLowDateTime = (DWORD)t;
		ft[i].dwHighDateTime = (DWORD)((uint64_t)t >> 32);
	}
	BOOL ok = SetFileTime(h, NULL, &ft[0], &ft[1]);
	DWORD e = GetLastError();
	CloseHandle(h);
	if (!ok) {
		errno = xo_w_errno(e);
		return -1;
	}
	return 0;
}

static char *xo_w_getcwd(char *buf, size_t n) {
	wchar_t w[4096];
	DWORD k = GetCurrentDirectoryW(4096, w);
	int u = k == 0 || k >= 4096 ? -1 : xo_w_utf8(w, (int)k, buf, n - 1);
	if (u < 0) {
		errno = ERANGE;
		return NULL;
	}
	buf[u] = 0;
	return buf;
}
#define getcwd(b, n) xo_w_getcwd((b), (n))

// symbolic links: created without the privilege where developer mode
// allows it; a directory target makes a directory link
static int symlink(const char *target, const char *path) {
	wchar_t *wt = xo_w_path(target), *wp = xo_w_path(path);
	if (!wt || !wp) {
		free(wt);
		free(wp);
		return -1;
	}
	// a relative target is relative to the link's directory
	DWORD flags = SYMBOLIC_LINK_FLAG_ALLOW_UNPRIVILEGED_CREATE;
	DWORD a = GetFileAttributesW(wt);
	if (a != INVALID_FILE_ATTRIBUTES && (a & FILE_ATTRIBUTE_DIRECTORY)) {
		flags |= SYMBOLIC_LINK_FLAG_DIRECTORY;
	}
	for (wchar_t *c = wt; *c; c++) {
		if (*c == L'/') {
			*c = L'\\';
		}
	}
	BOOL ok = CreateSymbolicLinkW(wp, wt, flags);
	DWORD e = GetLastError();
	free(wt);
	free(wp);
	if (!ok) {
		errno = xo_w_errno(e);
		return -1;
	}
	return 0;
}

// the reparse data of a symbolic link (ntifs.h's REPARSE_DATA_BUFFER)
typedef struct {
	ULONG tag;
	USHORT len, reserved;
	USHORT sub_off, sub_len, print_off, print_len;
	ULONG flags; // symbolic links only
	WCHAR buf[1];
} xo_w_reparse;

// readlink reads a symbolic link's target (its print name, with / for \,
// as Fs.read_link gives it, std.md 3.1); no link names /etc/localtime,
// so the local zone is "Local" (xo_time.c)
static ssize_t readlink(const char *path, char *buf, size_t n) {
	HANDLE h = xo_w_open_meta(path, 0, 1);
	if (h == INVALID_HANDLE_VALUE) {
		return -1;
	}
	union {
		xo_w_reparse r;
		char raw[MAXIMUM_REPARSE_DATA_BUFFER_SIZE];
	} u;
	DWORD got = 0;
	BOOL ok = DeviceIoControl(h, FSCTL_GET_REPARSE_POINT, NULL, 0, &u, sizeof u, &got, NULL);
	DWORD e = GetLastError();
	CloseHandle(h);
	if (!ok) {
		errno = xo_w_errno(e);
		return -1;
	}
	if (u.r.tag != IO_REPARSE_TAG_SYMLINK) {
		errno = EINVAL;
		return -1;
	}
	WCHAR *name = (WCHAR *)((char *)u.r.buf + u.r.print_off);
	int k = xo_w_utf8(name, u.r.print_len / 2, buf, n);
	if (k < 0) {
		errno = ENAMETOOLONG;
		return -1;
	}
	for (int i = 0; i < k; i++) {
		if (buf[i] == '\\') {
			buf[i] = '/';
		}
	}
	return k;
}

// advisory locks on a descriptor's whole file
#define LOCK_SH 1
#define LOCK_EX 2
#define LOCK_NB 4
#define LOCK_UN 8
static int flock(int fd, int op) {
	HANDLE h = (HANDLE)_get_osfhandle(xo_w_raw(fd));
	OVERLAPPED o;
	memset(&o, 0, sizeof o);
	BOOL ok;
	if (op & LOCK_UN) {
		ok = UnlockFileEx(h, 0, MAXDWORD, MAXDWORD, &o);
	} else {
		DWORD f = ((op & LOCK_EX) ? LOCKFILE_EXCLUSIVE_LOCK : 0) | ((op & LOCK_NB) ? LOCKFILE_FAIL_IMMEDIATELY : 0);
		ok = LockFileEx(h, f, 0, MAXDWORD, MAXDWORD, &o);
	}
	if (!ok) {
		DWORD e = GetLastError();
		errno = e == ERROR_LOCK_VIOLATION || e == ERROR_IO_PENDING ? EWOULDBLOCK : xo_w_errno(e);
		return -1;
	}
	return 0;
}

#define read(fd, p, n) _read(xo_w_raw(fd), (p), (unsigned)(n))
#define write(fd, p, n) _write(xo_w_raw(fd), (p), (unsigned)(n))

// ---------------------------------------------------------------------------
// Pumps: a handle the poller cannot wait on (a pipe, the console) joined
// to a loopback socket it can, by a thread that copies between them

typedef struct {
	HANDLE h;
	SOCKET s;
	int out, close_h;
} xo_w_pump_t;

static void *xo_w_pump_main(void *a) {
	xo_w_pump_t *p = a;
	char b[65536];
	if (p->out) {
		DWORD n;
		for (;;) {
			if (!ReadFile(p->h, b, sizeof b, &n, NULL)) {
				break; // ERROR_BROKEN_PIPE: every writer closed it
			}
			if (n == 0 && GetFileType(p->h) == FILE_TYPE_CHAR) {
				break; // the console's end of input (Ctrl-Z)
			}
			int64_t at = 0;
			while (at < (int64_t)n) {
				int w = send(p->s, b + at, (int)(n - at), 0);
				if (w <= 0) {
					goto done; // the program closed its end
				}
				at += w;
			}
		}
	} else {
		for (;;) {
			int r = recv(p->s, b, sizeof b, 0);
			if (r <= 0) {
				break; // the program closed its end
			}
			DWORD at = 0, w;
			while (at < (DWORD)r) {
				if (!WriteFile(p->h, b + at, (DWORD)r - at, &w, NULL)) {
					goto done; // the reader closed it
				}
				at += w;
			}
		}
	}
done:
	if (p->close_h) {
		CloseHandle(p->h);
	}
	closesocket(p->s);
	free(p);
	return NULL;
}

// xo_w_pump joins h to a loopback socket pair and returns the program's
// end (nonblocking), -1 when it cannot: out copies what h reads to the
// socket, else what the socket receives to h. At the end (h's or the
// program's) the thread closes its socket, and h when close_h.
static int xo_w_pump(HANDLE h, int out, int close_h) {
	int sv[2];
	if (xo_w_socketpair(sv) < 0) {
		return -1;
	}
	u_long zero = 0;
	ioctlsocket((SOCKET)sv[1], FIONBIO, &zero); // the thread's end blocks
	xo_w_pump_t *p = malloc(sizeof *p);
	if (!p) {
		xo_w_close(sv[0]);
		xo_w_close(sv[1]);
		return -1;
	}
	*p = (xo_w_pump_t){h, (SOCKET)sv[1], out, close_h};
	pthread_t t;
	pthread_attr_t at;
	pthread_attr_init(&at);
	pthread_attr_setstacksize(&at, 256 << 10);
	pthread_attr_setdetachstate(&at, PTHREAD_CREATE_DETACHED);
	int e = pthread_create(&t, &at, xo_w_pump_main, p);
	pthread_attr_destroy(&at);
	if (e != 0) {
		xo_w_close(sv[0]);
		xo_w_close(sv[1]);
		free(p);
		return -1;
	}
	return sv[0];
}

// ---------------------------------------------------------------------------
// The environment in UTF-8 (stage 5): the C runtime's _environ and getenv
// are in the ANSI code page, so a value outside it came out as '?'. The
// runtime reads a UTF-8 copy of GetEnvironmentStringsW, taken once (Xo
// programs do not set variables), and getenv looks names up in it
// without case, as Windows does.

static char **xo_w_env;
static INIT_ONCE xo_w_env_o = INIT_ONCE_STATIC_INIT;

static BOOL CALLBACK xo_w_env_init(PINIT_ONCE o, PVOID p, PVOID *c) {
	(void)o;
	(void)p;
	(void)c;
	WCHAR *block = GetEnvironmentStringsW();
	size_t n = 0;
	for (WCHAR *w = block; w && *w; w += wcslen(w) + 1) {
		n++;
	}
	char **env = calloc(n + 1, sizeof(char *));
	if (!env) {
		abort();
	}
	size_t k = 0;
	for (WCHAR *w = block; w && *w; w += wcslen(w) + 1) {
		if (w[0] == L'=') {
			continue; // the per drive directories (=C:=C:\...)
		}
		int u = WideCharToMultiByte(CP_UTF8, 0, w, -1, NULL, 0, NULL, NULL);
		char *e = u > 0 ? malloc((size_t)u) : NULL;
		if (!e) {
			continue;
		}
		WideCharToMultiByte(CP_UTF8, 0, w, -1, e, u, NULL, NULL);
		env[k++] = e;
	}
	env[k] = NULL;
	if (block) {
		FreeEnvironmentStringsW(block);
	}
	xo_w_env = env;
	return TRUE;
}

// xo_w_environ is the environment as UTF-8 NAME=VALUE entries.
static char **xo_w_environ(void) {
	InitOnceExecuteOnce(&xo_w_env_o, xo_w_env_init, NULL, NULL);
	return xo_w_env;
}

// xo_w_getenv is getenv over xo_w_environ (names without case).
static const char *xo_w_getenv(const char *name) {
	size_t n = strlen(name);
	for (char **e = xo_w_environ(); *e; e++) {
		if (_strnicmp(*e, name, n) == 0 && (*e)[n] == '=') {
			return *e + n + 1;
		}
	}
	return NULL;
}
#define getenv(n) xo_w_getenv(n)

// ---------------------------------------------------------------------------
// Random bytes

static int xo_w_entropy(void *p, size_t n) {
	return BCryptGenRandom(NULL, (PUCHAR)p, (ULONG)n, BCRYPT_USE_SYSTEM_PREFERRED_RNG) == 0 ? 0 : -1;
}

#endif
