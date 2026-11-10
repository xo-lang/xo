// use_c_stdio's C side: printf's family (snprintf, sprintf, fprintf) and
// <ctype.h>, which a FreeBSD program gets from Xo's generated C (decision
// 0573 `use c` stage 3) and elsewhere from the system's C library.
#include <ctype.h>
#include <stdio.h>
#include <string.h>
#include "libst.h"

static char buf[256];

const char *libst_fmt(int64_t k) {
  switch (k) {
  case 0: snprintf(buf, sizeof buf, "%d|%5d|%-5d|%05d|%+d|% d", 42, -7, 3, 12, 9, 4); break;
  case 1: snprintf(buf, sizeof buf, "%x|%X|%#x|%o|%#o|%u", 255u, 3054u, 255u, 8u, 8u, 4000000000u); break;
  case 2: snprintf(buf, sizeof buf, "%ld|%lld|%zu|%hd|%hhd", -1234567890123L, 9223372036854775807LL, (size_t)77, (short)70000, (signed char)300); break;
  case 3: snprintf(buf, sizeof buf, "[%s]|[%8s]|[%-8s]|[%.3s]|[%c]|[%%]", "abc", "right", "left", "truncate", 'Z'); break;
  case 4: snprintf(buf, sizeof buf, "%.2f|%8.3f|%-8.1f|%f|%.0f|%+.1f", 3.14159, -2.5, 1.25, 0.1, 2.5, 7.0); break;
  case 5: snprintf(buf, sizeof buf, "%*d|%-*d|%.*s", 6, 1, 4, 2, 2, "xyz"); break;
  case 6: {
    char small[8];
    int n = snprintf(small, sizeof small, "%s", "overflowing");
    snprintf(buf, sizeof buf, "n=%d small=%s", n, small);
    break;
  }
  case 7: {
    char t[64];
    int n = sprintf(t, "%d-%s", 5, "five");
    snprintf(buf, sizeof buf, "%s len %d", t, n);
    break;
  }
  default: buf[0] = 0;
  }
  return buf;
}

const char *libst_ctype(void) {
  const char *s = "Hello, World 42!\t";
  size_t n = strlen(s), j = 0;
  for (size_t i = 0; i < n && j + 8 < sizeof buf; i++) {
    int c = (unsigned char)s[i];
    buf[j++] = (char)toupper(c);
    buf[j++] = isalpha(c) ? 'a' : isdigit(c) ? 'd' : isspace(c) ? 's' : ispunct(c) ? 'p' : '?';
  }
  buf[j] = 0;
  return buf;
}

int64_t libst_eprint(int64_t n) {
  return fprintf(stderr, "eprint %lld %s\n", (long long)n, "done");
}
