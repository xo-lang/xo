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
//   - files, directories, pipes, epoll, the clock (the system call, not
//     the vDSO), getrandom, signals (rt_sigaction with its own restorer);
//   - locks (futex based mutexes, once), threads (clone with a stack and
//     a TLS block per thread, joined through CLONE_CHILD_CLEARTID);
//   - not here (stage 2): unwinding (_Unwind_ForcedUnwind finds no
//     frames: a fault is reported and ends the program, without running
//     deferred code), floating point printf conversions and strtod (they
//     end the program with a message); stage 3: sockets past shutdown,
//     DNS (getaddrinfo is not defined).
// The system call layer of Xo code is std/native/llrt/linux.xo; this
// file serves the C runtime only, and shrinks as the runtime moves to Xo.

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

// The static TLS block of every thread is laid out from the program's
// PT_TLS segment: variant I on arm64 (the thread pointer at a 16 byte
// control block, the block after it), variant II on x86_64 (the block
// before the thread pointer, which points at itself).
static const Elf64_Phdr *nl_tls_ph;

static unsigned long nl_tls_align(void) { return nl_tls_ph && nl_tls_ph->p_align > 16 ? nl_tls_ph->p_align : 16; }

static unsigned long nl_tls_block(void) {
	unsigned long a = nl_tls_align();
	return ((nl_tls_ph ? nl_tls_ph->p_memsz : 0) + a - 1) & ~(a - 1);
}

// nl_tls_size is the bytes nl_tls_init needs (page aligned memory).
static unsigned long nl_tls_size(void) { return nl_tls_block() + 2 * nl_tls_align() + 64; }

// nl_tls_init lays a TLS block out in zeroed memory at area and returns
// its thread pointer.
static char *nl_tls_init(char *area) {
	unsigned long filesz = nl_tls_ph ? nl_tls_ph->p_filesz : 0;
#if defined(__aarch64__)
	char *tp = area;
	unsigned long a = nl_tls_align();
	char *data = tp + ((16 + a - 1) & ~(a - 1));
#else
	char *tp = area + nl_tls_block();
	char *data = tp - nl_tls_block();
	*(char **)tp = tp;
#endif
	if (filesz) {
		__builtin_memcpy(data, (const void *)nl_tls_ph->p_vaddr, filesz);
	}
	return tp;
}

