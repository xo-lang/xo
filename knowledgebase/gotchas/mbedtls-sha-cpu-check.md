# Mbed TLS's SHA instruction check: a macOS compile error and a data race

Status: Observed (2026-10-10, decision 0117) Obsolete since 2026-10-15: Mbed TLS removed (decision 0202).

Symptom 1: with `MBEDTLS_SHA512_USE_A64_CRYPTO_IF_PRESENT` on, the TLS
archive failed to build on macOS: `sha512.c` includes `<sys/sysctl.h>`,
which stopped at `unknown type name 'u_int'` in `<sys/proc.h>`.

Cause 1: the library compiles with `_POSIX_C_SOURCE` set, which hides the
BSD types the macOS header needs; the run time check of the SHA-512
instructions (`sysctlbyname("hw.optional.armv8_2_sha512")`) is the only
code that includes it.

Fix 1: `mbedtls.FileDefines` compiles `sha512.c` alone with
`-D_DARWIN_C_SOURCE` (no effect on Linux), so the vendored file stays
unmodified.

Symptom 2: the `http_tls` golden under ThreadSanitizer (TestGoldenThreads)
aborted with a data race in `mbedtls_sha512_update`, two client
handshakes on two threads.

Cause 2: `mbedtls_a64_crypto_sha512_has_support` (and the SHA-256 one)
keep the answer of the CPU check in two function statics written without
a lock on first use.

Fix 2: `tls_init_once` in `std/llrt/xo_tls.c` (a `pthread_once` every TLS
entry point passes first) hashes once with SHA-256 and SHA-512, so every
later call only reads the statics. Any other `_IF_PRESENT` option turned
on later needs the same warm up.
