// ---------------------------------------------------------------------------
// xo_nolibc.c: the C library functions the C runtime calls, for Linux
// programs linked statically with no C library (decisions 0203 and 0206,
// stage 1). Included at the end of xo_rt.c when it is compiled with
// -DXO_NOLIBC (XO_NOLIBC=1 builds): the runtime keeps its C library calls
// and this file defines them over raw system calls, so the program needs
// no libc at run time and links with -static -nostdlib --gc-sections
// (only what a program reaches is kept; a program that reaches a function
// defined nowhere here fails to link, naming it).
//
// What is here and what is not (decision 0206):
//   - process entry (_start: arguments, environment, the auxiliary vector,
//     the thread pointer and the static TLS block, a 1 GiB stack for main
//     as the runtime's main thread has), exit;
//   - memory (malloc and friends over mmap), strings, integer parsing,
//     qsort, a printf family without floating point conversions;
//   - files, directories, pipes, epoll, getrandom;
//   - forwarded to Xo (stage 2, std/native/llrt/cabi_nolibc.xo, called
//     as xo_x_<name>): the clock (the vDSO's clock_gettime), signals
//     (rt_sigaction with the emitters' restorer), locks (futex based
//     mutexes, once), threads (clone with a stack and a TLS block per
//     thread, joined through CLONE_CHILD_CLEARTID), the TLS block's
//     layout;
//   - unwinding is Xo (decision 0221, std/native/llrt/unwind.xo): no
//     _Unwind_* function is defined or called;
//   - not here (stage 2): floating point printf conversions and strtod
//     (they end the program with a message); stage 3: sockets past
//     shutdown, DNS (getaddrinfo is not defined).
// The system call layer of Xo code is std/native/llrt/linux.xo; this
// file serves the C runtime only, and shrinks as the runtime moves to Xo.

// The Xo exports this file forwards to (std/native/llrt/cabi_nolibc.xo).
int64_t xo_x_clock_gettime(int64_t id, int64_t ts);
int64_t xo_x_mutex_lock(int64_t m);
int64_t xo_x_mutex_unlock(int64_t m);
int64_t xo_x_mutex_trylock(int64_t m);
int64_t xo_x_once_run(int64_t w, int64_t f);
int64_t xo_x_sigaction(int64_t sig, int64_t act, int64_t old);
int64_t xo_x_signal(int64_t sig, int64_t h);
int64_t xo_x_sigprocmask(int64_t how, int64_t set, int64_t old);
int64_t xo_x_tls_size(void);
int64_t xo_x_tls_init(int64_t area);
int64_t xo_x_thread_create(int64_t stack, int64_t f, int64_t arg);
int64_t xo_x_thread_join(int64_t t);

#ifndef XO_NOLIBC_IMPL
#define XO_NOLIBC_IMPL

#include <dirent.h>
#include <sys/stat.h>
#include <sys/random.h>
#include <sys/syscall.h>
#include <elf.h>

#ifndef AT_EMPTY_PATH
#define AT_EMPTY_PATH 0x1000
#endif
#ifndef O_DIRECTORY
#define O_DIRECTORY 00200000
#endif

#if defined(__clang__)
#define NL_NOBUILTIN __attribute__((no_builtin))
#else
#define NL_NOBUILTIN __attribute__((optimize("no-tree-loop-distribute-patterns")))
#endif
#define NL_USED __attribute__((used))

// Every C library function here is NL_USED, kept through LTO: the LLVM
// backend's link optimizes the runtime with the program, and its code
// generation may call a C library function (calloc, memcpy) after the
// optimizer dropped an unreferenced definition. The linker's
// --gc-sections still drops what nothing calls.

// ---------------------------------------------------------------------------
// System calls

static inline long nl_sys6(long n, long a, long b, long c, long d, long e, long f) {
#if defined(__aarch64__)
	register long x8 __asm__("x8") = n;
	register long x0 __asm__("x0") = a;
	register long x1 __asm__("x1") = b;
	register long x2 __asm__("x2") = c;
	register long x3 __asm__("x3") = d;
	register long x4 __asm__("x4") = e;
	register long x5 __asm__("x5") = f;
	__asm__ volatile("svc #0" : "+r"(x0) : "r"(x8), "r"(x1), "r"(x2), "r"(x3), "r"(x4), "r"(x5) : "memory");
	return x0;
#elif defined(__x86_64__)
	long ret;
	register long r10 __asm__("r10") = d;
	register long r8 __asm__("r8") = e;
	register long r9 __asm__("r9") = f;
	__asm__ volatile("syscall" : "=a"(ret) : "a"(n), "D"(a), "S"(b), "d"(c), "r"(r10), "r"(r8), "r"(r9) : "rcx", "r11", "memory");
	return ret;
#else
#error "XO_NOLIBC: arm64 or x86_64 Linux only"
#endif
}

#define nl_sys0(n) nl_sys6((n), 0, 0, 0, 0, 0, 0)
#define nl_sys1(n, a) nl_sys6((n), (long)(a), 0, 0, 0, 0, 0)
#define nl_sys2(n, a, b) nl_sys6((n), (long)(a), (long)(b), 0, 0, 0, 0)
#define nl_sys3(n, a, b, c) nl_sys6((n), (long)(a), (long)(b), (long)(c), 0, 0, 0)
#define nl_sys4(n, a, b, c, d) nl_sys6((n), (long)(a), (long)(b), (long)(c), (long)(d), 0, 0)

static __thread int nl_errno;

NL_USED int *__errno_location(void) { return &nl_errno; }

// nl_ret is r, or -1 with errno set when r is a negative errno.
static long nl_ret(long r) {
	if (r < 0 && r > -4096) {
		nl_errno = (int)-r;
		return -1;
	}
	return r;
}

// ---------------------------------------------------------------------------
// Process entry and exit

char **environ;
static long nl_pagesz = 4096;
static int nl_argc;
static char **nl_argv;

extern int main(int, char **);

NL_USED __attribute__((noreturn)) void _exit(int code) {
	for (;;) {
		nl_sys1(SYS_exit_group, code);
	}
}

NL_USED __attribute__((noreturn)) void exit(int code) { _exit(code); }

NL_USED __attribute__((noreturn)) void abort(void) {
	nl_sys4(SYS_kill, nl_sys0(SYS_getpid), SIGABRT, 0, 0);
	_exit(134);
}

// nl_die writes msg to standard error and ends the program (a call of a
// C library function this file does not provide).
NL_USED __attribute__((noreturn, cold)) static void nl_die(const char *msg) {
	const char *p = "xo: no C library (XO_NOLIBC, decision 0206): ";
	nl_sys3(SYS_write, 2, p, __builtin_strlen(p));
	nl_sys3(SYS_write, 2, msg, __builtin_strlen(msg));
	nl_sys3(SYS_write, 2, "\n", 1);
	_exit(70);
}

