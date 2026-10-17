// win_consts.c: the Windows constants and layouts the native runtime's
// Xo hard codes (std/native/llrt sock.xo, cabi_net.xo, poll.xo, prof.xo), checked
// against the Windows SDK's headers. Compiled, never run, for
// windows/amd64 and windows/arm64 by internal/llvm TestWinConsts with the
// Windows cross toolchain (knowledgebase/infrastructure/toolchain.md
// "Windows cross builds on the Mac"); was xo_rt.c's (decision 0234).
#ifndef WIN32_LEAN_AND_MEAN
#define WIN32_LEAN_AND_MEAN
#endif
#include <winsock2.h>
#include <ws2tcpip.h>
#include <windows.h>
#include <tlhelp32.h>
#include <errno.h>
#include <stddef.h>

// sock.xo wsa_errno and cabi_net.xo
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

// poll.xo reads the completions GetQueuedCompletionStatusEx returns
_Static_assert(sizeof(OVERLAPPED_ENTRY) == 32 && offsetof(OVERLAPPED_ENTRY, lpOverlapped) == 8, "poll.xo's OVERLAPPED_ENTRY");

// prof.xo's sampler thread (decision 0238): the snapshot's entries, the
// thread access, the timer, the image's headers, and (x86_64) CONTEXT
_Static_assert(sizeof(THREADENTRY32) == 28 && offsetof(THREADENTRY32, th32ThreadID) == 8 &&
                   offsetof(THREADENTRY32, th32OwnerProcessID) == 12 && TH32CS_SNAPTHREAD == 4,
               "prof.xo's THREADENTRY32");
_Static_assert((THREAD_SUSPEND_RESUME | THREAD_GET_CONTEXT | THREAD_QUERY_INFORMATION) == 74 && TIMER_ALL_ACCESS == 2031619 &&
                   CREATE_WAITABLE_TIMER_HIGH_RESOLUTION == 2 && MEM_COMMIT + MEM_RESERVE == 12288,
               "prof.xo's access rights and flags");
_Static_assert(offsetof(IMAGE_DOS_HEADER, e_lfanew) == 60 && offsetof(IMAGE_NT_HEADERS64, OptionalHeader) == 24 &&
                   offsetof(IMAGE_FILE_HEADER, NumberOfSections) == 2 && offsetof(IMAGE_FILE_HEADER, PointerToSymbolTable) == 8 &&
                   offsetof(IMAGE_FILE_HEADER, NumberOfSymbols) == 12 && offsetof(IMAGE_FILE_HEADER, SizeOfOptionalHeader) == 16 &&
                   offsetof(IMAGE_OPTIONAL_HEADER64, DataDirectory) + 8 * IMAGE_DIRECTORY_ENTRY_EXCEPTION == 136 &&
                   sizeof(IMAGE_SECTION_HEADER) == 40 && offsetof(IMAGE_SECTION_HEADER, VirtualAddress) == 12 &&
                   offsetof(IMAGE_SECTION_HEADER, Misc.VirtualSize) == 8 && offsetof(IMAGE_SECTION_HEADER, Characteristics) == 36 &&
                   IMAGE_SCN_MEM_EXECUTE == 0x20000000 && IMAGE_SCN_CNT_CODE == 0x20 && IMAGE_SIZEOF_SYMBOL == 18 &&
                   IMAGE_SYM_CLASS_EXTERNAL == 2 && IMAGE_SYM_CLASS_STATIC == 3,
               "prof.xo's PE and COFF headers");
#if defined(__x86_64__)
_Static_assert(sizeof(CONTEXT) == 1232 && offsetof(CONTEXT, ContextFlags) == 48 && offsetof(CONTEXT, Rsp) == 152 &&
                   offsetof(CONTEXT, Rip) == 248 && (CONTEXT_CONTROL | CONTEXT_INTEGER) == 1048579 &&
                   sizeof(RUNTIME_FUNCTION) == 12,
               "prof.xo's x86_64 CONTEXT and RUNTIME_FUNCTION");
#endif
