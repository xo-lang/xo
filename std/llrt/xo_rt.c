// xo_rt.c: the C left of the native runtime (the runtime is Xo:
// std/native/llrt, decisions 0206, 0233 and 0234; main is cabi_c.xo's).
// What stays is C by design: Windows' stack overflow handler, which runs
// on what is left of the overflowed stack, and the services record that
// hands it to stop.xo. The Windows constants Xo hard codes are checked by
// std/llrt/testdata/win_consts.c (internal/llvm TestWinConsts), the C
// tests' setup is testdata/xcalls.h's, and the import libraries are the
// driver's (selfhost/xo link.xo sys_link_args). It is compiled with the
// generated code (clang, -flto on LLVM).

#ifdef _WIN32
#ifndef WIN32_LEAN_AND_MEAN
#define WIN32_LEAN_AND_MEAN
#endif
#include <windows.h>
#endif
#include <stdint.h>

// Output is stdio.xo's; the stack overflow handler flushes it.
void xo_x_flush_try(void);

// an address as the int64_t the C ABI exports (cabi.xo) take
#define XO_ADDR(p) ((int64_t)(intptr_t)(p))

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

// A Linux program linked with no C library (XO_NOLIBC=1 builds, decisions
// 0203, 0206, 0230 and 0234) has no C of its own beyond this file: its C
// library is Xo (std/native/llrt/cabi_libc.xo), its data symbols and
// process entry the backends' (selfhost/ssa ir.xo entry_stub_arm64);
// std/llrt/xo_nolibc.c was deleted on 2026-10-15.