NL_USED __attribute__((noreturn)) static void nl_run(void) { exit(main(nl_argc, nl_argv)); }

// The auxiliary vector _start found (getauxval: the program headers for
// the TLS image, the vDSO, the page size).
static long *nl_auxv;

NL_USED unsigned long getauxval(unsigned long k) {
	for (long *a = nl_auxv; a && a[0] != AT_NULL; a += 2) {
		if ((unsigned long)a[0] == k) {
			return (unsigned long)a[1];
		}
	}
	return 0;
}

// nl_tls sets up the main thread's TLS block (laid out by Xo,
// xo_x_tls_init) and thread pointer.
static void nl_tls(void) {
	long area = nl_sys6(SYS_mmap, 0, (long)xo_x_tls_size(), PROT_READ | PROT_WRITE, MAP_PRIVATE | MAP_ANONYMOUS, -1, 0);
	if (area < 0 && area > -4096) {
		nl_die("mmap of the TLS block failed");
	}
	char *tp = (char *)xo_x_tls_init(area);
#if defined(__aarch64__)
	__asm__ volatile("msr tpidr_el0, %0" ::"r"(tp));
#else
	nl_sys2(SYS_arch_prctl, 0x1002 /* ARCH_SET_FS */, tp);
#endif
}

// nl_start runs from _start with the initial stack: argc, argv, envp, the
// auxiliary vector.
NL_USED __attribute__((noreturn)) void xo_nolibc_start(long *sp) {
	nl_argc = (int)sp[0];
	nl_argv = (char **)(sp + 1);
	environ = nl_argv + nl_argc + 1;
	char **e = environ;
	while (*e) {
		e++;
	}
	nl_auxv = (long *)(e + 1);
	if (getauxval(AT_PAGESZ)) {
		nl_pagesz = (long)getauxval(AT_PAGESZ);
	}
	nl_tls();
	// main on a 1 GiB stack (the runtime runs the program's main on a
	// thread with one, and this thread waits for it)
	long gb = 1L << 30;
	long st = nl_sys6(SYS_mmap, 0, gb, PROT_READ | PROT_WRITE, MAP_PRIVATE | MAP_ANONYMOUS | MAP_NORESERVE, -1, 0);
	if (st < 0 && st > -4096) {
		nl_run();
	}
	char *top = (char *)st + gb;
#if defined(__aarch64__)
	__asm__ volatile("mov sp, %0\n\tmov x29, #0\n\tmov x30, #0\n\tbr %1" ::"r"(top), "r"(nl_run) : "memory");
#else
	__asm__ volatile("mov %0, %%rsp\n\txor %%ebp, %%ebp\n\tpush $0\n\tjmp *%1" ::"r"(top), "r"(nl_run) : "memory");
#endif
	__builtin_unreachable();
}

#if defined(__aarch64__)
__asm__(".text\n.global _start\n.type _start, %function\n_start:\n"
	"\tmov x29, #0\n\tmov x30, #0\n\tmov x0, sp\n\tand sp, x0, #-16\n\tbl xo_nolibc_start\n\tbrk #1\n"
	".size _start, .-_start\n");
#else
__asm__(".text\n.global _start\n.type _start, @function\n_start:\n"
	"\txor %ebp, %ebp\n\tmov %rsp, %rdi\n\tand $-16, %rsp\n\tcall xo_nolibc_start\n\thlt\n"
	".size _start, .-_start\n");
#endif

// ---------------------------------------------------------------------------
// Memory: malloc over mmap. Blocks up to 64 KiB come from free lists by
// power of two size class (carved from 1 MiB regions), larger ones are
// mapped one by one; a 16 byte header keeps the class or the mapped size.

#define NL_MIN_SHIFT 4
#define NL_MAX_SHIFT 16
#define NL_REGION (1L << 20)

typedef struct nl_free {
	struct nl_free *next;
} nl_free;

static nl_free *nl_lists[NL_MAX_SHIFT + 1];
static char *nl_cur, *nl_end;
static long nl_mu;

static void nl_lock(long *f) { xo_x_mutex_lock((int64_t)f); }
static void nl_unlock(long *f) { xo_x_mutex_unlock((int64_t)f); }

static void *nl_map(long n) {
	long r = nl_sys6(SYS_mmap, 0, n, PROT_READ | PROT_WRITE, MAP_PRIVATE | MAP_ANONYMOUS, -1, 0);
	if (r < 0 && r > -4096) {
		return NULL;
	}
	return (void *)r;
}

// Freed mappings are kept for reuse (up to NL_BIG of them, 64 MiB at
// most each): the runtime frees and allocates large blocks in loops (a
// file's contents), and a fresh mapping costs page faults every time.
#define NL_BIG 16
static char *nl_big[NL_BIG];
static long nl_big_n[NL_BIG];

// nl_big_take is a kept mapping of total bytes to 2 * total, or NULL.
static char *nl_big_take(long total) {
	char *p = NULL;
	nl_lock(&nl_mu);
	for (int i = 0; i < NL_BIG; i++) {
		if (nl_big[i] && nl_big_n[i] >= total && nl_big_n[i] <= 2 * total) {
			p = nl_big[i];
			nl_big[i] = NULL;
			((long *)p)[0] = -nl_big_n[i];
			break;
		}
	}
	nl_unlock(&nl_mu);
	return p;
}

// nl_big_keep keeps the mapping h of total bytes for reuse, or says no.
static int nl_big_keep(char *h, long total) {
	if (total > 64L << 20) {
		return 0;
	}
	int ok = 0;
	nl_lock(&nl_mu);
	for (int i = 0; i < NL_BIG; i++) {
		if (!nl_big[i]) {
			nl_big[i] = h;
			nl_big_n[i] = total;
			ok = 1;
			break;
		}
	}
	nl_unlock(&nl_mu);
	return ok;
}

NL_USED void *malloc(size_t n) {
	if (n == 0) {
		n = 1;
	}
	if (n + 16 > (size_t)1 << NL_MAX_SHIFT) {
		long total = (long)((n + 16 + (size_t)nl_pagesz - 1) & ~((size_t)nl_pagesz - 1));
		char *p = nl_big_take(total);
		if (p) {
			return p + 16;
		}
		p = nl_map(total);
		if (!p) {
			nl_errno = ENOMEM;
			return NULL;
		}
		((long *)p)[0] = -total;
		return p + 16;
	}
	int c = NL_MIN_SHIFT;
	while (((size_t)1 << c) < n + 16) {
		c++;
	}
	nl_lock(&nl_mu);
	nl_free *b = nl_lists[c];
	if (b) {
		nl_lists[c] = b->next;
	} else {
		long size = 1L << c;
		if (!nl_cur || nl_end - nl_cur < size) {
			nl_cur = nl_map(NL_REGION);
			if (!nl_cur) {
				nl_unlock(&nl_mu);
				nl_errno = ENOMEM;
				return NULL;
			}
			nl_end = nl_cur + NL_REGION;
		}
		b = (nl_free *)nl_cur;
		nl_cur += size;
	}
	nl_unlock(&nl_mu);
	((long *)b)[0] = c;
	return (char *)b + 16;
}

