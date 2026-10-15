// fault_shared.c: a fault raised in a qsort comparator, so that frames of
// the C library (a shared object: libc.so.6 on Linux, libsystem_c in the
// dyld shared cache on macOS) sit between the fault and the frame that
// catches it. Run by internal/llvm's TestRuntimeFaultShared with
// fault_shared.ll, whose catcher has a landing pad under
// xo_x_personality. The Xo unwinder (decision 0221) finds the C
// library's unwind tables through ld.so's link_map on Linux and through
// dladdr on macOS; without them the walk would end at qsort's frame and
// the fault would be reported instead of caught.
//
// Expected: "caught" on stderr and exit status 0.

#define XO_RT_NO_MAIN
#include "../xo_rt.c"
#include "xcalls.h"

static int cmp(const void *a, const void *b) {
	(void)a;
	(void)b;
	static const char msg[] = "boom in qsort";
	static const char loc[] = "fault_shared.xo:1:1";
	t_fault_raise(msg, (int64_t)sizeof msg - 1, loc, (int64_t)sizeof loc - 1);
}

// call_qsort sorts through the C library's qsort, which calls cmp.
void call_qsort(void) {
	int v[4] = {3, 1, 2, 0};
	qsort(v, 4, sizeof v[0], cmp);
}

// caught runs in catcher's landing pad.
void caught(void) {
	fputs("caught\n", stderr);
	exit(0);
}

void catcher(void);

int main(void) {
	// the fault is reported once it unwinds past this frame
	t_ctx_boundary((uintptr_t)__builtin_frame_address(0));
	catcher();
	fputs("unreachable: catcher returned\n", stderr);
	return 1;
}
