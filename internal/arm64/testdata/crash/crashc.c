// TestCrashReport's C side (decision 0343). The functions are noinline
// but crashc_write_inl: the test checks the frame walker, and the Linux
// LLVM link's full LTO inlined crashc_write into poke, so the report
// named poke alone (gotchas/linux-lto-inlines-c-frames.md, 2026-10-30);
// crashc_write_inl is left to be inlined, and the report shows it from
// the C unit's debug information (decision 0446).
#include "crashc.h"

#define XO_NOINLINE __attribute__((noinline))

XO_NOINLINE int64_t crashc_write(int64_t a) {
  volatile int64_t *p = (volatile int64_t *)(intptr_t)a;
  *p = 42;
  return *p;
}

XO_NOINLINE int64_t crashc_trap(int64_t x) {
  if (x >= 0) {
    __builtin_trap();
  }
  return x;
}

XO_NOINLINE int64_t crashc_deep(int64_t n) {
  volatile char buf[4096];
  buf[0] = (char)n;
  buf[4095] = (char)n;
  return crashc_deep(n + 1) + buf[0] + buf[4095];
}

int64_t crashc_write_inl(int64_t a) {
  volatile int64_t *p = (volatile int64_t *)(intptr_t)a;
  *p = 43;
  return *p;
}