NL_USED void free(void *p) {
	if (!p) {
		return;
	}
	char *h = (char *)p - 16;
	long c = ((long *)h)[0];
	if (c < 0) {
		if (!nl_big_keep(h, -c)) {
			nl_sys2(SYS_munmap, h, -c);
		}
		return;
	}
	nl_lock(&nl_mu);
	((nl_free *)h)->next = nl_lists[c];
	nl_lists[c] = (nl_free *)h;
	nl_unlock(&nl_mu);
}

// no_builtin: the optimizer would fold malloc and a zeroing memset back
// into a call of calloc (itself).
NL_USED NL_NOBUILTIN void *calloc(size_t n, size_t m) {
	size_t t;
	if (__builtin_mul_overflow(n, m, &t)) {
		nl_errno = ENOMEM;
		return NULL;
	}
	void *p = malloc(t);
	if (p) {
		memset(p, 0, t);
	}
	return p;
}

NL_USED void *realloc(void *p, size_t n) {
	if (!p) {
		return malloc(n);
	}
	long c = ((long *)((char *)p - 16))[0];
	size_t have = c < 0 ? (size_t)(-c - 16) : ((size_t)1 << c) - 16;
	if (n <= have && c >= 0) {
		return p;
	}
	void *q = malloc(n);
	if (!q) {
		return NULL;
	}
	__builtin_memcpy(q, p, have < n ? have : n);
	free(p);
	return q;
}

// ---------------------------------------------------------------------------
// Strings and memory (no_builtin: the compiler must not turn these loops
// into calls of themselves)

NL_USED NL_NOBUILTIN void *memcpy(void *restrict d, const void *restrict s, size_t n) {
	unsigned char *o = d;
	const unsigned char *i = s;
	while (n >= 8) {
		uint64_t w;
		__builtin_memcpy(&w, i, 8);
		__builtin_memcpy(o, &w, 8);
		o += 8;
		i += 8;
		n -= 8;
	}
	while (n--) {
		*o++ = *i++;
	}
	return d;
}

NL_USED NL_NOBUILTIN void *memmove(void *d, const void *s, size_t n) {
	unsigned char *o = d;
	const unsigned char *i = s;
	if (o == i || n == 0) {
		return d;
	}
	// a word at a time, each word read before it is written
	if (o < i || o >= i + n) {
		while (n >= 8) {
			uint64_t w;
			__builtin_memcpy(&w, i, 8);
			__builtin_memcpy(o, &w, 8);
			o += 8;
			i += 8;
			n -= 8;
		}
		while (n--) {
			*o++ = *i++;
		}
		return d;
	}
	o += n;
	i += n;
	while (n >= 8) {
		uint64_t w;
		o -= 8;
		i -= 8;
		n -= 8;
		__builtin_memcpy(&w, i, 8);
		__builtin_memcpy(o, &w, 8);
	}
	while (n--) {
		*--o = *--i;
	}
	return d;
}

NL_USED NL_NOBUILTIN void *memset(void *d, int c, size_t n) {
	unsigned char *o = d;
	uint64_t w = (unsigned char)c * 0x0101010101010101ULL;
	while (n >= 8) {
		__builtin_memcpy(o, &w, 8);
		o += 8;
		n -= 8;
	}
	while (n--) {
		*o++ = (unsigned char)c;
	}
	return d;
}

NL_USED NL_NOBUILTIN int memcmp(const void *a, const void *b, size_t n) {
	const unsigned char *x = a, *y = b;
	size_t i = 0;
	for (; i + 8 <= n; i += 8) {
		uint64_t u, v;
		__builtin_memcpy(&u, x + i, 8);
		__builtin_memcpy(&v, y + i, 8);
		if (u != v) {
			break;
		}
	}
	for (; i < n; i++) {
		if (x[i] != y[i]) {
			return x[i] < y[i] ? -1 : 1;
		}
	}
	return 0;
}

NL_USED NL_NOBUILTIN int bcmp(const void *a, const void *b, size_t n) { return memcmp(a, b, n); }

NL_USED NL_NOBUILTIN void *memchr(const void *s, int c, size_t n) {
	const unsigned char *p = s;
	for (size_t i = 0; i < n; i++) {
		if (p[i] == (unsigned char)c) {
			return (void *)(p + i);
		}
	}
	return NULL;
}

NL_USED NL_NOBUILTIN size_t strlen(const char *s) {
	size_t n = 0;
	while (s[n]) {
		n++;
	}
	return n;
}

NL_USED NL_NOBUILTIN char *strchr(const char *s, int c) {
	for (;; s++) {
		if (*s == (char)c) {
			return (char *)s;
		}
		if (!*s) {
			return NULL;
		}
	}
}

NL_USED NL_NOBUILTIN char *strrchr(const char *s, int c) {
	const char *r = NULL;
	for (;; s++) {
		if (*s == (char)c) {
			r = s;
		}
		if (!*s) {
			return (char *)r;
		}
	}
}

NL_USED NL_NOBUILTIN int strcmp(const char *a, const char *b) {
	while (*a && *a == *b) {
		a++;
		b++;
	}
	return (unsigned char)*a - (unsigned char)*b;
}

NL_USED NL_NOBUILTIN int strncmp(const char *a, const char *b, size_t n) {
	for (; n; n--, a++, b++) {
		if (*a != *b || !*a) {
			return (unsigned char)*a - (unsigned char)*b;
		}
	}
	return 0;
}

static int nl_lower(int c) { return c >= 'A' && c <= 'Z' ? c + 32 : c; }

NL_USED NL_NOBUILTIN int strncasecmp(const char *a, const char *b, size_t n) {
	for (; n; n--, a++, b++) {
		int x = nl_lower((unsigned char)*a), y = nl_lower((unsigned char)*b);
		if (x != y || !x) {
			return x - y;
		}
	}
	return 0;
}

NL_USED NL_NOBUILTIN char *strcpy(char *d, const char *s) {
	char *o = d;
	while ((*o++ = *s++)) {
	}
	return d;
}

NL_USED NL_NOBUILTIN char *strstr(const char *h, const char *n) {
	size_t k = strlen(n);
	for (; *h; h++) {
		if (strncmp(h, n, k) == 0) {
			return (char *)h;
		}
	}
	return k == 0 ? (char *)h : NULL;
}

NL_USED NL_NOBUILTIN char *strpbrk(const char *s, const char *set) {
	for (; *s; s++) {
		if (strchr(set, *s)) {
			return (char *)s;
		}
	}
	return NULL;
}

