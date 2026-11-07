#include "vk.h"

#if defined(__aarch64__)
#include <arm_neon.h>

// NEON by hand (independent of whether cc vectorizes at -O2: gcc's cost
// model at -O2 does not for an unknown trip count): 16 bytes a step, each
// lane's continuation test (b & 0xC0 == 0x80) subtracted as -1 into a
// byte counter, widened every 255 steps before it can wrap.
int64_t vk_cont(const uint8_t *p, int64_t n) {
  int64_t c = 0;
  int64_t i = 0;
  const uint8x16_t m = vdupq_n_u8(0xC0), k = vdupq_n_u8(0x80);
  while (n - i >= 16) {
    uint8x16_t acc = vdupq_n_u8(0);
    for (int r = 0; r < 255 && n - i >= 16; r++, i += 16) {
      uint8x16_t v = vld1q_u8(p + i);
      acc = vsubq_u8(acc, vceqq_u8(vandq_u8(v, m), k));
    }
    c += vaddlvq_u8(acc);
  }
  for (; i < n; i++) {
    c += (p[i] & 0xC0) == 0x80;
  }
  return c;
}
#else
int64_t vk_cont(const uint8_t *p, int64_t n) {
  int64_t c = 0;
  for (int64_t i = 0; i < n; i++) {
    c += (p[i] & 0xC0) == 0x80;
  }
  return c;
}
#endif

int64_t vk_none(const uint8_t *p, int64_t n) {
  return n > 0 ? p[0] & 1 : 0;
}
