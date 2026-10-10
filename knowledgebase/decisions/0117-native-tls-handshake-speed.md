# 0117: Faster native TLS handshakes, session tickets on

Status: Implemented (2026-10-10). Amends 0105 (configuration: session
tickets now on; handshake costs). Amended by 0132 (2026-10-10): the
P-256 signer of the consequences is built (fiat-crypto, vendored with
the approval below; `xo/xo_p256.c` interposes
`mbedtls_psa_ecdsa_sign_hash` and `mbedtls_psa_ecdsa_verify_hash`),
signing 110 to 23 us, full handshake 354 to 251 us of CPU. Code: `third_party/mbedtls/xo/`
(`xo_tls_accel.c`, `xo_crypto_config.h`, `xo_tls_config.h`),
`third_party/mbedtls/mbedtls.go` (`FileDefines`, `xo` in `SourceDirs`),
`internal/build/tls.go`, `std/llrt/xo_tls.c`. Tests: TestTLSAccel
(internal/build, `testdata/tls_accel.c`), TestTLSDiff and the `http_tls`
goldens (internal/llvm). Numbers: `bench/results/native-runtime-2026-10-10.md`.

## Context

With a new connection per request the native HTTPS server did 9.7k req/s
at 763 us of CPU per request against the Go backend's 17.0k at 230 us
(decision 0105 measured 9.8k and 17.4k). A profile of the server under
that load showed where Mbed TLS 4.1 spends a TLS 1.3 server handshake:
the X25519 key share's public key computed by the library's generic
Montgomery ladder on heap bignums (41 percent; HACL*, which Mbed TLS
has, was only used for the key agreement), ECDSA P-256 signing (25
percent, a third of it two binary GCD inverses), and worker threads
asleep on the PSA key store's one mutex (about 25 percent of samples).
SHA-256 and SHA-512 ran in portable C. The Go backend's server also
sends session tickets by default; the native server sent none.

## Decision

1. **Interpose, do not patch.** The vendored sources stay unmodified.
   Xo files in `third_party/mbedtls/xo/` are compiled into the library
   archive; a library file whose function they replace is compiled with
   `-D<name>=xo_builtin_<name>` (`mbedtls.FileDefines`), and the Xo file
   defines the original name, takes the fast case, and calls the renamed
   function otherwise. The flags are part of the archive's cache key.
2. **X25519 public keys through HACL\***
   (`mbedtls_psa_ecp_export_public_key` for 255 bit Montgomery key
   pairs: X25519(k, 9)). Same bytes as the ladder (RFC 7748 vector and
   400 random keys in TestTLSAccel).
3. **P-256 inverses by Fermat** (`mbedtls_mpi_gcd_modinv_odd` without a
   GCD, modulo P-256's prime or order only: A^(m-2) with the library's
   Montgomery exponentiation, public exponent, constant time
   multiplications). Half the time of the binary GCD; the contract's
   "indeterminate" result for A = 0 becomes 0. Checked against the
   original on random values and edges and by the RFC 6979 A.2.5
   signature, byte for byte.
4. **A spinning lock for the library's mutexes**: Mbed TLS's
   `mbedtls_mutex_lock_ptr` (a global function pointer in the pthread
   configuration) is set, once in xo_tls.c's `pthread_once`, to a lock
   that tries 100 times with a CPU pause before blocking. Still a
   pthread mutex, so the library's condition variables work unchanged.
5. **SHA-256 and SHA-512 instructions** when the CPU has them
   (`MBEDTLS_SHA256_USE_ARMV8_A_CRYPTO_IF_PRESENT`,
   `MBEDTLS_SHA512_USE_A64_CRYPTO_IF_PRESENT`; x86-64 is unaffected). The
   library caches its CPU check in unlocked statics, so xo_tls.c hashes
   once in its `pthread_once` (ThreadSanitizer found the race).
   AES-NI, the ARMv8 AES instructions, and the fixed point tables were
   already on.
6. **Session tickets as Go's server sends them**: one ticket after each
   full handshake (TLS 1.3 and 1.2), AES-256-GCM keys random per server
   rotated every 24 hours (Go: 24 hours), tickets good for 24 hours (Go:
   7 days, shorter is the safer side), TLS 1.3 resumption only with a
   fresh ECDHE (psk_dhe_ke, as Go), no pure PSK, no early data, no
   session ID cache. The native client keeps no sessions (net/http's
   default Transport has no ClientSessionCache either).

Measured (interleaved, medians of 3; full handshake per request): llvm
9743 to 15067 req/s and 763 to 377 us per request before tickets, 14469
and 389 us with them; Go backend 17078 at 234 us. With the client
resuming: llvm 18346 at 244 us, Go 18843 at 210 us.

## Consequences

- A full native handshake costs about 1.7 times Go's CPU (was 3.3), a
  resumed one about 1.15 times.
- ECDSA P-256 signing is now the largest cost (about 45 percent, 112 us
  per signature on one M4 core). The next step is a fast constant time
  P-256 signer behind the same interposition (fiat-crypto's verified
  field and scalar arithmetic, Go's complete addition formulas, a
  precomputed generator table, RFC 6979 nonces through Mbed TLS's
  HMAC_DRBG so the output stays byte identical). It needs the owner's
  approval to vendor fiat-crypto (refused by the session's permission
  check on 2026-10-10).
- Every full handshake writes a ticket (about 3 percent of its CPU).
- A library update must recheck that the interposed functions keep
  their names, signatures, and single callers (TestTLSAccel fails at
  link time otherwise, and TestTLSDiff covers the handshakes).

## Alternatives

- **Patch the vendored files**: simpler, but every update would have to
  reapply the patches and `scripts/vendor-mbedtls.sh` checks the
  release's checksum against unmodified sources.
- **Mbed TLS's PSA driver interface** for X25519 and P-256: the driver
  dispatch is generated code (Python and JSON at the library's build
  time), which the toolchain does not run.
- **p256-m** (vendored, a PSA driver): cannot coexist with other curves
  in TLS (0105), and is 5 times slower than the built in P-256 here.
- **`MBEDTLS_THREADING_ALT`**: the same lock with more code; the pointer
  of the pthread configuration suffices.
- **Fewer worker threads for TLS**: hides the contention instead of
  removing it.

## Owner approval (2026-10-10)

Vendoring fiat-crypto's P-256 code (pinned upstream commit, checked
hash, as for Mbed TLS) for the fast signer described above is approved;
scheduled at low priority. Done in decision 0132 (commit `f3ecb2c...`,
`scripts/vendor-fiat-crypto.sh`).