// nl_tls sets up the main thread's TLS block and thread pointer.
static void nl_tls(const Elf64_Phdr *ph, long phnum) {
	for (long i = 0; ph && i < phnum; i++) {
		if (ph[i].p_type == PT_TLS) {
			nl_tls_ph = &ph[i];
		}
	}
	long area = nl_sys6(SYS_mmap, 0, (long)nl_tls_size(), PROT_READ | PROT_WRITE, MAP_PRIVATE | MAP_ANONYMOUS, -1, 0);
	if (area < 0 && area > -4096) {
		nl_die("mmap of the TLS block failed");
	}
	char *tp = nl_tls_init((char *)area);
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
	const Elf64_Phdr *ph = NULL;
	long phnum = 0;
	for (long *a = (long *)(e + 1); a[0] != AT_NULL; a += 2) {
		switch (a[0]) {
		case AT_PHDR:
			ph = (const Elf64_Phdr *)a[1];
			break;
		case AT_PHNUM:
			phnum = a[1];
			break;
		case AT_PAGESZ:
			nl_pagesz = a[1];
			break;
		}
	}
	nl_tls(ph, phnum);
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
// rt_sigaction's sa_restorer: rt_sigreturn
__asm__(".text\n.type xo_nolibc_restorer, %function\nxo_nolibc_restorer:\n\tmov x8, #139\n\tsvc #0\n");
#else
__asm__(".text\n.global _start\n.type _start, @function\n_start:\n"
	"\txor %ebp, %ebp\n\tmov %rsp, %rdi\n\tand $-16, %rsp\n\tcall xo_nolibc_start\n\thlt\n"
	".size _start, .-_start\n");
__asm__(".text\n.type xo_nolibc_restorer, @function\nxo_nolibc_restorer:\n\tmov $15, %eax\n\tsyscall\n");
#endif
void xo_nolibc_restorer(void);

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
static int nl_mu;

static void nl_lock(int *f);
static void nl_unlock(int *f);

static void *nl_map(long n) {
	long r = nl_sys6(SYS_mmap, 0, n, PROT_READ | PROT_WRITE, MAP_PRIVATE | MAP_ANONYMOUS, -1, 0);
	if (r < 0 && r > -4096) {
		return NULL;
	}
	return (void *)r;
}

NL_USED void *malloc(size_t n) {
	if (n == 0) {
		n = 1;
	}
	if (n + 16 > (size_t)1 << NL_MAX_SHIFT) {
		long total = (long)((n + 16 + (size_t)nl_pagesz - 1) & ~((size_t)nl_pagesz - 1));
		char *p = nl_map(total);
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
		nl_sys2(SYS_munmap, h, -c);
		return;
	}
	nl_lock(&nl_mu);
	((nl_free *)h)->next = nl_lists[c];
	nl_lists[c] = (nl_free *)h;
	nl_unlock(&nl_mu);
}

NL_USED void *calloc(size_t n, size_t m) {
	size_t t;
	if (__builtin_mul_overflow(n, m, &t)) {
		nl_errno = ENOMEM;
		return NULL;
	}
	void *p = malloc(t);
	if (p) {
		__builtin_memset(p, 0, t);
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
	if (o < i || o >= i + n) {
		while (n--) {
			*o++ = *i++;
		}
		return d;
	}
	o += n;
	i += n;
	while (n--) {
		*--o = *--i;
	}
	return d;
}

NL_USED NL_NOBUILTIN void *memset(void *d, int c, size_t n) {
	unsigned char *o = d;
	while (n--) {
		*o++ = (unsigned char)c;
	}
	return d;
}

NL_USED NL_NOBUILTIN int memcmp(const void *a, const void *b, size_t n) {
	const unsigned char *x = a, *y = b;
	for (size_t i = 0; i < n; i++) {
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

NL_USED int clock_gettime(clockid_t id, struct timespec *ts) { return (int)nl_ret(nl_sys2(SYS_clock_gettime, id, ts)); }

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
// Signals: the kernel's struct sigaction (handler, flags, restorer, mask)
// with this file's rt_sigreturn trampoline.

NL_USED int sigemptyset(sigset_t *s) {
	memset(s, 0, sizeof *s);
	return 0;
}

NL_USED int sigaddset(sigset_t *s, int sig) {
	((unsigned long *)s)[(sig - 1) / 64] |= 1UL << ((sig - 1) % 64);
	return 0;
}

NL_USED int sigaction(int sig, const struct sigaction *restrict act, struct sigaction *restrict old) {
	struct {
		void *handler;
		unsigned long flags;
		void *restorer;
		unsigned long mask;
	} k = {0}, ko = {0};
	if (act) {
		k.handler = (void *)act->sa_handler;
		k.flags = (unsigned long)act->sa_flags | 0x04000000; // SA_RESTORER
		k.restorer = (void *)xo_nolibc_restorer;
		memcpy(&k.mask, &act->sa_mask, sizeof k.mask);
	}
	long r = nl_ret(nl_sys4(SYS_rt_sigaction, sig, act ? &k : NULL, &ko, 8));
	if (r == 0 && old) {
		memset(old, 0, sizeof *old);
		old->sa_handler = (void (*)(int))ko.handler;
		old->sa_flags = (int)ko.flags;
		memcpy(&old->sa_mask, &ko.mask, sizeof ko.mask);
	}
	return (int)r;
}

NL_USED void (*signal(int sig, void (*h)(int)))(int) {
	struct sigaction sa, old;
	memset(&sa, 0, sizeof sa);
	sa.sa_handler = h;
	sa.sa_flags = SA_RESTART;
	if (sigaction(sig, &sa, &old) != 0) {
		return SIG_ERR;
	}
	return old.sa_handler;
}

NL_USED int sigprocmask(int how, const sigset_t *restrict set, sigset_t *restrict old) {
	return (int)nl_ret(nl_sys4(SYS_rt_sigprocmask, how, set, old, 8));
}

NL_USED int pthread_sigmask(int how, const sigset_t *restrict set, sigset_t *restrict old) {
	return sigprocmask(how, set, old) == 0 ? 0 : nl_errno;
}

// ---------------------------------------------------------------------------
// Locks over futex (Drepper's mutex: 0 free, 1 held, 2 held with
// waiters); the first int of glibc's pthread_mutex_t is the word.

static void nl_lock(int *f) {
	int c = 0;
	if (__atomic_compare_exchange_n(f, &c, 1, 0, __ATOMIC_ACQUIRE, __ATOMIC_RELAXED)) {
		return;
	}
	if (c != 2) {
		c = __atomic_exchange_n(f, 2, __ATOMIC_ACQUIRE);
	}
	while (c != 0) {
		nl_sys4(SYS_futex, f, 128 /* FUTEX_WAIT_PRIVATE */, 2, 0);
		c = __atomic_exchange_n(f, 2, __ATOMIC_ACQUIRE);
	}
}

static void nl_unlock(int *f) {
	if (__atomic_exchange_n(f, 0, __ATOMIC_RELEASE) == 2) {
		nl_sys4(SYS_futex, f, 129 /* FUTEX_WAKE_PRIVATE */, 1, 0);
	}
}

NL_USED int pthread_mutex_init(pthread_mutex_t *restrict m, const pthread_mutexattr_t *restrict a) {
	(void)a;
	memset(m, 0, sizeof *m);
	return 0;
}

NL_USED int pthread_mutex_destroy(pthread_mutex_t *m) {
	(void)m;
	return 0;
}

NL_USED int pthread_mutex_lock(pthread_mutex_t *m) {
	nl_lock((int *)m);
	return 0;
}

NL_USED int pthread_mutex_trylock(pthread_mutex_t *m) {
	int c = 0;
	return __atomic_compare_exchange_n((int *)m, &c, 1, 0, __ATOMIC_ACQUIRE, __ATOMIC_RELAXED) ? 0 : EBUSY;
}

NL_USED int pthread_mutex_unlock(pthread_mutex_t *m) {
	nl_unlock((int *)m);
	return 0;
}

NL_USED int pthread_once(pthread_once_t *o, void (*f)(void)) {
	int *w = (int *)o;
	int c = 0;
	if (__atomic_compare_exchange_n(w, &c, 1, 0, __ATOMIC_ACQUIRE, __ATOMIC_ACQUIRE)) {
		f();
		__atomic_store_n(w, 2, __ATOMIC_RELEASE);
		nl_sys4(SYS_futex, w, 129, 0x7fffffff, 0);
		return 0;
	}
	while ((c = __atomic_load_n(w, __ATOMIC_ACQUIRE)) != 2) {
		nl_sys4(SYS_futex, w, 128, c, 0);
	}
	return 0;
}

// Threads: clone with the flags of a POSIX thread, a stack and a TLS block
// in one mapping (with a guard page below the stack), the thread's id
// cleared and woken by the kernel at its exit (CLONE_CHILD_CLEARTID),
// which pthread_join waits for. A detached thread's mapping is not
// reused (the runtime detaches its poller and workers, which live until
// the program ends).

typedef struct {
	size_t stack;
	int detached;
} nl_attr;

_Static_assert(sizeof(nl_attr) <= sizeof(pthread_attr_t), "pthread_attr_t holds nl_attr");

typedef struct nl_thread {
	void *(*f)(void *);
	void *arg;
	void *ret;
	volatile int tid;
	int detached;
	char *map;
	size_t maplen;
} nl_thread;

static nl_thread nl_main_thread;
static __thread nl_thread *nl_self;

NL_USED int pthread_attr_init(pthread_attr_t *a) {
	memset(a, 0, sizeof *a);
	((nl_attr *)a)->stack = 8 << 20;
	return 0;
}

NL_USED int pthread_attr_destroy(pthread_attr_t *a) {
	(void)a;
	return 0;
}

NL_USED int pthread_attr_setstacksize(pthread_attr_t *a, size_t n) {
	((nl_attr *)a)->stack = n;
	return 0;
}

NL_USED int pthread_attr_setdetachstate(pthread_attr_t *a, int s) {
	((nl_attr *)a)->detached = s == PTHREAD_CREATE_DETACHED;
	return 0;
}

NL_USED __attribute__((noreturn)) void xo_nolibc_thread_start(nl_thread *t) {
	nl_self = t;
	t->ret = t->f(t->arg);
	for (;;) {
		nl_sys1(SYS_exit, 0);
	}
}

// nl_clone(flags, stack, parent_tid, tls, child_tid, thread): clone(2);
// the child calls xo_nolibc_thread_start(thread) on its stack.
long nl_clone(long flags, char *stack, volatile int *ptid, char *tls, volatile int *ctid, nl_thread *t);
#if defined(__aarch64__)
__asm__(".text\n.type nl_clone, %function\nnl_clone:\n"
	"\tstr x5, [x1, #-16]!\n" // the thread, on the child's stack
	"\tmov x8, #220\n\tsvc #0\n" // clone(flags, stack, ptid, tls, ctid)
	"\tcbz x0, 1f\n\tret\n"
	"1:\tldr x0, [sp], #16\n\tmov x29, #0\n\tmov x30, #0\n\tbl xo_nolibc_thread_start\n\tbrk #1\n");
#else
__asm__(".text\n.type nl_clone, @function\nnl_clone:\n"
	"\tsub $16, %rsi\n\tmov %r9, (%rsi)\n" // the thread, on the child's stack
	"\tmov %r8, %r10\n\tmov %rcx, %r8\n" // child_tid, tls
	"\tmov $56, %eax\n\tsyscall\n" // clone(flags, stack, ptid, ctid, tls)
	"\ttest %rax, %rax\n\tjz 1f\n\tret\n"
	"1:\txor %ebp, %ebp\n\tpop %rdi\n\tpop %rax\n\tcall xo_nolibc_thread_start\n\thlt\n");
#endif

NL_USED int pthread_create(pthread_t *restrict tp, const pthread_attr_t *restrict a, void *(*f)(void *), void *restrict arg) {
	size_t stack = a ? ((const nl_attr *)a)->stack : (size_t)8 << 20;
	if (stack < (size_t)1 << 16) {
		stack = (size_t)1 << 16;
	}
	size_t page = (size_t)nl_pagesz;
	stack = (stack + page - 1) & ~(page - 1);
	size_t tls = (nl_tls_size() + sizeof(nl_thread) + page - 1) & ~(page - 1);
	size_t len = page + stack + tls;
	long m = nl_sys6(SYS_mmap, 0, (long)len, PROT_READ | PROT_WRITE, MAP_PRIVATE | MAP_ANONYMOUS | MAP_NORESERVE, -1, 0);
	if (m < 0 && m > -4096) {
		return EAGAIN;
	}
	char *map = (char *)m;
	nl_sys3(SYS_mprotect, map, page, PROT_NONE);
	nl_thread *t = (nl_thread *)(map + page + stack);
	t->f = f;
	t->arg = arg;
	t->detached = a ? ((const nl_attr *)a)->detached : 0;
	t->map = map;
	t->maplen = len;
	char *area = (char *)(((uintptr_t)(t + 1) + 63) & ~(uintptr_t)63);
	char *tpp = nl_tls_init(area);
	// CLONE_VM, FS, FILES, SIGHAND, THREAD, SYSVSEM, SETTLS, PARENT_SETTID,
	// CHILD_CLEARTID
	long flags = 0x100 | 0x200 | 0x400 | 0x800 | 0x10000 | 0x40000 | 0x80000 | 0x100000 | 0x200000;
	long r = nl_clone(flags, map + page + stack, &t->tid, tpp, &t->tid, t);
	if (r < 0) {
		nl_sys2(SYS_munmap, map, len);
		return (int)-r;
	}
	*tp = (pthread_t)t;
	return 0;
}

NL_USED int pthread_join(pthread_t th, void **r) {
	nl_thread *t = (nl_thread *)th;
	int tid;
	while ((tid = t->tid) != 0) {
		nl_sys4(SYS_futex, &t->tid, 0 /* FUTEX_WAIT: CLONE_CHILD_CLEARTID wakes it shared */, tid, 0);
	}
	if (r) {
		*r = t->ret;
	}
	nl_sys2(SYS_munmap, t->map, t->maplen);
	return 0;
}

NL_USED int pthread_detach(pthread_t th) {
	((nl_thread *)th)->detached = 1;
	return 0;
}

NL_USED pthread_t pthread_self(void) { return (pthread_t)(nl_self ? nl_self : &nl_main_thread); }

NL_USED int sched_yield(void) { return (int)nl_ret(nl_sys0(SYS_sched_yield)); }

// ---------------------------------------------------------------------------
// Sockets the runtime's event loop touches in every program

NL_USED int shutdown(int fd, int how) { return (int)nl_ret(nl_sys2(SYS_shutdown, fd, how)); }

NL_USED int epoll_create1(int flags) { return (int)nl_ret(nl_sys1(SYS_epoll_create1, flags)); }

NL_USED int epoll_ctl(int ep, int op, int fd, struct epoll_event *ev) { return (int)nl_ret(nl_sys4(SYS_epoll_ctl, ep, op, fd, ev)); }

NL_USED int epoll_wait(int ep, struct epoll_event *evs, int n, int ms) {
	return (int)nl_ret(nl_sys6(SYS_epoll_pwait, ep, (long)evs, n, ms, 0, 8));
}

// ---------------------------------------------------------------------------
// Unwinding is stage 2: the forced unwind of a fault finds no frames, so
// the runtime reports the fault and ends the program (fault_exit).

NL_USED _Unwind_Reason_Code _Unwind_ForcedUnwind(struct _Unwind_Exception *e, _Unwind_Stop_Fn stop, void *arg) {
	(void)e;
	(void)stop;
	(void)arg;
	return _URC_END_OF_STACK;
}

NL_USED _Unwind_Word _Unwind_GetCFA(struct _Unwind_Context *c) {
	(void)c;
	return 0;
}

NL_USED _Unwind_Ptr _Unwind_GetIP(struct _Unwind_Context *c) {
	(void)c;
	return 0;
}

NL_USED void *_Unwind_GetLanguageSpecificData(struct _Unwind_Context *c) {
	(void)c;
	return NULL;
}

NL_USED _Unwind_Ptr _Unwind_GetRegionStart(struct _Unwind_Context *c) {
	(void)c;
	return 0;
}

NL_USED void _Unwind_SetGR(struct _Unwind_Context *c, int i, _Unwind_Word v) {
	(void)c;
	(void)i;
	(void)v;
}

NL_USED void _Unwind_SetIP(struct _Unwind_Context *c, _Unwind_Ptr v) {
	(void)c;
	(void)v;
}

NL_USED void _Unwind_Resume(struct _Unwind_Exception *e) {
	(void)e;
	nl_die("_Unwind_Resume: unwinding is not available");
}

#endif
