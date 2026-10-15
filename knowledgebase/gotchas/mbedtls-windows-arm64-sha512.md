# Mbed TLS's SHA-512 detection does not compile on Windows on Arm

Status: Observed (2026-10-12), fixed in third_party/mbedtls/xo/xo_crypto_config.h. Obsolete since 2026-10-15: Mbed TLS removed (decision 0202).

Symptom: cross building any std/http golden for windows/arm64 failed in
the TLS library: `sha512.c: call to undeclared function
'mbedtls_a64_crypto_sha512_determine_support'`.

Cause: with `MBEDTLS_SHA512_USE_A64_CRYPTO_IF_PRESENT`, sha512.c defines
the run time CPU check for Linux (getauxval), macOS (sysctl), and Unix
(SIGILL); its Windows on Arm branch only prints a warning and, unlike
the fallback branch, does not undefine the option, so the call site
names a function that does not exist (Mbed TLS 4.1.1).

Fix: the Xo configuration defines the option except on Windows on Arm
(`_WIN32` with `__aarch64__` or `_M_ARM64`), where SHA-512 is the
portable C. A detection through `IsProcessorFeaturePresent` once
Windows reports SHA-512 (no PF_ flag for it as of 2026) would bring the
fast path back.

Related: Everest's Curve25519 on any Windows target takes kremlib's MSVC
branch for its 128-bit integers (`__m128i`), which clang rejects; the
library build passes `-U__SIZEOF_INT128__` for that file on Windows
(internal/build tls.go, tlsFileDefines).
