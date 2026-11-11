#include <stdlib.h>
#include <string.h>

#include "nmc.h"

struct nmc_box {
  int64_t r;
};

static char str_buf[64];
static const char data[] = "hello";

int64_t nmc_scalar(int64_t r, int64_t c, int64_t v, int64_t n, int64_t xo_len) {
  return r * 10000 + c * 1000 + v * 100 + n * 10 + xo_len;
}

int64_t nmc_self(int64_t nmc_self) { return nmc_self + 1; }

const char *nmc_str(const char *c, int64_t r) {
  size_t l = strlen(c);
  if (l > 60) {
    l = 60;
  }
  memcpy(str_buf, c, l);
  str_buf[l] = (char)('0' + r);
  str_buf[l + 1] = 0;
  return str_buf;
}

struct nmc_buf nmc_bytes(int64_t n, int64_t v) {
  struct nmc_buf b = {data + v, n < 5 - v ? n : 5 - v};
  return b;
}

int64_t nmc_sum(const void *v, int64_t n) {
  const unsigned char *d = v;
  int64_t s = 0;
  for (int64_t i = 0; i < n; i++) {
    s += d[i];
  }
  return s;
}

struct nmc_box *nmc_open(int64_t r) {
  struct nmc_box *b = malloc(sizeof *b);
  if (b) {
    b->r = r;
  }
  return b;
}

int64_t nmc_peek(struct nmc_box *c, int64_t v) { return c->r + v; }

void nmc_close(struct nmc_box *r) { free(r); }

struct nmc_pair nmc_swap(struct nmc_pair v) {
  struct nmc_pair o = {v.n, v.r};
  return o;
}
