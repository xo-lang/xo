// fault_resume.c: a fault unwinding through a C frame whose cleanup
// resumes the unwind (_Unwind_Resume), run by internal/llvm's
// TestRuntimeFaultResume. ThreadSanitizer gives every function such a
// cleanup (__tsan_func_exit on unwind); on Linux CI (2026-10-09) the
// resume read the fault's exception object from fault_raise's frame after
// that frame was gone, and jumped to a clobbered stop function
// (gotchas/tsan-resumes-fault-unwind.md). Here the cleanup itself fills
// the dead frames' stack, so the bug is deterministic without a sanitizer.
//
// Expected: "cleanup ran" on stderr where libgcc or libunwind unwinds
// (x86_64; on arm64 Linux and macOS the Xo unwinder of decision 0221
// passes the C frame over and the cleanup does not run), then the
// runtime's report of the fault past the boundary ("fault: boom at
// fault_resume.xo:3:5") and exit status 70.

#include "../xo_rt.c"
#include "xcalls.h"

static void scrub(void) {
	// runs on the stack below the cleanup's frame: where the frames of
	// xo_fault_raise and fault_raise were
	volatile char junk[16384];
	for (size_t i = 0; i < sizeof junk; i++) {
		junk[i] = (char)0xa5;
	}
}

static void note(int *p) {
	(void)p;
	scrub();
	fputs("cleanup ran\n", stderr);
}

__attribute__((noinline)) static void middle(void) {
	int guard __attribute__((cleanup(note))) = 0;
	(void)guard;
	static const char msg[] = "boom";
	static const char loc[] = "fault_resume.xo:3:5";
	t_fault_raise(msg, (int64_t)sizeof msg - 1, loc, (int64_t)sizeof loc - 1);
}

__attribute__((noinline)) static void outer(void) {
	// the fault is reported once it unwinds past this frame
	t_ctx_boundary((uintptr_t)__builtin_frame_address(0));
	middle();
	fputs("unreachable: middle returned\n", stderr);
}

int main(void) {
	outer();
	return 1;
}