NL_USED char *strdup(const char *s) {
	size_t n = strlen(s) + 1;
	char *d = malloc(n);
	if (d) {
		memcpy(d, s, n);
	}
	return d;
}

NL_USED char *strndup(const char *s, size_t m) {
	size_t n = 0;
	while (n < m && s[n]) {
		n++;
	}
	char *d = malloc(n + 1);
	if (d) {
		memcpy(d, s, n);
		d[n] = 0;
	}
	return d;
}

// glibc 2.34's messages (std/native/llrt/linux.xo strerror has the same
// table), which the C runtime's file errors report.
static const char *const nl_errors[] = {
	"Success",
	"Operation not permitted",
	"No such file or directory",
	"No such process",
	"Interrupted system call",
	"Input/output error",
	"No such device or address",
	"Argument list too long",
	"Exec format error",
	"Bad file descriptor",
	"No child processes",
	"Resource temporarily unavailable",
	"Cannot allocate memory",
	"Permission denied",
	"Bad address",
	"Block device required",
	"Device or resource busy",
	"File exists",
	"Invalid cross-device link",
	"No such device",
	"Not a directory",
	"Is a directory",
	"Invalid argument",
	"Too many open files in system",
	"Too many open files",
	"Inappropriate ioctl for device",
	"Text file busy",
	"File too large",
	"No space left on device",
	"Illegal seek",
	"Read-only file system",
	"Too many links",
	"Broken pipe",
	"Numerical argument out of domain",
	"Numerical result out of range",
	"Resource deadlock avoided",
	"File name too long",
	"No locks available",
	"Function not implemented",
	"Directory not empty",
	"Too many levels of symbolic links",
	"Unknown error 41",
	"No message of desired type",
	"Identifier removed",
	"Channel number out of range",
	"Level 2 not synchronized",
	"Level 3 halted",
	"Level 3 reset",
	"Link number out of range",
	"Protocol driver not attached",
	"No CSI structure available",
	"Level 2 halted",
	"Invalid exchange",
	"Invalid request descriptor",
	"Exchange full",
	"No anode",
	"Invalid request code",
	"Invalid slot",
	"Unknown error 58",
	"Bad font file format",
	"Device not a stream",
	"No data available",
	"Timer expired",
	"Out of streams resources",
	"Machine is not on the network",
	"Package not installed",
	"Object is remote",
	"Link has been severed",
	"Advertise error",
	"Srmount error",
	"Communication error on send",
	"Protocol error",
	"Multihop attempted",
	"RFS specific error",
	"Bad message",
	"Value too large for defined data type",
	"Name not unique on network",
	"File descriptor in bad state",
	"Remote address changed",
	"Can not access a needed shared library",
	"Accessing a corrupted shared library",
	".lib section in a.out corrupted",
	"Attempting to link in too many shared libraries",
	"Cannot exec a shared library directly",
	"Invalid or incomplete multibyte or wide character",
	"Interrupted system call should be restarted",
	"Streams pipe error",
	"Too many users",
	"Socket operation on non-socket",
	"Destination address required",
	"Message too long",
	"Protocol wrong type for socket",
	"Protocol not available",
	"Protocol not supported",
	"Socket type not supported",
	"Operation not supported",
	"Protocol family not supported",
	"Address family not supported by protocol",
	"Address already in use",
	"Cannot assign requested address",
	"Network is down",
	"Network is unreachable",
	"Network dropped connection on reset",
	"Software caused connection abort",
	"Connection reset by peer",
	"No buffer space available",
	"Transport endpoint is already connected",
	"Transport endpoint is not connected",
	"Cannot send after transport endpoint shutdown",
	"Too many references: cannot splice",
	"Connection timed out",
	"Connection refused",
	"Host is down",
	"No route to host",
	"Operation already in progress",
	"Operation now in progress",
	"Stale file handle",
	"Structure needs cleaning",
	"Not a XENIX named type file",
	"No XENIX semaphores available",
	"Is a named type file",
	"Remote I/O error",
	"Disk quota exceeded",
	"No medium found",
	"Wrong medium type",
	"Operation canceled",
	"Required key not available",
	"Key has expired",
	"Key has been revoked",
	"Key was rejected by service",
	"Owner died",
	"State not recoverable",
	"Operation not possible due to RF-kill",
	"Memory page has hardware error",
};

NL_USED char *strerror(int e) {
	static __thread char unknown[32];
	if (e >= 0 && e < (int)(sizeof nl_errors / sizeof nl_errors[0])) {
		return (char *)nl_errors[e];
	}
	snprintf(unknown, sizeof unknown, "Unknown error %d", e);
	return unknown;
}

static unsigned long long nl_strtoull(const char *s, char **end, int base, int *neg) {
	while (*s == ' ' || (*s >= '\t' && *s <= '\r')) {
		s++;
	}
	*neg = 0;
	if (*s == '-' || *s == '+') {
		*neg = *s == '-';
		s++;
	}
	if ((base == 0 || base == 16) && s[0] == '0' && (s[1] == 'x' || s[1] == 'X')) {
		s += 2;
		base = 16;
	} else if (base == 0) {
		base = s[0] == '0' ? 8 : 10;
	}
	unsigned long long v = 0;
	const char *start = s;
	for (;; s++) {
		int d;
		if (*s >= '0' && *s <= '9') {
			d = *s - '0';
		} else if (*s >= 'a' && *s <= 'z') {
			d = *s - 'a' + 10;
		} else if (*s >= 'A' && *s <= 'Z') {
			d = *s - 'A' + 10;
		} else {
			break;
		}
		if (d >= base) {
			break;
		}
		v = v * (unsigned)base + (unsigned)d;
	}
	if (end) {
		*end = (char *)(s == start ? start : s);
	}
	return v;
}

NL_USED unsigned long long strtoull(const char *s, char **end, int base) {
	int neg;
	unsigned long long v = nl_strtoull(s, end, base, &neg);
	return neg ? -v : v;
}

NL_USED unsigned long strtoul(const char *s, char **end, int base) { return (unsigned long)strtoull(s, end, base); }

NL_USED long long strtoll(const char *s, char **end, int base) {
	int neg;
	unsigned long long v = nl_strtoull(s, end, base, &neg);
	return neg ? -(long long)v : (long long)v;
}

NL_USED long strtol(const char *s, char **end, int base) { return (long)strtoll(s, end, base); }

NL_USED int atoi(const char *s) { return (int)strtol(s, NULL, 10); }

NL_USED long atol(const char *s) { return strtol(s, NULL, 10); }

NL_USED long long atoll(const char *s) { return strtoll(s, NULL, 10); }

NL_USED double strtod(const char *s, char **end) {
	(void)s;
	(void)end;
	nl_die("strtod (floating point text in C) is not available");
}

