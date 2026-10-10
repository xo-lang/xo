// A tiny C library for the `use c` tests (decision 0113).
#ifndef TINY_H
#define TINY_H

#include <stddef.h>
#include <stdint.h>

typedef struct tiny_counter tiny_counter;

struct tiny_point {
  int x;
  int y;
};

struct tiny_pair {
  double a;
  float b;
  _Bool ok;
};

enum tiny_mode { TINY_UP, TINY_DOWN };

int tiny_add(int a, int b);
double tiny_scale(double x, float f);
const char *tiny_greeting(void);
size_t tiny_count(const char *s);
uint32_t tiny_sum(const unsigned char *data, size_t len);
int64_t tiny_text_len(const char *text, size_t len);
const char *tiny_echo(const char *s);
tiny_counter *tiny_counter_new(int64_t start);
void tiny_counter_inc(tiny_counter *c);
long tiny_counter_get(const tiny_counter *c);
void tiny_counter_free(tiny_counter *c);
tiny_counter *tiny_counter_none(void);
_Bool tiny_is_even(long n);
uint64_t tiny_big(void);
int8_t tiny_neg8(void);
uint8_t tiny_byte(uint8_t b);
int tiny_step(enum tiny_mode m, int v);
struct tiny_point tiny_point_add(struct tiny_point a, struct tiny_point b);
struct tiny_pair tiny_pair_make(double a, float b);

// skipped: an output parameter, a variadic function, a mutable buffer,
// a char * result, a pointer to a defined struct
void tiny_out(int *out);
int tiny_printf(const char *fmt, ...);
void tiny_fill(char *buf, size_t n);
char *tiny_dup(const char *s);
void tiny_move(struct tiny_point *p);

#endif
