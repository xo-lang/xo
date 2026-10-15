// ---------------------------------------------------------------------------
// xo_nolibc.c: what stays C of the C library of Linux programs linked
// statically with no C library (decisions 0203, 0206, and 0230).
// Included at the end of xo_rt.c when it is compiled with -DXO_NOLIBC
// (XO_NOLIBC=1 builds): the runtime keeps its C library calls, and the
// program links with -static -nostdlib --gc-sections (a program that
// reaches a function defined nowhere fails to link, naming it).
//
// The C library is Xo, exported under the C names by
// std/native/llrt/cabi_libc.xo (the zero-C audit's B04): malloc and
// friends (libcmem.xo), the mem* and str* functions, strerror, the strto*
// family, and qsort (libcstr.xo), exit, errno, and the system call
// wrappers (libcsys.xo), the environment, getauxval, FILE, and
// directories (libcio.xo), and signals, locks, and threads
// (cabi_nolibc.xo). strtod ends the program with a message; getaddrinfo
// is not defined (the network is cabi_net.xo's); unwinding is Xo
// (decision 0221, unwind.xo); there is no printf family (no C of such a
// build calls it, decision 0234).
//
// The data symbols environ, stdin, stdout, and stderr are the emitters'
// (decision 0234), and the process entry (_start, the thread pointer, the
// switch to main's 1 GiB stack, the call of main) the backends'
// (selfhost/ssa ir.xo entry_stub_arm64); the C one below is built only
// with XO_NOLIBC_C_START=1, until CI has run the stubs.

// The Xo exports the start calls (std/native/llrt/cabi_nolibc.xo), before
// the main thread has its thread pointer.
int64_t xo_x_nl_start(int64_t sp);
int64_t xo_x_nl_std(int64_t i);
int64_t xo_x_nl_main_tp(void);
int64_t xo_x_nl_main_stack(void);

#ifndef XO_NOLIBC_IMPL
#define XO_NOLIBC_IMPL

// Kept through LTO: the linker's --gc-sections still drops what nothing
// calls.
#define NL_USED __attribute__((used))

// ---------------------------------------------------------------------------
// Process entry

// environ, stdin, stdout, and stderr are the emitters' (decision 0234),
// which cabi_nolibc.xo nl_start sets
extern char **environ;

#ifdef XO_NOLIBC_C_START
// The C process entry, kept selectable (XO_NOLIBC_C_START=1) until CI has
// run the backends' (selfhost/ssa ir.xo entry_stub_arm64 and
// entry_stub_amd64, which do the same with no C), then deleted.

extern int main(int, char **);
static int nl_argc;
static char **nl_argv;

NL_USED __attribute__((noreturn)) static void nl_run(void) { exit(main(nl_argc, nl_argv)); }

// xo_nolibc_start runs from _start with the initial stack: argc, argv,
// envp, the auxiliary vector (Xo records them, maps the TLS block, and
// makes the standard streams), then runs main on a 1 GiB stack.
NL_USED __attribute__((noreturn)) void xo_nolibc_start(long *sp) {
	nl_argc = (int)sp[0];
	nl_argv = (char **)(sp + 1);
	environ = (char **)(intptr_t)xo_x_nl_start((int64_t)(intptr_t)sp);
	stdin = (FILE *)(intptr_t)xo_x_nl_std(0);
	stdout = (FILE *)(intptr_t)xo_x_nl_std(1);
	stderr = (FILE *)(intptr_t)xo_x_nl_std(2);
	char *tp = (char *)(intptr_t)xo_x_nl_main_tp();
#if defined(__aarch64__)
	__asm__ volatile("msr tpidr_el0, %0" ::"r"(tp));
#else
	(void)tp; // set by arch_prctl in Xo
#endif
	char *top = (char *)(intptr_t)xo_x_nl_main_stack();
	if (!top) {
		nl_run();
	}
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
#elif defined(__x86_64__)
__asm__(".text\n.global _start\n.type _start, @function\n_start:\n"
	"\txor %ebp, %ebp\n\tmov %rsp, %rdi\n\tand $-16, %rsp\n\tcall xo_nolibc_start\n\thlt\n"
	".size _start, .-_start\n");
#else
#error "XO_NOLIBC: arm64 or x86_64 Linux only"
#endif
#endif // XO_NOLIBC_C_START

#endif
