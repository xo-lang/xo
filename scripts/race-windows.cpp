// Xo's additions to ThreadSanitizer's Go flavor on windows/amd64
// (decision 0273's windows addendum; scripts/race-runtime.sh appends it
// to buildgo.sh's sources): what the sources leave to a C runtime or
// leave undefined with SANITIZER_GO, so the object links with Xo's PE
// linker and runtime and no C library.
#include <windows.h>

namespace __sanitizer {
// sanitizer_win.cpp defines it only without SANITIZER_GO; the mmap
// failure reports still call it
void DumpProcessMap() {}
}  // namespace __sanitizer

extern "C" {
// gcc's __builtin_popcountll without -mpopcnt calls libgcc's helper
int __popcountdi2(unsigned long long a) {
  a = a - ((a >> 1) & 0x5555555555555555ULL);
  a = (a & 0x3333333333333333ULL) + ((a >> 2) & 0x3333333333333333ULL);
  a = (a + (a >> 4)) & 0x0f0f0f0f0f0f0f0fULL;
  return (int)((a * 0x0101010101010101ULL) >> 56);
}

// the C runtime's, built with -D_CRTIMP= so they are plain references:
// errno of the one thread that reports, and the end of the process
static int xo_race_errno;
int *_errno(void) { return &xo_race_errno; }
void exit(int code) { ExitProcess((UINT)code); }

// the Go flavor's __tsan_acquire and __tsan_release take the task's
// ThreadState, so Xo's runtime, whose std/native/sys_c.xo declares the C
// flavor's one argument forms under those names, calls these
// (sanhook.xo); the release a merge, as the C flavor's __tsan_release
// (the Go flavor's __tsan_release is a store)
void __tsan_acquire(void *thr, void *addr);
void __tsan_release_merge(void *thr, void *addr);
void xo_tsan_go_acquire(void *thr, void *addr) { __tsan_acquire(thr, addr); }
void xo_tsan_go_release(void *thr, void *addr) { __tsan_release_merge(thr, addr); }

// __tsan_init with std/native/llrt/cabi_race.xo's callback, an export
// only a --race build has, so the runtime names it only through here
void __tsan_init(void **thrp, void **procp, void (*cb)(unsigned long long, void *));
void xo_x_race_cb(unsigned long long cmd, void *ctx);
void xo_tsan_go_init(void **thrp, void **procp) { __tsan_init(thrp, procp, xo_x_race_cb); }
}
