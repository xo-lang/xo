#include <stdlib.h>
#include <string.h>
#include "libcc.h"

/* C library calls a binding commonly makes: on FreeBSD, where an Xo
   program links no C library, Xo's runtime defines them (decision 0573). */
int64_t libcc_strings(const char *s) {
  size_t n = strlen(s);
  int64_t r = (strchr(s, 'l') - s) * 1000000;
  r += (strrchr(s, 'l') - s) * 100000;
  r += (strncmp(s, "help", 3) == 0) * 10000;
  r += (int64_t)strspn(s, "hel") * 1000;
  r += (int64_t)strcspn(s, "o") * 100;
  r += ((const char *)memchr(s, 'o', n) - s) * 10;
  r += (strcmp(s, "hello") == 0) + (strcmp("a", "b") < 0) + (strcmp("b", "a") > 0);
  return r;
}

int64_t libcc_heap(int64_t n) {
  int64_t *p = calloc((size_t)n, sizeof *p);
  int64_t sum = 0;
  for (int64_t i = 0; i < n; i++) {
    sum += p[i];
    p[i] = i;
  }
  p = realloc(p, (size_t)(2 * n) * sizeof *p);
  for (int64_t i = n; i < 2 * n; i++) {
    p[i] = i;
  }
  for (int64_t i = 0; i < 2 * n; i++) {
    sum += p[i];
  }
  free(p);
  return sum;
}

int64_t libcc_strtol(const char *s, int64_t base) { return (int64_t)strtol(s, NULL, (int)base); }

int64_t libcc_strtol_used(const char *s, int64_t base) {
  char *end;
  strtol(s, &end, (int)base);
  return end - s;
}

int64_t libcc_atoi(const char *s) { return atoi(s); }
