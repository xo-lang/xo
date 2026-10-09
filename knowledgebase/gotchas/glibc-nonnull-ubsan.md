# glibc marks memcpy and qsort nonnull: UBSan flags zero length calls

Status: Observed (2026-10-09, first Linux run of the AddressSanitizer and
UBSan tests)

Symptom: TestGoldenASan (http_client, http_server, http_wire),
TestHTTPParseDiff, TestHTTPFuzz, and FuzzHTTPDiff failed on Linux with
`runtime error: null pointer passed as argument 2, which is declared to
never be null` (memcpy, qsort) and `applying zero offset to null
pointer` (`buf + off` with buf NULL). They pass on macOS.

Cause: glibc's `string.h` and `stdlib.h` declare `memcpy` and `qsort`
with `__attribute__((nonnull))`, so UBSan's `nonnull-attribute` check
fires for `memcpy(dst, NULL, 0)`. Apple's headers do not carry the
attribute. Pointer arithmetic on NULL (`NULL + 0`) is UB in C and clang
19's `pointer-overflow` check reports it; the buffers in xo_http.c and
`nbuf` start NULL and grow lazily.

Fix: guard zero length copies and sorts (`if (n) memcpy(...)`), return
early from `nbuf_put` when n is 0, and write `buf ? buf + off : buf` where
a lazily allocated buffer may still be NULL. Run the sanitizer tests on
Linux (`scripts/linux-test.sh`) after touching std/llrt.
