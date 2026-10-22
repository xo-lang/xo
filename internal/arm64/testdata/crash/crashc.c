// TestCrashReport's C side (decision 0343).
#include "crashc.h"

int64_t crashc_write(int64_t a) {
  volatile int64_t *p = (volatile int64_t *)(intptr_t)a;
  *p = 42;
  return *p;
}

int64_t crashc_trap(int64_t x) {
  if (x >= 0) {
    __builtin_trap();
  }
  return x;
}

int64_t crashc_deep(int64_t n) {
  volatile char buf[4096];
  buf[0] = (char)n;
  buf[4095] = (char)n;
  return crashc_deep(n + 1) + buf[0] + buf[4095];
}
