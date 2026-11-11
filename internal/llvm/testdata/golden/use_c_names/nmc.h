// use_c_names's C side: every adapter shape (the result kinds scalar,
// Str, Bytes, handle, struct and none; the parameter kinds scalar, Str,
// byte buffer, handle and struct) with C parameters named like the
// locals the binder's adapters declared (r, c, v, n, xo_len) and one named
// like its own function.
#ifndef NMC_H
#define NMC_H

#include <stdint.h>

struct nmc_pair {
  int64_t r;
  int64_t n;
};

struct nmc_buf {
  const void *p;
  int64_t n;
};

struct nmc_box;

int64_t nmc_scalar(int64_t r, int64_t c, int64_t v, int64_t n, int64_t xo_len);
int64_t nmc_self(int64_t nmc_self);
const char *nmc_str(const char *c, int64_t r);
struct nmc_buf nmc_bytes(int64_t n, int64_t v);
int64_t nmc_sum(const void *v, int64_t n);
struct nmc_box *nmc_open(int64_t r);
int64_t nmc_peek(struct nmc_box *c, int64_t v);
void nmc_close(struct nmc_box *r);
struct nmc_pair nmc_swap(struct nmc_pair v);

#endif
