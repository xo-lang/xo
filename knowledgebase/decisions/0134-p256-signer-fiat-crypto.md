# 0134: P-256 ECDSA on fiat-crypto in front of Mbed TLS

Numbered 0132 until 2026-10-10, when it was renumbered because the
mascot decision already had 0132; commit messages before then call it
0132.

Status: Superseded by 0202 (2026-10-15: Mbed TLS and fiat-crypto removed). Was: Implemented (2026-10-10). Amends 0105 and 0117 (the next step
0117 planned). Owner approval to vendor fiat-crypto: 0117, "Owner
approval (2026-10-10)", repeated in the session on 2026-10-10. Code:
`third_party/fiat-crypto/` (vendored, `scripts/vendor-fiat-crypto.sh`),
`third_party/mbedtls/xo/xo_p256.c`, `third_party/mbedtls/mbedtls.go`
(`FileDefines`, `IncludeDirs`, `Tree`), `internal/build/tls.go` (cache
key). Tests: TestTLSAccel, TestP256, TestTLSAccelSanitizers
(internal/build, `testdata/tls_accel.c`), TestVendored
(third_party/fiat-crypto), TestTLSDiff and the `http_tls` goldens.
Numbers: `bench/results/native-runtime-2026-10-10.md` section 6.

## Context

After decision 0117 a native full TLS handshake cost about 1.7 times
the Go backend's CPU, and ECDSA P-256 signing was the largest part
(about 45 percent, 110 us per signature on one M4 core in Mbed TLS's
portable bignum code). Go signs in about 10 to 15 us with assembly.
TLS signs with deterministic ECDSA (`MBEDTLS_PK_ALG_ECDSA` is
`PSA_ALG_DETERMINISTIC_ECDSA` when `MBEDTLS_ECDSA_DETERMINISTIC` is on),
so a replacement must give the same bytes.

## Decision

1. **Vendor fiat-crypto's generated C** for P-256 (field modulo p and
   scalars modulo n, 64 bit words: `fiat-c/src/p256_64.c`,
   `p256_scalar_64.c`) from github.com/mit-plv/fiat-crypto at commit
   `f3ecb2cf1f771de34771e967193ee7f0ee0b0acf`, unmodified, with its
   license files (MIT, Apache-2.0, or BSD-1-Clause). The script pins the
   commit and every file's SHA-256; TestVendored checks the files
   against `SHA256SUMS`. Only the 64 bit files: every native host is 64
   bit and clang has `__int128`.
2. **`xo_p256.c` replaces `mbedtls_psa_ecdsa_sign_hash` and
   `mbedtls_psa_ecdsa_verify_hash`** through 0117's interposition
   (`psa_crypto_ecp.c` compiled with `-D<name>=xo_builtin_<name>`). The
   fast path takes P-256 key pairs (32 byte private keys) and P-256
   public keys (65 byte uncompressed); everything else, and the rare
   cases it does not handle (r or s zero, a key out of range, a random
   generator error), calls the library's function, which then does the
   whole operation itself.
3. **Arithmetic as Go's crypto/internal/fips140/nistec (generic
   build)**: projective points with the complete formulas of Renes,
   Costello, and Batina (2015, a = -3; no special case for doubling or
   infinity), a generator table of 43 x 32 affine points (each table
   pre-doubled six times, built once in a `pthread_once` with one batched
   inversion, 88 KB, about 0.7 ms), signed 6 bit windows (Booth) for k*G
   and 5 bit windows with a 16 point table for u*Q, Fermat inverses with
   Go's addition chains for p and n.
4. **The same signatures as the library**: k comes from Mbed TLS's own
   HMAC_DRBG seeded with d || bits2octets(h) (RFC 6979, the hash of the
   algorithm) and the same rejection sampling as
   `mbedtls_mpi_core_random` (draw 32 bytes, accept 1 <= k < n); a
   randomized ECDSA key draws from the PSA generator instead.
   Verification returns what `mbedtls_ecdsa_verify` would (r, s in
   [1, n-1], R not infinity, x(R) mod n == r).
