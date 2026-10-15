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
// What is here and what is not (decisions 0206 and 0230):
//   - process entry (_start: arguments, environment, the auxiliary vector,
//     the thread pointer and the static TLS block, a 1 GiB stack for main
//     as the runtime's main thread has), getauxval, getenv;
//   - FILE and a printf family without floating point conversions, and
//     directories (opendir, readdir);
//   - Xo, exported under the C names by std/native/llrt/cabi_libc.xo (the
//     zero-C audit's B04): malloc and friends (libcmem.xo), the mem* and
//     str* functions, strerror, the strto* family, and qsort (libcstr.xo),
//     exit, errno, the descriptor, file, mapping, clock, randomness, and
//     process calls (libcsys.xo), and signals, locks, and threads
//     (cabi_nolibc.xo);
//   - unwinding is Xo (decision 0221, std/native/llrt/unwind.xo): no
//     _Unwind_* function is defined or called;
//   - not here: floating point printf conversions and strtod (they end
//     the program with a message); sockets past shutdown and DNS
//     (getaddrinfo is not defined: the network is cabi_net.xo's).
// The system call layer of Xo code is std/native/llrt/linux.xo; this
// file serves the C runtime only, and shrinks as the runtime moves to Xo.

// The Xo exports this file calls (std/native/llrt/cabi_nolibc.xo): the
// TLS block's layout, before the main thread has its thread pointer.
int64_t xo_x_tls_size(void);
int64_t xo_x_tls_init(int64_t area);

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

// errno is Xo's (std/native/llrt/libcsys.xo: thread local word 10,
// __errno_location exported by cabi_libc.xo).
int *__errno_location(void);
#define nl_errno (*__errno_location())

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

// _exit, exit, and abort are Xo (libcsys.xo through cabi_libc.xo).

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
// Memory: malloc, free, calloc, and realloc are Xo
// (std/native/llrt/libcmem.xo, exported under these names by
// cabi_libc.xo, decision 0230).

// ---------------------------------------------------------------------------
// Strings and memory are Xo (std/native/llrt/libcstr.xo through
// cabi_libc.xo under the C names, the zero-C audit's B04 stage (b),
// decision 0230): memcpy, memmove, memset, memcmp, bcmp, memchr, the str*
// functions, strerror, the strto* family, atoi, atol, atoll, and qsort.

// strtod (floating point text in C) is not available
NL_USED double strtod(const char *s, char **end) {
	(void)s;
	(void)end;
	nl_die("strtod (floating point text in C) is not available");
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
// The environment (the descriptor, file, mapping, clock, randomness,
// process, signal, lock, and thread functions are Xo: libcsys.xo and
// cabi_nolibc.xo through cabi_libc.xo, the zero-C audit's B04 stage (d))

NL_USED char *getenv(const char *name) {
	size_t n = strlen(name);
	for (char **e = environ; e && *e; e++) {
		if (strncmp(*e, name, n) == 0 && (*e)[n] == '=') {
			return *e + n + 1;
		}
	}
	return NULL;
}

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
