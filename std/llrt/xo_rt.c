// xo_rt.c: the C left of the native runtime (the runtime is Xo:
// std/native/llrt, decisions 0206, 0233 and 0234; main is cabi_c.xo's).
// What stays is C by design: Windows' stack overflow handler (it runs on
// what is left of the overflowed stack) and the services record that
// hands it to stop.xo, the Windows headers with the asserts of the
// constants and layouts sock.xo, cabi_net.xo and poll.xo hard code, and
// the C tests' constructor in place of main's setup. It is
// compiled with the generated code (clang, -flto on LLVM); its
// interface is scalars and pointers only.

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

// ---------------------------------------------------------------------------
// The C tests of internal/llvm (XO_RT_NO_MAIN) bring their own main: a
// constructor does main's setup there (cabi_c.xo main's entry.xo init,
// through cabi.xo rt_init; xo's own ELF linker takes no .init_array,
// so the program's main does it otherwise:
// gotchas/runtime-c-no-constructors.md).
#ifdef XO_RT_NO_MAIN
void xo_x_rt_init(void);
void xo_x_proc_start(int64_t argc, int64_t argv, int64_t envp);
#ifndef _WIN32
extern char **environ;
#endif
__attribute__((constructor)) static void obj_heap_words(void) {
	xo_x_rt_init();
	// the environment for the runtime's settings (poolsetup.xo reads
	// XO_THREADS)
	xo_x_proc_start(0, 0, (int64_t)(intptr_t)environ);
}
#endif

// ---------------------------------------------------------------------------
// Output is stdio.xo's; the stack overflow handler flushes it.
void xo_x_flush_try(void);

// an address as the int64_t the C ABI exports (cabi.xo) take
#define XO_ADDR(p) ((int64_t)(intptr_t)(p))

#ifdef _WIN32
// the AFD polls' NtDeviceIoControlFile and NtCreateFile (poll.xo)
#pragma comment(lib, "ntdll")
// poll.xo reads the completions GetQueuedCompletionStatusEx returns
_Static_assert(sizeof(OVERLAPPED_ENTRY) == 32 && offsetof(OVERLAPPED_ENTRY, lpOverlapped) == 8, "poll.xo's OVERLAPPED_ENTRY");
#endif

// ---------------------------------------------------------------------------
// Two phase shutdown (core 7.5) is stop.xo's; stop_svc holds the one C
// word entry.xo reads (E_OVERFLOW: Windows' on_overflow, below), which
// entry.xo init hands to stop.xo (sys_c.xo xo_c_stop_svc, decision 0234).

#ifdef _WIN32
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

static int64_t stop_svc[20];
int64_t xo_c_stop_svc(void) {
#ifdef _WIN32
	stop_svc[14] = XO_ADDR(&on_overflow);
#endif
	return XO_ADDR(stop_svc);
}

// The C library functions of a Linux program linked with no C library
// (XO_NOLIBC=1 builds, decisions 0203 and 0206).
#ifdef XO_NOLIBC
#include "xo_nolibc.c"
#endif