5. **Constant time**: no branch or memory index depends on d, k, or
   k^-1. Selections are fiat's masked `selectznz`/`cmovznz` behind its
   empty `asm` value barrier; table lookups read every entry and pick
   with masks; window positions are public; field and scalar arithmetic
   is fiat's (branch free). How it was checked: review of the source,
   and the assembly of `xo_p256.c` (clang 21, `-O2`, arm64 and
   x86-64, also `-fno-inline` to attribute branches to functions; stack
   protector checks excluded with `-fno-stack-protector`): the only
   conditional branches in `base_mult`, `point_mult`, `fe_inv`, `sc_inv`
   and their inlined callees are loop counters and the public window
   index; `pt_add`, `pt_double`, `pt_add_affine`, `ct_eq`, `booth`,
   `limbs_lt`, `limbs_reduce`, and `fe_cneg` have none. The branches in
   `p256_sign` are on public facts (key validity, random generator
   errors, whether a drawn k is rejected, r or s zero), as in the
   library. Not checked by a dynamic tool (no valgrind or ctgrind on
   macOS); the next check is dudect style timing classes or ctgrind
   under Linux (see Consequences).

Measured (M4 Pro, one core, `XO_P256_BENCH=2000 go test ./internal/build
-run TestP256Bench`, load average about 15): signing 23.0 us against the
library's 110.1 us (24.8 us through `psa_sign_hash`); verification 94.1
us against 377.1 us. Full handshake per request
(`SCHEME=https NEWCONN=1`, medians of 3, interleaved): llvm 13522 to
15351 req/s and 354 to 251 us of CPU per request, arm64 15854 at 251
us, Go backend 15598 at 227 us.

## Consequences

- A native full handshake costs about 1.1 times the Go backend's CPU
  (was 1.55 in the same run, 1.7 in 0117's).
- Clients verify certificate chains about 4 times faster (P-256
  verification 94 us against 377 us).
- The fast path is deterministic and byte identical to the library's:
  TestTLSAccel compares 800 keys and messages (SHA-256, -384, -512,
  hashes of 0 to 64 bytes, edge keys 1, 2, n-1, n-2, edge hashes, keys
  out of range) and verification of good, altered, and out of range
  signatures; TestP256 compares 300 signatures with Go's
  `crypto/ecdsa` RFC 6979 signatures and verifies Go's randomized ones;
  with `XO_WYCHEPROOF_DIR` it runs Wycheproof's P-256 P1363 vectors (594
  with SHA-256 and SHA-512, all agree with the library and the
  expected results, 2026-10-10). ASan and UBSan run the checks
  (TestTLSAccelSanitizers).
- A fiat-crypto update reruns the script with new pins (README in
  `third_party/fiat-crypto`); an Mbed TLS update must recheck that the
  two interposed functions keep their names and signatures (link
  failure otherwise) and that the deterministic nonce is still drawn as
  above (TestTLSAccel fails otherwise).
- Next steps for handshake CPU, by expected gain: a profile of the
  native server under `NEWCONN=1` now (signing should be about 10
  percent); a combined u1*G + u2*Q (Shamir, variable time is fine for
  public inputs) for verification; P-256 ECDHE key shares through the
  same code (`mbedtls_psa_ecp_export_public_key` and the ECDH agreement
  for SECP_R1 256, for clients that pick P-256); a dynamic constant time
  check (dudect classes for d and k in `tls_accel.c`, or ctgrind in a
  Linux container).

## Alternatives

- **Mbed TLS's p256-m driver**: 5 times slower than the built in code
  here and cannot coexist with other curves in TLS (0105, 0117).
- **BoringSSL's P-256** (fiat-crypto based with assembly): C++ library
  around it; taking its C files would vendor a second large project for
  one function.
- **Go's table embedded as data** (`p256_table.go`, 88 KB): built at
  first use instead (0.7 ms once), no generated source to keep.
- **Variable time verification**: faster, but the constant time code
  already exists and verification is not on the server's path.
- **The 32 bit fiat files**: no 32 bit native host.
