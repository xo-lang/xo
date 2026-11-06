// use_c_bytes's C side: a buffer returned by value as a pointer and a
// length (spec 4.5: a struct of a const byte pointer then an integer binds
// as a Bytes result), holding NUL bytes a `const char *` result would cut.
#ifndef BUFC_H
#define BUFC_H

#include <stdint.h>

typedef struct bufc_buf {
  const void *p;
  int64_t n;
} bufc_buf;

bufc_buf bufc_get(int64_t which);
int64_t bufc_sum(const void *data, int64_t len);

#endif
