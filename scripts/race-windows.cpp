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
}
