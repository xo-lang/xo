# Mbed TLS fills its cipher suite list lazily without a lock

Status: Observed (2026-10-09, decision 0105)

Symptom: the `http_tls` golden under ThreadSanitizer with four `main`s at
once (TestGoldenThreads, `XO_RT_TEST_THREADS=4`) aborted with a data race
in `mbedtls_ssl_list_ciphersuites`, called by `mbedtls_ssl_config_defaults`
from two threads making a client (`Client.with_roots`) at the same time.

Cause: Mbed TLS 4.1 builds the global `supported_ciphersuites` array the
first time it is asked for it, with a plain `if (!supported_init)` and no
lock. Its threading support (`MBEDTLS_THREADING_C`) covers the PSA key
store and the random generator, not this.

Fix: `std/llrt/xo_tls.c` calls `mbedtls_ssl_list_ciphersuites()` inside
`tls_init_once` (a `pthread_once` every TLS entry point passes first), so
the array is filled before any configuration is made. When updating Mbed
TLS, run the TLS goldens under ThreadSanitizer (`go test ./internal/llvm
-run 'TestGolden(Threads|Tasks)/http_tls'`): another lazily filled global
would show the same way. The archive is built with the sanitizer of the
link (internal/build `tlsFlavor`), so races inside the library are seen.
