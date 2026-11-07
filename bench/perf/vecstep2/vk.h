// bench/perf/vecstep2's C kernel (ADR 0532 step 2's measurement): the loop
// of std's count_cont in NEON by hand (arm64; plain C elsewhere); a bound
// on what step 2's emitted kernel could reach.
#ifndef VK_H
#define VK_H

#include <stdint.h>

// The bytes 0x80 to 0xBF among the n bytes at p (arm64: NEON by hand).
int64_t vk_cont(const uint8_t *p, int64_t n);
// The boundary alone: the same parameters, no loop.
int64_t vk_none(const uint8_t *p, int64_t n);

#endif