// qsort: a shell sort (the runtime sorts header and channel lists).
NL_USED void qsort(void *base, size_t n, size_t w, int (*cmp)(const void *, const void *)) {
	char *b = base;
	char tmp[256];
	char *t = w <= sizeof tmp ? tmp : malloc(w);
	for (size_t gap = n / 2; gap > 0; gap /= 2) {
		for (size_t i = gap; i < n; i++) {
			memcpy(t, b + i * w, w);
			size_t j = i;
			while (j >= gap && cmp(b + (j - gap) * w, t) > 0) {
				memcpy(b + j * w, b + (j - gap) * w, w);
				j -= gap;
			}
			memcpy(b + j * w, t, w);
		}
	}
	if (t != tmp) {
		free(t);
	}
}

// ---------------------------------------------------------------------------
// Formatted output (no floating point)

typedef struct {
	char *p;
	size_t n, cap;
} nl_out;

static void nl_put(nl_out *o, char c) {
	if (o->n + 1 < o->cap) {
		o->p[o->n] = c;
	}
	o->n++;
}

static void nl_pad(nl_out *o, char c, int k) {
	while (k-- > 0) {
		nl_put(o, c);
	}
}

NL_USED int vsnprintf(char *restrict buf, size_t cap, const char *restrict f, va_list ap) {
	nl_out o = {buf, 0, cap};
	for (; *f; f++) {
		if (*f != '%') {
			nl_put(&o, *f);
			continue;
		}
		f++;
		int left = 0, zero = 0, plus = 0, space = 0, alt = 0;
		for (;; f++) {
			if (*f == '-') {
				left = 1;
			} else if (*f == '0') {
				zero = 1;
			} else if (*f == '+') {
				plus = 1;
			} else if (*f == ' ') {
				space = 1;
			} else if (*f == '#') {
				alt = 1;
			} else {
				break;
			}
		}
		int width = 0, prec = -1;
		if (*f == '*') {
			width = va_arg(ap, int);
			if (width < 0) {
				left = 1;
				width = -width;
			}
			f++;
		} else {
			while (*f >= '0' && *f <= '9') {
				width = width * 10 + (*f++ - '0');
			}
		}
		if (*f == '.') {
			f++;
			prec = 0;
			if (*f == '*') {
				prec = va_arg(ap, int);
				f++;
			} else {
				while (*f >= '0' && *f <= '9') {
					prec = prec * 10 + (*f++ - '0');
				}
			}
		}
		int lng = 0; // 1 long, 2 long long, -1 short, -2 char
		for (;; f++) {
			if (*f == 'l') {
				lng++;
			} else if (*f == 'z' || *f == 'j' || *f == 't') {
				lng = 2;
			} else if (*f == 'h') {
				lng--;
			} else {
				break;
			}
		}
		char c = *f;
		if (c == 0) {
			break;
		}
		char num[72];
		int nn = 0;
		const char *s = NULL;
		size_t sn = 0;
		char sign = 0;
		switch (c) {
		case '%':
			nl_put(&o, '%');
			continue;
		case 'c':
			num[0] = (char)va_arg(ap, int);
			s = num;
			sn = 1;
			break;
		case 's':
			s = va_arg(ap, const char *);
			if (!s) {
				s = "(null)";
			}
			sn = 0;
			while (s[sn] && (prec < 0 || sn < (size_t)prec)) {
				sn++;
			}
			break;
		case 'd':
		case 'i':
		case 'u':
		case 'x':
		case 'X':
		case 'o':
		case 'p': {
			unsigned long long v;
			int base = c == 'x' || c == 'X' || c == 'p' ? 16 : (c == 'o' ? 8 : 10);
			if (c == 'p') {
				v = (unsigned long long)(uintptr_t)va_arg(ap, void *);
				alt = 1;
			} else if (c == 'd' || c == 'i') {
				long long x = lng >= 2 ? va_arg(ap, long long) : (lng == 1 ? va_arg(ap, long) : va_arg(ap, int));
				if (lng == -1) {
					x = (short)x;
				} else if (lng <= -2) {
					x = (signed char)x;
				}
				if (x < 0) {
					sign = '-';
					v = 0 - (unsigned long long)x;
				} else {
					sign = plus ? '+' : (space ? ' ' : 0);
					v = (unsigned long long)x;
				}
			} else {
				v = lng >= 2 ? va_arg(ap, unsigned long long) : (lng == 1 ? va_arg(ap, unsigned long) : va_arg(ap, unsigned int));
				if (lng == -1) {
					v = (unsigned short)v;
				} else if (lng <= -2) {
					v = (unsigned char)v;
				}
			}
			const char *dig = c == 'X' ? "0123456789ABCDEF" : "0123456789abcdef";
			char t[64];
			int k = 0;
			do {
				t[k++] = dig[v % (unsigned)base];
				v /= (unsigned)base;
			} while (v);
			while (k < prec) {
				t[k++] = '0';
			}
			if (alt && base == 16) {
				num[nn++] = '0';
				num[nn++] = c == 'X' ? 'X' : 'x';
			}
			while (k) {
				num[nn++] = t[--k];
			}
			s = num;
			sn = (size_t)nn;
			break;
		}
		default:
			nl_die("printf of a floating point value is not available");
		}
		int body = (int)sn + (sign ? 1 : 0);
		if (!left && !(zero && prec < 0 && c != 's' && c != 'c')) {
			nl_pad(&o, ' ', width - body);
		}
		if (sign) {
			nl_put(&o, sign);
		}
		if (!left && zero && prec < 0 && c != 's' && c != 'c') {
			nl_pad(&o, '0', width - body);
		}
		for (size_t i = 0; i < sn; i++) {
			nl_put(&o, s[i]);
		}
		if (left) {
			nl_pad(&o, ' ', width - body);
		}
	}
	if (cap > 0) {
		buf[o.n < cap ? o.n : cap - 1] = 0;
	}
	return (int)o.n;
}

NL_USED int snprintf(char *restrict buf, size_t cap, const char *restrict f, ...) {
	va_list ap;
	va_start(ap, f);
	int n = vsnprintf(buf, cap, f, ap);
	va_end(ap);
	return n;
}

NL_USED int sprintf(char *restrict buf, const char *restrict f, ...) {
	va_list ap;
	va_start(ap, f);
	int n = vsnprintf(buf, (size_t)1 << 30, f, ap);
	va_end(ap);
	return n;
}

// ---------------------------------------------------------------------------
// Streams: a FILE is a descriptor (unbuffered: the runtime buffers
// standard output itself)

typedef struct {
	int fd;
	int err;
} nl_file;

static nl_file nl_std[3] = {{0, 0}, {1, 0}, {2, 0}};
FILE *stdin = (FILE *)&nl_std[0];
FILE *stdout = (FILE *)&nl_std[1];
FILE *stderr = (FILE *)&nl_std[2];

