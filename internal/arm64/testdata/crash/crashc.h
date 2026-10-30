// TestCrashReport's C side (decision 0343): the hardware faults Xo code
// cannot raise itself.
#ifndef CRASHC_H
#define CRASHC_H

#include <stdint.h>

// Writes through the address a (0: a null pointer).
int64_t crashc_write(int64_t a);
// Runs the target's trap instruction (arm64 brk, x86 ud2).
int64_t crashc_trap(int64_t x);
// Recurses with a large frame until the stack overflows.
int64_t crashc_deep(int64_t n);
// crashc_write without noinline: a Linux LLVM link's LTO inlines it into
// its Xo caller, and the report shows it as an inlined frame from the C
// unit's debug information (decision 0446).
int64_t crashc_write_inl(int64_t a);

#endif
