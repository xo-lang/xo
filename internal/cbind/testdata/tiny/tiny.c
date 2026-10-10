// A tiny C library for the `use c` tests (decision 0113).
#include "tiny.h"

#include <stdlib.h>
#include <string.h>

struct tiny_counter {
  int64_t n;
};

int tiny_add(int a, int b) { return a + b; }

double tiny_scale(double x, float f) { return x * (double)f; }

const char *tiny_greeting(void) { return "hello from C"; }

size_t tiny_count(const char *s) { return strlen(s); }

uint32_t tiny_sum(const unsigned char *data, size_t len) {
  uint32_t s = 0;
  for (size_t i = 0; i < len; i++) {
    s += data[i];
  }
  return s;
}

int64_t tiny_text_len(const char *text, size_t len) {
  (void)text;
  return (int64_t)len;
}

const char *tiny_echo(const char *s) { return s; }

tiny_counter *tiny_counter_new(int64_t start) {
  tiny_counter *c = malloc(sizeof *c);
  c->n = start;
  return c;
}

void tiny_counter_inc(tiny_counter *c) { c->n++; }

long tiny_counter_get(const tiny_counter *c) { return (long)c->n; }

void tiny_counter_free(tiny_counter *c) { free(c); }

tiny_counter *tiny_counter_none(void) { return NULL; }

_Bool tiny_is_even(long n) { return n % 2 == 0; }

uint64_t tiny_big(void) { return 18446744073709551615ull; }

int8_t tiny_neg8(void) { return -5; }

uint8_t tiny_byte(uint8_t b) { return (uint8_t)(b + 1); }

int tiny_step(enum tiny_mode m, int v) { return m == TINY_UP ? v + 1 : v - 1; }

struct tiny_point tiny_point_add(struct tiny_point a, struct tiny_point b) {
  struct tiny_point r = {a.x + b.x, a.y + b.y};
  return r;
}

struct tiny_pair tiny_pair_make(double a, float b) {
  struct tiny_pair p = {a, b, a > 0};
  return p;
}

void tiny_out(int *out) { *out = 1; }

int tiny_printf(const char *fmt, ...) { (void)fmt; return 0; }

void tiny_fill(char *buf, size_t n) { memset(buf, 'x', n); }

char *tiny_dup(const char *s) { return strdup(s); }

void tiny_move(struct tiny_point *p) { p->x++; }