static int nl_fd(FILE *f) { return ((nl_file *)f)->fd; }

static size_t nl_write_all(int fd, const char *p, size_t n) {
	size_t off = 0;
	while (off < n) {
		long w = nl_sys3(SYS_write, fd, p + off, n - off);
		if (w == -EINTR) {
			continue;
		}
		if (w <= 0) {
			break;
		}
		off += (size_t)w;
	}
	return off;
}

NL_USED size_t fwrite(const void *restrict p, size_t size, size_t n, FILE *restrict f) {
	size_t t = size * n;
	size_t w = nl_write_all(nl_fd(f), p, t);
	if (w < t) {
		((nl_file *)f)->err = 1;
	}
	return size ? w / size : 0;
}

NL_USED int fputs(const char *restrict s, FILE *restrict f) {
	size_t n = strlen(s);
	return fwrite(s, 1, n, f) == n ? 0 : -1;
}

NL_USED int fputc(int c, FILE *f) {
	char b = (char)c;
	return fwrite(&b, 1, 1, f) == 1 ? (unsigned char)c : -1;
}

NL_USED int putc(int c, FILE *f) { return fputc(c, f); }

NL_USED int puts(const char *s) {
	fputs(s, stdout);
	return fputc('\n', stdout);
}

NL_USED int putchar(int c) { return fputc(c, stdout); }

NL_USED int fflush(FILE *f) {
	(void)f;
	return 0;
}

NL_USED int vfprintf(FILE *restrict f, const char *restrict fmt, va_list ap) {
	char small[1024];
	va_list aq;
	va_copy(aq, ap);
	int n = vsnprintf(small, sizeof small, fmt, ap);
	if (n < (int)sizeof small) {
		fwrite(small, 1, (size_t)n, f);
	} else {
		char *big = malloc((size_t)n + 1);
		if (big) {
			vsnprintf(big, (size_t)n + 1, fmt, aq);
			fwrite(big, 1, (size_t)n, f);
			free(big);
		}
	}
	va_end(aq);
	return n;
}

NL_USED int fprintf(FILE *restrict f, const char *restrict fmt, ...) {
	va_list ap;
	va_start(ap, fmt);
	int n = vfprintf(f, fmt, ap);
	va_end(ap);
	return n;
}

NL_USED int printf(const char *restrict fmt, ...) {
	va_list ap;
	va_start(ap, fmt);
	int n = vfprintf(stdout, fmt, ap);
	va_end(ap);
	return n;
}

NL_USED FILE *fopen(const char *restrict path, const char *restrict mode) {
	int flags = O_RDONLY;
	if (mode[0] == 'w') {
		flags = O_WRONLY | O_CREAT | O_TRUNC;
	} else if (mode[0] == 'a') {
		flags = O_WRONLY | O_CREAT | O_APPEND;
	}
	if (strchr(mode, '+')) {
		flags = (flags & ~(O_RDONLY | O_WRONLY)) | O_RDWR;
	}
	long fd = nl_ret(nl_sys4(SYS_openat, AT_FDCWD, path, flags | O_CLOEXEC, 0666));
	if (fd < 0) {
		return NULL;
	}
	nl_file *f = malloc(sizeof *f);
	if (!f) {
		nl_sys1(SYS_close, fd);
		return NULL;
	}
	f->fd = (int)fd;
	f->err = 0;
	return (FILE *)f;
}

NL_USED size_t fread(void *restrict p, size_t size, size_t n, FILE *restrict f) {
	size_t t = size * n, off = 0;
	while (off < t) {
		long r = nl_sys3(SYS_read, nl_fd(f), (char *)p + off, t - off);
		if (r == -EINTR) {
			continue;
		}
		if (r < 0) {
			nl_errno = (int)-r;
			((nl_file *)f)->err = 1;
			break;
		}
		if (r == 0) {
			break;
		}
		off += (size_t)r;
	}
	return size ? off / size : 0;
}

NL_USED int ferror(FILE *f) { return ((nl_file *)f)->err; }

NL_USED int fclose(FILE *f) {
	long r = nl_sys1(SYS_close, nl_fd(f));
	if ((nl_file *)f < nl_std || (nl_file *)f > nl_std + 2) {
		free(f);
	}
	return (int)nl_ret(r);
}

// ---------------------------------------------------------------------------
// Files, directories, descriptors

NL_USED int open(const char *path, int flags, ...) {
	va_list ap;
	va_start(ap, flags);
	int mode = (flags & O_CREAT) ? va_arg(ap, int) : 0;
	va_end(ap);
	return (int)nl_ret(nl_sys4(SYS_openat, AT_FDCWD, path, flags, mode));
}

NL_USED int openat(int dirfd, const char *path, int flags, ...) {
	va_list ap;
	va_start(ap, flags);
	int mode = (flags & O_CREAT) ? va_arg(ap, int) : 0;
	va_end(ap);
	return (int)nl_ret(nl_sys4(SYS_openat, dirfd, path, flags, mode));
}

NL_USED ssize_t read(int fd, void *p, size_t n) { return nl_ret(nl_sys3(SYS_read, fd, p, n)); }

NL_USED ssize_t write(int fd, const void *p, size_t n) { return nl_ret(nl_sys3(SYS_write, fd, p, n)); }

NL_USED ssize_t pread(int fd, void *p, size_t n, off_t off) { return nl_ret(nl_sys4(SYS_pread64, fd, p, n, off)); }

NL_USED int close(int fd) { return (int)nl_ret(nl_sys1(SYS_close, fd)); }

NL_USED off_t lseek(int fd, off_t off, int whence) { return nl_ret(nl_sys3(SYS_lseek, fd, off, whence)); }

NL_USED int pipe2(int fds[2], int flags) { return (int)nl_ret(nl_sys2(SYS_pipe2, fds, flags)); }

NL_USED int pipe(int fds[2]) { return pipe2(fds, 0); }

NL_USED int dup2(int a, int b) { return (int)nl_ret(nl_sys3(SYS_dup3, a, b, 0)); }

NL_USED int fcntl(int fd, int cmd, ...) {
	va_list ap;
	va_start(ap, cmd);
	long arg = va_arg(ap, long);
	va_end(ap);
	return (int)nl_ret(nl_sys3(SYS_fcntl, fd, cmd, arg));
}

NL_USED int ioctl(int fd, unsigned long req, ...) {
	va_list ap;
	va_start(ap, req);
	long arg = va_arg(ap, long);
	va_end(ap);
	return (int)nl_ret(nl_sys3(SYS_ioctl, fd, req, arg));
}

NL_USED int isatty(int fd) {
	char t[64];
	return nl_sys3(SYS_ioctl, fd, 0x5401 /* TCGETS */, t) == 0;
}

