# memcpy and qsort with a NULL pointer and a zero count are undefined

Status: Observed (2026-10-12, Linux, glibc, clang 15 UBSan), fixed in
std/llrt/xo_h2.c (h2_head).

Symptom: on Linux, TestHTTPFuzz and TestH2Sanitizers/address (built with
`-fsanitize=address,undefined -fno-sanitize-recover=undefined`) stopped
with `xo_h2.c:1946:12: runtime error: null pointer passed as argument 2,
which is declared to never be null`; the HTTP/2 test then saw `program
ended before serving: EOF`. The Mac passes.

Cause: h2_head copied the handler's header list with
`memcpy(s, h->a, h->n * sizeof(hpair))` and sorted it with qsort. A
handler that set no field has `h->a == NULL` and `h->n == 0`. C makes a
NULL argument undefined even for a zero count, and glibc's string.h and
stdlib.h declare the pointers nonnull, so UBSan checks it there; the
macOS headers do not, so the Mac never reported it.

Fix: copy and sort only when `h->n > 0`. In C runtime code, guard every
memcpy, memmove, memcmp, and qsort whose pointer can be NULL with a zero
length (an empty list or string with no buffer).
