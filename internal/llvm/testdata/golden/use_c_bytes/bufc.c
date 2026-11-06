#include "bufc.h"

static const unsigned char nul_text[] = {0x00, 'a', 0x00, 'b'};
static const unsigned char high[] = {0xff, 0x00, 0xfe};

bufc_buf bufc_get(int64_t which) {
  bufc_buf b = {0, 0};
  if (which == 0) {
    b.p = nul_text;
    b.n = sizeof nul_text;
  } else if (which == 1) {
    b.p = high;
    b.n = sizeof high;
  }
  /* any other: NULL, which binds as empty Bytes */
  return b;
}

int64_t bufc_sum(const void *data, int64_t len) {
  const unsigned char *d = data;
  int64_t s = 0;
  for (int64_t i = 0; i < len; i++) {
    s += d[i];
  }
  return s;
}