NL_USED int unlink(const char *p) { return (int)nl_ret(nl_sys3(SYS_unlinkat, AT_FDCWD, p, 0)); }

NL_USED int rmdir(const char *p) { return (int)nl_ret(nl_sys3(SYS_unlinkat, AT_FDCWD, p, AT_REMOVEDIR)); }

NL_USED int mkdir(const char *p, mode_t m) { return (int)nl_ret(nl_sys3(SYS_mkdirat, AT_FDCWD, p, m)); }

NL_USED int rename(const char *a, const char *b) { return (int)nl_ret(nl_sys6(SYS_renameat2, AT_FDCWD, (long)a, AT_FDCWD, (long)b, 0, 0)); }

NL_USED int chmod(const char *p, mode_t m) { return (int)nl_ret(nl_sys3(SYS_fchmodat, AT_FDCWD, p, m)); }

NL_USED int symlink(const char *t, const char *p) { return (int)nl_ret(nl_sys3(SYS_symlinkat, t, AT_FDCWD, p)); }

NL_USED ssize_t readlink(const char *restrict p, char *restrict buf, size_t n) { return nl_ret(nl_sys4(SYS_readlinkat, AT_FDCWD, p, buf, n)); }

NL_USED int ftruncate(int fd, off_t n) { return (int)nl_ret(nl_sys2(SYS_ftruncate, fd, n)); }

NL_USED int fsync(int fd) { return (int)nl_ret(nl_sys1(SYS_fsync, fd)); }

NL_USED int flock(int fd, int op) { return (int)nl_ret(nl_sys2(SYS_flock, fd, op)); }

NL_USED char *getcwd(char *buf, size_t n) {
	long r = nl_ret(nl_sys2(SYS_getcwd, buf, n));
	return r < 0 ? NULL : buf;
}

NL_USED int stat(const char *restrict p, struct stat *restrict st) { return (int)nl_ret(nl_sys4(SYS_newfstatat, AT_FDCWD, p, st, 0)); }

NL_USED int lstat(const char *restrict p, struct stat *restrict st) {
	return (int)nl_ret(nl_sys4(SYS_newfstatat, AT_FDCWD, p, st, AT_SYMLINK_NOFOLLOW));
}

NL_USED int fstat(int fd, struct stat *st) { return (int)nl_ret(nl_sys4(SYS_newfstatat, fd, "", st, AT_EMPTY_PATH)); }

NL_USED int utimensat(int dirfd, const char *p, const struct timespec t[2], int flags) {
	return (int)nl_ret(nl_sys4(SYS_utimensat, dirfd, p, t, flags));
}

// A DIR reads entries with getdents64: its records are glibc's struct
// dirent on 64 bit Linux.
struct __dirstream {
	int fd;
	long pos, len;
	char buf[16384];
};

NL_USED DIR *opendir(const char *p) {
	long fd = nl_ret(nl_sys4(SYS_openat, AT_FDCWD, p, O_RDONLY | O_DIRECTORY | O_CLOEXEC, 0));
	if (fd < 0) {
		return NULL;
	}
	DIR *d = malloc(sizeof *d);
	if (!d) {
		nl_sys1(SYS_close, fd);
		return NULL;
	}
	d->fd = (int)fd;
	d->pos = d->len = 0;
	return d;
}

struct dirent *readdir(DIR *d) {
	if (d->pos >= d->len) {
		long n = nl_sys3(SYS_getdents64, d->fd, d->buf, sizeof d->buf);
		if (n <= 0) {
			if (n < 0) {
				nl_errno = (int)-n;
			}
			return NULL;
		}
		d->len = n;
		d->pos = 0;
	}
	struct dirent *e = (struct dirent *)(d->buf + d->pos);
	d->pos += e->d_reclen;
	return e;
}

NL_USED int closedir(DIR *d) {
	int r = (int)nl_ret(nl_sys1(SYS_close, d->fd));
	free(d);
	return r;
}

// ---------------------------------------------------------------------------
// Memory mappings, the clock, randomness, the system

NL_USED void *mmap(void *a, size_t n, int prot, int flags, int fd, off_t off) {
	long r = nl_sys6(SYS_mmap, (long)a, (long)n, prot, flags, fd, off);
	if (r < 0 && r > -4096) {
		nl_errno = (int)-r;
		return MAP_FAILED;
	}
	return (void *)r;
}

NL_USED int munmap(void *a, size_t n) { return (int)nl_ret(nl_sys2(SYS_munmap, a, n)); }

NL_USED int mprotect(void *a, size_t n, int prot) { return (int)nl_ret(nl_sys3(SYS_mprotect, a, n, prot)); }

NL_USED int madvise(void *a, size_t n, int adv) { return (int)nl_ret(nl_sys3(SYS_madvise, a, n, adv)); }

NL_USED int clock_gettime(clockid_t id, struct timespec *ts) { return (int)nl_ret(xo_x_clock_gettime(id, (int64_t)ts)); }

NL_USED time_t time(time_t *t) {
	struct timespec ts;
	clock_gettime(CLOCK_REALTIME, &ts);
	if (t) {
		*t = ts.tv_sec;
	}
	return ts.tv_sec;
}

NL_USED int nanosleep(const struct timespec *req, struct timespec *rem) { return (int)nl_ret(nl_sys2(SYS_nanosleep, req, rem)); }

// gmtime_r: the civil date of Unix seconds (Howard Hinnant's algorithm).
struct tm *gmtime_r(const time_t *restrict t, struct tm *restrict tm) {
	long long s = *t;
	long long days = s / 86400, rem = s % 86400;
	if (rem < 0) {
		rem += 86400;
		days--;
	}
	tm->tm_hour = (int)(rem / 3600);
	tm->tm_min = (int)(rem % 3600 / 60);
	tm->tm_sec = (int)(rem % 60);
	tm->tm_wday = (int)((days + 4) % 7 + 7) % 7;
	long long z = days + 719468;
	long long era = (z >= 0 ? z : z - 146096) / 146097;
	long long doe = z - era * 146097;
	long long yoe = (doe - doe / 1460 + doe / 36524 - doe / 146096) / 365;
	long long y = yoe + era * 400;
	long long doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
	long long mp = (5 * doy + 2) / 153;
	long long d = doy - (153 * mp + 2) / 5 + 1;
	long long m = mp < 10 ? mp + 3 : mp - 9;
	y += m <= 2;
	tm->tm_year = (int)(y - 1900);
	tm->tm_mon = (int)(m - 1);
	tm->tm_mday = (int)d;
	static const int before[12] = {0, 31, 59, 90, 120, 151, 181, 212, 243, 273, 304, 334};
	int leap = (y % 4 == 0 && y % 100 != 0) || y % 400 == 0;
	tm->tm_yday = before[m - 1] + (int)d - 1 + (leap && m > 2);
	tm->tm_isdst = 0;
	return tm;
}

NL_USED ssize_t getrandom(void *p, size_t n, unsigned flags) { return nl_ret(nl_sys3(SYS_getrandom, p, n, flags)); }

NL_USED int getentropy(void *p, size_t n) {
	size_t off = 0;
	while (off < n) {
		long r = nl_sys3(SYS_getrandom, (char *)p + off, n - off, 0);
		if (r == -EINTR) {
			continue;
		}
		if (r < 0) {
			nl_errno = (int)-r;
			return -1;
		}
		off += (size_t)r;
	}
	return 0;
}

NL_USED char *getenv(const char *name) {
	size_t n = strlen(name);
	for (char **e = environ; e && *e; e++) {
		if (strncmp(*e, name, n) == 0 && (*e)[n] == '=') {
			return *e + n + 1;
		}
	}
	return NULL;
}

NL_USED pid_t getpid(void) { return (pid_t)nl_sys0(SYS_getpid); }

NL_USED int kill(pid_t pid, int sig) { return (int)nl_ret(nl_sys2(SYS_kill, pid, sig)); }

NL_USED long sysconf(int name) {
	if (name == _SC_PAGESIZE) {
		return nl_pagesz;
	}
	if (name == _SC_NPROCESSORS_ONLN || name == _SC_NPROCESSORS_CONF) {
		unsigned long set[16] = {0};
		long r = nl_sys3(SYS_sched_getaffinity, 0, sizeof set, set);
		long n = 0;
		for (long i = 0; r > 0 && i < r / 8; i++) {
			n += __builtin_popcountl(set[i]);
		}
		return n > 0 ? n : 1;
	}
	nl_errno = EINVAL;
	return -1;
}

NL_USED long syscall(long n, ...) {
	va_list ap;
	va_start(ap, n);
	long a = va_arg(ap, long), b = va_arg(ap, long), c = va_arg(ap, long);
	long d = va_arg(ap, long), e = va_arg(ap, long), f = va_arg(ap, long);
	va_end(ap);
	return nl_ret(nl_sys6(n, a, b, c, d, e, f));
}

// ---------------------------------------------------------------------------
// Signals (in Xo: cabi_nolibc.xo sigaction, with the emitters'
// rt_sigreturn trampoline).

NL_USED int sigemptyset(sigset_t *s) {
	memset(s, 0, sizeof *s);
	return 0;
}

NL_USED int sigaddset(sigset_t *s, int sig) {
	((unsigned long *)s)[(sig - 1) / 64] |= 1UL << ((sig - 1) % 64);
	return 0;
}

NL_USED int sigaction(int sig, const struct sigaction *restrict act, struct sigaction *restrict old) {
	return (int)nl_ret(xo_x_sigaction(sig, (int64_t)act, (int64_t)old));
}

NL_USED void (*signal(int sig, void (*h)(int)))(int) {
	long r = nl_ret(xo_x_signal(sig, (int64_t)h));
	return r == -1 ? SIG_ERR : (void (*)(int))r;
}

NL_USED int sigprocmask(int how, const sigset_t *restrict set, sigset_t *restrict old) {
	return (int)nl_ret(xo_x_sigprocmask(how, (int64_t)set, (int64_t)old));
}

NL_USED int pthread_sigmask(int how, const sigset_t *restrict set, sigset_t *restrict old) {
	return sigprocmask(how, set, old) == 0 ? 0 : nl_errno;
}

// ---------------------------------------------------------------------------
// Locks (in Xo: cabi_nolibc.xo, futex based; a mutex is the first word
// of glibc's pthread_mutex_t).

NL_USED int pthread_mutex_init(pthread_mutex_t *restrict m, const pthread_mutexattr_t *restrict a) {
	(void)a;
	memset(m, 0, sizeof *m);
	return 0;
}

NL_USED int pthread_mutex_destroy(pthread_mutex_t *m) {
	(void)m;
	return 0;
}

NL_USED int pthread_mutex_lock(pthread_mutex_t *m) { return (int)xo_x_mutex_lock((int64_t)m); }

NL_USED int pthread_mutex_trylock(pthread_mutex_t *m) { return (int)xo_x_mutex_trylock((int64_t)m); }

NL_USED int pthread_mutex_unlock(pthread_mutex_t *m) { return (int)xo_x_mutex_unlock((int64_t)m); }

NL_USED int pthread_once(pthread_once_t *o, void (*f)(void)) { return (int)xo_x_once_run((int64_t)o, (int64_t)f); }

// Threads (in Xo: cabi_nolibc.xo thread_create and thread_join, clone
// through the emitters' trampoline). The attributes the runtime sets: the
// stack size (detached threads' mappings are never reused: the runtime
// detaches its poller and workers, which live until the program ends).

NL_USED int pthread_attr_init(pthread_attr_t *a) {
	memset(a, 0, sizeof *a);
	*(size_t *)a = 8 << 20;
	return 0;
}

NL_USED int pthread_attr_destroy(pthread_attr_t *a) {
	(void)a;
	return 0;
}

NL_USED int pthread_attr_setstacksize(pthread_attr_t *a, size_t n) {
	*(size_t *)a = n;
	return 0;
}

NL_USED int pthread_attr_setdetachstate(pthread_attr_t *a, int s) {
	(void)a;
	(void)s;
	return 0;
}

NL_USED int pthread_create(pthread_t *restrict tp, const pthread_attr_t *restrict a, void *(*f)(void *), void *restrict arg) {
	long r = (long)xo_x_thread_create(a ? (int64_t) * (const size_t *)a : 8 << 20, (int64_t)f, (int64_t)arg);
	if (r < 0) {
		return (int)-r;
	}
	*tp = (pthread_t)r;
	return 0;
}

NL_USED int pthread_join(pthread_t th, void **r) {
	xo_x_thread_join((int64_t)th);
	if (r) {
		*r = NULL; // the runtime's threads return nothing it reads
	}
	return 0;
}

NL_USED int pthread_detach(pthread_t th) {
	(void)th;
	return 0;
}

NL_USED int sched_yield(void) { return (int)nl_ret(nl_sys0(SYS_sched_yield)); }

// ---------------------------------------------------------------------------
// Sockets, the poller's epoll, and name resolution are not here: the C
// runtime calls the Xo exports of std/native/llrt/cabi_net.xo and
// cabi_poll.xo for them (sock.xo, dns.xo; decision 0206 stage 3).

// ---------------------------------------------------------------------------
// Unwinding is not here: fault_raise calls the Xo unwinder
// (std/native/llrt/unwind.xo through cabi.xo's unwind_fault); the unwind
// tables name cabi.xo's export xo_x_personality, and the link defines
// xo_personality as it for linux/amd64's (decision 0221). No C library
// unwinder function is called.

#endif
