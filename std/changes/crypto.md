# std/crypto changes

## 2026-11-04: feature: crypto.ecdsa_private_key

- Kind: feature
- Commit: this push
- Found by: the AWS SDK work (SigV4a derives an ECDSA P-256 key from a scalar)
- Go reference: crypto/ecdsa ParseRawPrivateKey (go1.26): a wrong length is `invalid scalar length`; zero and a scalar at or above the order are `ecdsa: invalid public key encoding`; a curve other than P-224, P-256, P-384, P-521 is refused
- Test: crypto_test.xo (Go's public points for 12 scalars on four curves, sign and verify, the refusals) and golden std_ecdsa_private_key (every backend)
- Shipped in: not yet released

`crypto.ecdsa_private_key(alg, d)` makes the key from its big endian scalar, as long as the curve's order; `public_key()` of it is the public point.

## 2026-11-04: perf: crypto.hmac with SHA-256 takes the one shot path

- Kind: perf
- Commit: this push
- Found by: bench/perf/otp sha256 1.27 of Go's wall; XO_PROF on the Mac: the compress was 11 percent, the rest setup (Hasher, padded key copies, state lists)
- Go reference: crypto/hmac (a hash.Hash for each call, the key pads absorbed once)
- Test: the crypto, otp, jwt, hkdf, tls and x509 corpora (arm64, LLVM, Go backend), `xo test --std-module=native/crypto`
- Timing: host 1 A/B otp sha256 0.818 of before (0.802-0.826, n=6), about 1.04 of Go's wall
- Shipped in: not yet released

`crypto.hmac` with `Hash.Sha256` calls digest.xo's `hmac_sha256` (the pad blocks hashed straight into word states, the outer hash of one block), in place of the generic Hasher. HS256 JWTs, TOTP and every other HMAC-SHA256 caller of `crypto.hmac` take it.

## 2026-11-04: perf: SHA-256 and SHA-512 on wasm run 16 rounds in a loop

- Kind: perf (size, speed)
- Commit: this push
- Found by: the wasm component size breakdown (decision 0447 queue, component size): digest_tables' unrolled
  sha256_block and sha512_block were 34 KB and 38 KB of every wasm module that hashes (TLS, JWT, Ed25519)
- Go reference: crypto/internal/fips140/sha256 and sha512 blockGeneric (a loop over the rounds)
- Test: the crypto, jwt and tls corpora on wasm, TestWasm* goldens; shabench's digests equal before and after
- Timing: host 1 under wasmtime 50, 1 MiB hashed 64 times, medians of 5: SHA-256 0.90 of before (1,409 -> 1,267 ms),
  SHA-512 0.89 of before (981 -> 873 ms); Ed25519 signing (2,000) unchanged (1,241 ms both)
- Size: an HTTPS client (xo-fetch) 2,193,968 -> 2,132,945 bytes (-61,023)
- Shipped in: not yet released

On wasm, `sha2.block256`, `compress` and digest.xo's Ed25519 hash call `cpu.sha256_block_r16` and
`cpu.sha512_block_r16`: 16 rounds unrolled in a loop of 4 (5 for SHA-512), the message schedule in 16 locals
updated in place, the working variables renamed round by round; as digest_tables, the SHA-256 rounds leave the
bits above 32 unmasked. Native targets keep the CPU instructions and digest_tables' unrolled rounds.

## 2026-11-04: perf: ECDSA verification multiplies with sliding windows

- Kind: perf
- Commit: this push
- Found by: bench/perf/jwt es256_verify about 1.72 of Go's wall (field arithmetic is the rest)
- Go reference: crypto/internal/fips140/nistec's variable time multiplication in ECDSA verify
- Test: the jwt, crypto, x509 and tls corpora (arm64, LLVM, Go backend), internal/repotest
- Timing: host 1 A/B es256_verify 0.964 of before (0.953-0.970, n=6)
- Shipped in: not yet released

`pt_mul_vt_public` (public scalars only, verify) reads sliding windows of up to 5 bits over the odd multiples [1]P to [31]P: about 43 additions and 16 table entries, against 60 and 15 for fixed 4 bit windows, and no doublings of the identity before the first set bit.

## 2026-11-04: perf: bigmod.mac_row as two carry chains (Go's addMulVVW shape on arm64)

- Kind: perf
- Commit: this commit
- Found by: the Lead (mac_row is 75% of an RS256 verify; Go has assembly for addMulVVW)
- Go reference: math/big arith_arm64.s addMulVVW (go1.26.1): the same two ADDS/ADCS chains in Xo, no assembly
- Test: internal/llvm TestRSADiff and TestConstantTimeLLVM; TestStdCorpora; tls_rsa, tls_x509, tls_verify, tls_nistec, http_tls_keys goldens under XO_FREE_CHECK=1; bench/perf/jwt and rsa outputs
- Timing: `xo bench --ab` on linux/arm64 (host 1, n=3), A the parent: bench/perf/jwt rs256_verify 0.933 (0.865 to 0.948), bench/perf/rsa sign2048 0.957 (0.934 to 0.964), sign4096 0.868 (0.865 to 0.870), verify2048 0.921 (0.869 to 1.006, within noise)

mac_row's 4-word round added each word's low half and the carry word in
series (two adds a word, 16 for four, and a store each). It now adds x and
the four low halves in one carry chain, then the carry word and the high
halves one word up in a second, the carry kept in the flags (arm64: 8
multiplies, 10 adds in two adds/adcs runs, two stp). No branch depends on
the values, as before.

## 2026-11-04: perf: ECDSA verification multiplies with public-input windows

- Kind: perf
- Commit: this push
- Found by: the jwt twin (bench/perf/jwt es256_verify 1.85 of golang-jwt) and the Mac XO_PROF: all time in point arithmetic, 64 percent in the variable base multiplication
- Go reference: crypto/internal/fips140/ecdsa Verify (a variable-time ScalarMult)
- Test: internal/stdtest corpora crypto, x509 and jwt (ES256, ES384, ES512 tokens) on arm64
- Timing: host 1 A/B es256_verify 0.954 of before (0.944-0.966); the Go row is in bench/results/std-twin-jwt-2026-11-03.md
- Shipped in: not yet released

`nistec.verify` has only public inputs, so it uses `pt_mul_vt_public` and `pt_mul_base_vt_public`: the window's table entry is read directly (no mask select over 16 entries) and a zero digit adds nothing. Signing and ECDH keep the constant time versions; the new functions are named `*_vt_public`, carry a VARIABLE TIME comment, and internal/repotest TestVtPublicOnlyInVerify fails when anything but `verify` calls them. The rest of the gap to Go is field arithmetic (Go's P-256 is assembly): next experiments are a wNAF window for the variable base and mixed (affine) additions for the base table.

## 2026-11-03: perf: PEM decoding compares a block's type and trailer without views

- Kind: perf
- Commit: this push
- Found by: xo vet --cost XO0913 (cmd/xo TestVetCostStd), once bench/perf/jwt reached pem_decode_one
- Go reference: encoding/pem Decode (bytes.HasSuffix, bytes.HasPrefix)
- Test: internal/stdtest corpora crypto, x509, x509create and jwt on arm64; cmd/xo TestVetCostStd
- Timing: none (a PEM block's two compares; the notes were the point)
- Shipped in: not yet released

pem_decode_one checked a block's "-----BEGIN TYPE-----" line and its END
trailer by slicing them and comparing the views; it now uses
Bytes.ends_with and starts_with, which make no view.

## 2026-11-03: perf: SHA-2 finishes with one append and four words a call

- Kind: perf
- Commit: this push
- Found by: the std timing rows (std-twin-rows18: otp sha256 1.327 and sha512 1.376 of Go) and XO_PROF on linux/arm64
- Go reference: crypto/sha256 and crypto/sha512 checkSum (one padding append, the words put big endian)
- Test: internal/stdtest corpora otp, crypto, aead and jwt on arm64 and Go; bench/perf/otp twincheck
- Timing: against its parent, otp sha256 0.931 of the wall (sha512 within noise); otp against Go after this and c5fbea698: sha256 1.316 (was 1.327), sha512 0.937 (was 1.376) (bench/results/std-twin-otp-2026-11-03.md)
- Shipped in: not yet released

A SHA-224, SHA-256, SHA-384 or SHA-512 digest pushed its zero padding an
8 byte word at a time and its digest a word a call, each a capacity
check; the padding is now one append from a constant and the digest four
words a call, as SHA-1's since the same day.

## 2026-11-03: perf: SHA-512 and SHA-384 with arm64's SHA512 instructions

- Kind: perf
- Commit: this commit
- Found by: Lang 2's std/otp census (TOTP with SHA-512 at 1.376 of Go's time, 72% in the software sha512_block); Go's crypto/sha512 uses the instructions on arm64
- Go reference: crypto/sha512 sha512block_arm64.s (the kernel's sha512-ce sequence)
- Test: TestSHA512HWDiff (internal/llvm: blocks against a Go reference, SHA-512 and SHA-384 digests, split and HMAC against crypto/sha512 on lengths 0 to 300 and around 4096, both arm64 backends); golden cpu_sha512 (arm64, LLVM, Go parity)
- Timing: `xo bench --ab` on the first linux/arm64 offload host, 2026-11-03, bench/perf/sha512 (SHA-512 and SHA-384 of 64 KiB 1,000 times each, HMAC-SHA512 of an 8 byte counter 600,000 times), A b19877176 (the parent with only the program) and B 4e10c4b77: wall 624.8 ms to 412.5 ms, B/A 0.66 (95% interval 0.657 to 0.661, 3 rounds); the HMAC loop's per MAC work (pads, Hasher state copies) is what is left

## 2026-11-03: perf: HMAC pads a key a word at a time; SHA-1 finishes with one append

- Kind: perf
- Commit: 075763993, c5643cda2
- Found by: the std timing rows (std-twin-rows14: otp sha1 1.36 of Go) and XO_PROF on linux/arm64
- Go reference: crypto/hmac New (the pads), crypto/sha1 checkSum
- Test: internal/stdtest corpora otp, crypto and aead on arm64, LLVM and Go; bench/perf/otp twincheck
- Timing: otp sha1 1.071 of Go's wall (was 1.36), 0.969 of its CPU (bench/results/std-twin-rows18-2026-11-03.md)
- Shipped in: not yet released

HMAC built each 64 to 136 byte padded key a checked byte at a time, twice
per MAC; it now starts from the pad constant and XORs the key in 8 bytes
at a time. A SHA-1 digest pads with one append of zeros and writes its
five words with one call.

## 2026-11-03: perf: SHA-1 with the CPU's SHA-1 instructions (arm64 SHA1C and the rest, x86-64 SHA-NI)

- Kind: perf
- Commit: this commit
- Found by: Lang 3's std/otp timing row (HMAC-SHA1 at 4.8x Go's time, otp branch 239f0b7b9); Go's crypto/sha1 uses the instructions
- Go reference: crypto/sha1 sha1block_arm64.s and sha1block_amd64.go's SHA-NI path (the kernel's sha1-ce and sha1_ni sequences)
- Test: TestSHA1HWDiff (internal/llvm: blocks against a Go reference, digest, split and HMAC against crypto/sha1 on lengths 0 to 200 and around 4096, both arm64 backends); golden cpu_sha1 (arm64, LLVM, amd64 sample)
- Timing: `xo bench --ab` on the first linux/arm64 offload host, 2026-11-03, bench/perf/sha1 (SHA-1 of 64 KiB 2,000 times and HMAC-SHA1 of an 8 byte counter 1,500,000 times), A 5cb9bae79 (the parent with only the program) and B 3f303624e: wall 3012 ms to 645.6 ms, B/A 0.214 (95% interval 0.213 to 0.217, 6 rounds)

## 2026-10-29: feature: ed25519_sign takes Go's 64 byte private key (0.65 of Go's signing time)

- Kind: feature
- Commit: this commit
- Found by: the Ed25519 timing rows: signing from the 32 byte seed computed the public key again, a second base multiplication Go's Sign (given its 64 byte private key) does not do; owner approved the API 2026-10-29 (decision 0189 amendment)
- Go reference: crypto/ed25519 Sign (PrivateKey is the seed then the public key; the public key half is used as stored)
- Test: corpus crypto signs every RFC 8032 and Go-generated vector from seed + public key too; golden native_ed25519 (key64) on arm64, LLVM and the Go backend; bench/perf/ed25519 signkey twincheck
- Timing: bench/perf/ed25519, quiet --ab, 15 rounds, Xo / Go wall: signkey 0.65 (0.647 to 0.653), instructions 0.471; sign from the seed 1.183, so signkey is 0.55 of the seed form's time

`crypto.ed25519_sign(key, msg)` takes the 32 byte seed as before, or
Go's 64 byte private key. Any other length faults with
`crypto.ed25519_sign: the key is N bytes, want 32 or 64`. As in Go, a
stored public key that does not belong to the seed is not detected and
gives a signature that does not verify.


## 2026-10-29: perf: Ed25519's scalars on Go's fiat-crypto Montgomery products (sign 0.656, verify 0.892 of their time)

- Kind: perf (port)
- Commit: this commit
- Found by: the Ed25519 port's timing row (sign 1.81x Go's wall at 1.09x its instructions): sc_reduce reduced a 512 bit hash bit by bit, 512 dependent steps, twice per signature and once per verify
- Go reference: crypto/internal/fips140/edwards25519 scalar.go (SetUniformBytes, MultiplyAdd) and scalar_fiat.go (fiatScalarMul's Montgomery constant 0xd2b51da312547e1b)
- Test: corpora crypto, x509 and x509create on arm64, LLVM and Go; the TLS goldens; TestConstantTimeLLVM; test/codegen/consttime (the masks moved to sc_sub_l, the conditional subtraction of L); bench/perf/ed25519 twincheck; fixed point
- Timing: bench/perf/ed25519, quiet --ab, 15 rounds, the parent's std as A: sign wall 0.656 (0.655 to 0.657), instructions 0.795; verify 0.892 (0.889 to 0.894), 0.944. Xo / Go wall: sign 1.185 (was 1.812), instructions 0.865; verify 0.795, instructions 0.511

sc_reduce splits the hash into its low and high 256 bits and sums
lo * R^2 / R and hi * R^3 / R (R = 2^256), which is x * R, then a
Montgomery product with 1 divides R out: three 4x4 word products (CIOS,
as fiat-crypto's) where Go splits into 21 byte pieces; sc_muladd is
k * a / R, times R^2 / R, plus r. Every step is constant time: the final
subtraction of L keeps its result by masks. A Go uint64 constant past
2^63 is written negative (gotchas/hex-literal-past-int-max.md).


## 2026-10-29: perf: Ed25519 on Go's edwards25519 algorithms over the x25519 fe64 field (sign 0.352 and verify 0.404 of their time)

- Kind: perf (port)
- Commit: this commit
- Found by: bench/perf/ed25519 (new) and bench/perf/x509 create at 2.676x Go, 99% of its profile Ed25519; Xo's Ed25519 ran a constant time ladder for every scalar multiplication
- Go reference: crypto/internal/fips140/edwards25519 (ScalarBaseMult: signed radix 16 over basepointTable; VarTimeDoubleScalarBaseMult: NAF width 5 for A, width 8 over basepointNafTable), crypto/ed25519 sign and verify
- Test: corpora crypto, x509 and x509create (Go-generated keys and signatures, byte for byte) on arm64, LLVM and Go; the TLS goldens; TestConstantTimeLLVM (x25519.base_mult and ed_base: no secret dependent branch or select); test/codegen/consttime; bench/perf/ed25519 twincheck

Key generation and signing use x25519.ed_base, Go's ScalarBaseMult over
the base table x25519 already had; verify uses x25519.ed_check, Go's
variable time double scalar multiplication with the width 8 table that
scripts/x25519btab.py now emits (BNAF, D2). The field is decision 0231's
fe64 (4x64 bit limbs on adc chains), not Go's 51 bit limbs: it is this
repository's faster field. Quiet --ab, 15 rounds, 5000 operations
(bench/perf/ed25519), the parent's std as A: keygen wall 0.273 (0.268 to
0.285), instructions 0.179; sign 0.352 (0.348 to 0.364), 0.221; verify
0.404 (0.401 to 0.407), 0.258. Against Go's crypto/ed25519 as A: keygen
0.892 (0.703 to 0.963, noisy), instructions 0.489; sign 1.812 (1.797 to
1.82), instructions 1.089, slower, its wall well past its instruction
ratio (next: profile sign's hashing and scalar steps); verify 0.896
(0.888 to 0.899), instructions 0.542.


## 2026-10-29: feature: marshal_private_key

- Kind: feature
- Commit: this commit
- Found by: std/x509's self_signed (a generated key must be saved)
- Go reference: crypto/x509 MarshalPKCS8PrivateKey (go1.26.1)
- Test: internal/stdtest/testdata/corpus/x509create (Go's PKCS #8 of Go's RSA, ECDSA and Ed25519 test keys, byte for byte, and parse_private_key reading it back)

`marshal_private_key(k)` is k as PKCS #8 DER, Go's encoding: PKCS #1
inside for RSA, an ECPrivateKey with its public key and without its
curve (the algorithm names it) for ECDSA, the seed for Ed25519.
## 2026-10-29: perf: the AES key schedule on the CPU's AES instructions keeps its words in registers

- Kind: perf
- Commit: this commit
- Found by: bench/perf/aead gcm_key still 1.357 times Go's time after the schedule above; the generic loop pushed every word to a list and read it back nk words later (36% of a key's setup)
- Go reference: crypto/internal/fips140/aes/aes_arm64.s expandKeyAsm (ks128, ks192, ks256)
- Test: internal/stdtest/testdata/corpus/aead and crypto; goldens aead_diff, crypto_prims and http_tls on arm64

aesgcm.schedule_hw makes the round keys for 128, 192 and 256 bit keys
with the words in locals, written two to a slot of the round key list
as they are made, where the CPU has AES instructions. Quiet --ab, 15
rounds, the parent's std as A: an AES-256 key made (gcm_key) wall 0.884
(0.878 to 0.897), instructions 0.689; small seals (gcm_small) within
noise. Against Go's crypto/aes: 1.204 (1.183 to 1.212), from 1.357.

## 2026-10-29: feature: AEAD (AES-GCM, ChaCha20-Poly1305) and HKDF

- Kind: feature
- Commit: this commit
- Found by: milestone 0.35.0 S4 (public AEAD and HKDF from the implementations TLS already has)
- Go reference: crypto/cipher (cipher.AEAD, NewGCM, NewGCMWithRandomNonce), golang.org/x/crypto/chacha20poly1305, crypto/hkdf (Extract, Expand, Key) (go1.26.1)
- Test: internal/stdtest/testdata/corpus/aead (scripts/stdcorpus aead.go: crypto/cipher's aesGCMTests and crypto/hkdf's hkdfTests, each checked against Go here; Wycheproof's AES-GCM and ChaCha20-Poly1305 vectors with a 96 bit nonce and a 128 bit tag, valid and invalid, and its HKDF-SHA-1/256/384/512 valid vectors; seeded AES-GCM rows sealed by Go; a changed byte anywhere not opening; key sizes; seal_random and open_random; hkdf over every Hash) on go, arm64 and LLVM; bench/perf/aead twin

New in std/crypto: `enum Aead { AesGcm, ChaCha20Poly1305 }`,
`aead_key(alg, key) -> AeadKey ! AeadErr`, and on an AeadKey
`seal_random` and `open_random` (a random 12 byte nonce kept first, the
way to seal unless a protocol counts nonces) and `seal` and `open` with
an explicit nonce; `hkdf_extract`, `hkdf_expand` and `hkdf` over any
Hash. A failed open is one AeadErr, `cipher: message authentication
failed`, on every backend; a nonce that is not 12 bytes is a fault. An
AeadKey has no Display and derives nothing, so it cannot be printed or
encoded (Debug text `<aeadkey>`). Natively the ciphers are the TLS
stack's (aesgcm.xo, chacha.xo); the Go backend uses Go's crypto/cipher
and crypto/hkdf. Timing (bench/perf/aead, its Go twin as A, quiet --ab,
15 rounds, wall): AES-GCM seal and open of 64 bytes 0.908 (0.892 to
0.926) of Go's time, of 1 MiB 0.744 (0.732 to 0.758); ChaCha20-Poly1305
of 64 bytes 1.008 (0.998 to 1.014, within noise), of 1 MiB 0.883 (0.872
to 0.889); HKDF-SHA-256 of 32 bytes 0.965 (0.952 to 0.985); an AES-256
key made 1.357 (1.335 to 1.365), the one row over 1.2: the key schedule,
a word a step in Xo where Go expands with AESE in assembly, is the next
step.

## 2026-10-29: perf: ChaCha20-Poly1305 under 256 bytes on arm64 with 64 bit Poly1305 limbs

- Kind: perf
- Commit: this commit
- Found by: bench/perf/aead chacha_small (64 bytes) at 1.70 times Go's instructions; its profile (Poly1305's 26 bit limbs, tag26, 64% cumulative)
- Go reference: golang.org/x/crypto/internal/poly1305 sum_generic.go (64 bit limbs, bits.Mul64)
- Test: internal/stdtest/testdata/corpus/aead (Wycheproof ChaCha20-Poly1305, every length to 256 and past it); goldens chacha_blocks4, crypto_prims, tls13_rfc8448, tls12_client on arm64 and LLVM

chacha.seal and open take the 64 bit limb Poly1305 path (seal64, open64,
until now amd64's) on arm64 for messages under 256 bytes, where no NEON
block runs: arm64's umulh does a 64 by 64 bit product in one instruction.
Longer messages keep the 26 bit path beside the NEON key stream. Quiet
--ab, 15 rounds, the parent's std as A: 64 byte seal and open wall 0.89
(0.884 to 0.903), instructions 0.907; 1 MiB within noise (1.001).

## 2026-10-29: perf: the AES key schedule makes its words once

- Kind: perf
- Commit: this commit
- Found by: bench/perf/aead gcm_key at 1.50 times Go's time; its profile (aesgcm.schedule 27% flat: its word list grown from empty, the round constants and the bitsliced loop's list made on every key)
- Go reference: crypto/internal/fips140/aes aes_generic.go expandKeyGeneric (a fixed array)
- Test: internal/stdtest/testdata/corpus/aead and crypto; goldens crypto_prims, cpu_prims and the TLS goldens on arm64 and LLVM

aesgcm.schedule (every AES key: std/crypto's AeadKey, TLS's record keys)
makes its word list with its final length, reads the round constants
from a constant, and makes the bitsliced loop's list only when that loop
runs. Quiet --ab, 15 rounds, the parent's std as A: an AES-256 key made
wall 0.897 (0.886 to 0.907), instructions 0.879; against Go 1.357.

## 2026-10-28: perf: SHA-3 and SHAKE on arm64 with the SHA3 extension (SHA3-256 0.577 of its time, 0.948 of Go's on 1 MiB)

- Kind: perf
- Commit: this commit
- Found by: Xo Lang 5's field-list compiler item (a scratch SHA3-256 bench against Go's crypto/sha3: 2.2 times Go's instructions, 1.66 times its time)
- Go reference: crypto/internal/fips140/sha3 sha3_arm64.s (keccakF1600NEON: the 25 lanes in v0 to v24, theta's parities with eor3 and rax1, rho and pi with xar, chi with bcax; used on darwin only, sha3_arm64.go)
- Test: std/native/llrt/cpu_test.xo (keccak_f1600's kernel against its body on 200 seeded states; FIPS 202's zero-state lanes); internal/llvm golden sha3_vectors (SHA3-256 and SHA3-512 around both rates' block edges and in pieces, want.txt from Go's crypto/sha3; arm64 and LLVM); crypto_prims and tls_mlkem_off goldens; compiler/macho encode tests (eor3, bcax, rax1, xar against clang's words); test/codegen/intrinsics sha3.permute; bench/perf/sha3's checksums equal Go's

sha3.xo's Keccak-f[1600] runs as one block of NEON instructions with the
SHA3 extension where cpu.hw() has HW_SHA3 (every Apple arm64 CPU; not
Linux, as Go): Go's register plan, the round constants read from the RCS
table. Every sponge goes through it: SHA3-256, SHA3-512, SHAKE128 and
SHAKE256, so ML-KEM's hashing too. amd64, wasm and Linux arm64 keep the
scalar rounds. Quiet --ab, 15 rounds, the parent's xo and std as A (bench/perf/sha3): SHA3-256 of 1 MiB 40 times wall 0.577 (0.574 to 0.587), instructions 0.501; 200,000 short messages wall 0.64 (0.629 to 0.647), instructions 0.614. Against Go (twin.go.txt as A): big 0.948 (0.933 to 0.953), small 1.151 (1.145 to 1.157; instructions 1.43, the sponge's per-call work), big was 1.66 before.

## 2026-10-27: perf: AES-GCM on amd64 eight blocks a round with AES-NI and PCLMULQDQ, and SubWord with AES-NI (seal 3.67 to 1.09 times Go's time, open 3.49 to 1.02, key setup 1.28)

- Kind: perf
- Commit: this commit
- Found by: Xo Lang 4's amd64 parity audit (bench/results/amd64-crypto-vs-go-2026-10-27.md: AES-GCM 3.5 to 4.4 times Go on an i7-9750H)
- Go reference: crypto/internal/fips140/aes/gcm gcm_amd64.s (gcmAesEnc, gcmAesDec: eight blocks a round, the previous 128 bytes hashed during the next ones' rounds)
- Test: internal/llvm golden cpu_prims (aes_gcm8 against the portable body, apart and in place, 11, 13 and 15 round keys; darwin/amd64 native and LLVM builds on the i7-9750H, linux/amd64 in Docker); the aesgcm twin's checksums equal Go's (N 1, 7, 2000); tls goldens in the Docker amd64 run; xo dev codegen test/codegen (intrinsics: aesgcm.Gcm.gcm_bulk's interleave, which fails on the parent's xo)

amd64 ran AES-GCM's bulk through the portable body: one AES-NI block per
call with its state through memory, and GHASH's carry-less products one at
a time through general registers (all of a seal's profile). The
cpu.aes_gcm8 intrinsics, arm64-only until now, are lowered on native
amd64 and LLVM on x86-64 (compiler/ssa x86aesgcm8.xo): eight counter
blocks through each round key, each block's GHASH as Karatsuba's three
products summed over the eight and reduced once, in Xo's own GHASH
convention (cpu.ghash8_soft's, so the portable body stays the oracle).
Sealing hashes the previous 128 bytes during the next ones' rounds, as
Go does; opening hashes the bytes it decrypts during their own rounds.
new_gcm builds H^8 to H^5 on amd64 too (they were 0 there: a first
version's tags of 128 bytes or more were wrong while the kernel's own
test, with its own table, passed). The key schedule's SubWord is one
aesenclast with a zero key when the CPU has AES instructions (a block of
four copies of the word; it was the bitsliced S-box): a key's setup
0.455 (0.447 to 0.457) of its time, seal with a key a record 0.743
(0.733 to 0.753). `xo bench --ab` on the i7-9750H, A Go 1.26.1's twin
(its seal, open and key make the key each record as Xo's loop does,
since this commit), B Xo, N=20000 records of 16 KiB, 11 rounds: seal1
wall 1.089 (1.077 to 1.097), open1 1.021 (1.004 to 1.04), from 3.671
and 3.486; seal 1.133 (1.114 to 1.153), open 1.148 (1.13 to 1.165); a
key and an empty seal 1.276 (1.252 to 1.291).
## 2026-10-27: perf: ChaCha20-Poly1305 on amd64 in one loop, AVX2 or SSE, with 64 bit Poly1305 limbs (seal 3.10 to 1.25 times Go's time)

- Kind: perf
- Commit: this commit
- Found by: Xo Lang 4's amd64 parity audit (bench/results/amd64-chacha-2026-10-27.md)
- Go reference: golang.org/x/crypto/chacha20poly1305 chacha20poly1305_amd64.s (the SSE and AVX2 loops, polyAdd, polyMul, polyMulReduce); golang.org/x/crypto/internal/poly1305 sum_generic.go (64 bit limbs)
- Test: internal/llvm golden crypto_prims (RFC 8439's vectors with A.3's Poly1305 edge cases) built for darwin/amd64 native and LLVM and run on an i7-9750H; the chacha twin's checksums equal Go's (seal, open, xor, poly at N=1, 7, 64) on both backends; the same with HW_AVX2 masked off (the SSE kernel) and with HW_AVX2 and HW_SSSE3 off (the portable path); internal/llvm TestTLSDiff and the arm64 crypto goldens (donna's path); xo dev codegen test/codegen (the kernels' patterns fail on the parent); internal/repotest TestX86SIMDGated

On amd64, seal and open hash the additional data first and then run
ChaCha20 and Poly1305 over the bulk in one loop (cpu.chacha_poly_seal4
and open4 with SSSE3, seal8 and open8 with AVX2; compiler/ssa
x86chachapoly.xo and x86chachapoly8.xo): a Poly1305 block's scalar
multiplies after each double round of the vector rounds, sealing hashing
the previous 256 bytes, opening the bytes it decrypts. Poly1305 there
has 64 bit limbs (cpu.poly_blocks, after Go's sum_generic.go: four 64 by
64 bit products a block where donna's 26 bit limbs took 25 for two):
alone 0.656 of the time (1.227 of Go's, from 1.788). Every other target
keeps donna's code: on arm64 the 64 bit limbs took 1.174 (1.168 to 1.18)
times its time with 0.667 of its instructions
(gotchas/poly1305-64bit-limbs-slower-on-apple-arm64.md). The AVX2 kernel
holds two blocks a ymm register and runs without legacy SSE (its
constants in 256 bytes after the state, read as VEX operands). `xo bench
--ab` on the i7-9750H, N=20000 records of 16 KiB, 11 rounds: SSE fused
against the unfused SSSE3 kernel, seal 0.639, open 0.636; AVX2 against
SSE fused, seal 0.638 (0.63 to 0.649), open 0.641; against Go 1.26.1,
seal 1.248 (1.239 to 1.27), open 1.29.

## 2026-10-27: perf: ChaCha20 on amd64 with SSSE3, two blocks' rows at a time (ChaCha20 0.382 of the time, ChaCha20-Poly1305 0.519)

- Kind: perf
- Commit: this commit
- Found by: Xo Lang 4's amd64 parity audit (bench/results/amd64-chacha-2026-10-27.md)
- Go reference: golang.org/x/crypto/chacha20poly1305 chacha20poly1305_amd64.s (the SSE path's horizontal layout)
- Test: internal/llvm golden chacha_blocks4 (Go's output) built for darwin/amd64 native and LLVM and run on an i7-9750H; the chacha twin's checksums equal Go's; xo dev codegen test/codegen (intrinsics: chacha.xor_stream's pshufb and pshufd, which fail without the lowering); Docker amd64 goldens in the gate

cpu.chacha_blocks4 and chacha_stream4, NEON-only until now, are lowered on
amd64 (compiler/ssa x86chacha.xo; the native backend for both, LLVM's own
vectors for blocks4 and the same lines as inline asm for stream4): a
block's four rows in four xmm registers, a half round's quarter rounds on
the four columns at once, pshufd bringing the diagonals into columns,
pshufb rotating by 16 and 8, two blocks interleaved. It runs when the CPU has SSSE3 and SSE4.1
(cpu.HW_SSSE3, CPUID leaf 1; x86-64's baseline is SSE2 only, as Go's
GOAMD64=v1, and Go's own SSE kernel checks SSSE3 too), else the portable
block. `xo bench --ab` on
the i7-9750H, A the portable body (with its locals), B this, N=20000
records of 16 KiB, 11 rounds: ChaCha20 0.382 (0.379 to 0.387),
ChaCha20-Poly1305 seal 0.519 (0.517 to 0.524); against Go 1.26.1,
ChaCha20 0.486 (x/crypto's ChaCha20 is Go code on amd64) and seal 3.098
(Go's AVX2 kernel with Poly1305 interleaved).

## 2026-10-27: perf: ChaCha20's portable block keeps its words in locals (amd64 ChaCha20 4.1 times faster, ChaCha20-Poly1305 3.4 times)

- Kind: perf
- Commit: this commit
- Found by: Xo Lang 4's amd64 parity audit (bench/results/amd64-crypto-vs-go-2026-10-27.md: ChaCha20 alone 5.2 times Go's scalar Go code)
- Go reference: golang.org/x/crypto/chacha20's generic block (the sixteen words as locals)
- Test: internal/llvm golden chacha_blocks4 (its want.txt is Go's ChaCha20) built for darwin/amd64 and run on an i7-9750H, where the portable body runs; the chacha twin's checksums equal Go's (seal, open, xor, poly); arm64 cpu_prims, crypto_prims and chacha_blocks4 goldens; wasm and Docker amd64 goldens in the gate

cpu.chacha_blocks4's portable body, which amd64, LLVM on x86-64 and wasm
run (arm64 has NEON), made two Lists a 64 byte block (the sixteen words,
then a copy on the first write) and indexed them, with bounds checks, in
every quarter round. The words are locals now and the quarter round
returns its four words; the key stream is xored 8 bytes at a time.
`xo bench --ab` on the i7-9750H, N=20000 records of 16 KiB, 11 rounds:
A the parent, B this: ChaCha20 alone (xor) 0.244 (0.243 to 0.246),
ChaCha20-Poly1305 seal 0.293 (0.292 to 0.294); against Go 1.26.1's twin,
xor 1.259 (from 5.172) and seal 5.918 (from 20.39; Go's chacha20poly1305
uses AVX2 on amd64).

## 2026-10-24: perf: RSA's Montgomery loops carry byte offsets (native sign4096 0.922 of the instructions, sign2048 0.883)

- Kind: perf
- Commit: this commit
- Found by: Xo Lang 1 (native RSA at 1.13 times LLVM's instructions: mac_row's prologue and the row loops computed `8 * i` and `len % 4` checked)
- Go reference: none (internal arithmetic; Go's own `8 * i` wraps)
- Test: internal/llvm TestRSADiff and TestConstantTimeLLVM; tls_rsa, crypto_keys, tls12_server, tls13_server, tls_x509, http_tls_keys goldens (arm64, LLVM with Go parity); xo dev codegen test/codegen (consttime); signatures unchanged (bench/perf/rsa checks each against Go's)

bigmod's mac_row computed its two loop bounds as `8 * (len - len % 4)` and
`8 * len`, checked multiplies and a signed remainder on every call (64 calls
a multiply at 4096 bits, rows down to one word in square_at), and prod_at,
square_at, final_sub, add_at, sub_at and below_at scaled `8 * i` with a
checked multiply per word. They now carry byte offsets (`wrapping_add`,
`wrapping_shl(3)`, a mask for the four word bound); the lengths are never
negative. Quiet `xo bench --ab` (A the parent, N=30): sign2048 wall 0.927 (0.921 to 0.94), instructions 0.883; sign4096 wall 0.946 (0.943 to 0.95), instructions 0.922; verify2048 wall within noise, instructions 0.953. Against LLVM
the instructions went from 0.869 to 0.942 (sign4096) and 0.982 (sign2048).

## 2026-10-22: security: RSA's private exponentiation checked free of selects again

- Kind: security
- Commit: 74eb6a305
- Found by: TestConstantTimeLLVM (full) failing on the Mac, Xo Lang 4's rerun of the 2026-10-22 full tier
- Go reference: none (crypto/internal/fips140/bigmod's Nat.Exp is the same fixed window under masks)
- Test: internal/llvm TestConstantTimeLLVM; xo dev codegen test/codegen (consttime, arm64 and LLVM); TestRSADiff; tls_rsa, crypto_keys, tls12_server, tls13_server, tls_x509, http_tls_keys goldens (arm64, LLVM)

bigmod.exp's window table, accumulator and selection now live in
exp_into, which allocates nothing: clang inlined the lists' allocation
into exp, whose capacity (at least 4, a public length) became a
conditional select that the constant time checks cannot tell from one on
the secret exponent. No secret-dependent select was found; the checks
pass again and RSA's output is unchanged.

## 2026-10-20: security: hedged ECDSA nonces, the TLS server's signatures too

- Kind: security
- Commit: 002ef1c4
- Found by: review (Xo Lang lead, decision 0285)
- Go reference: crypto/internal/fips140/ecdsa Sign and hmacdrbg.go (go1.26.1)
- Test: std/native/crypto_test.xo (Go's signatures for given entropy, byte for byte; an all-zero entropy never repeats r); internal/llvm/testdata/golden/crypto_keys

ECDSA signing (std/native/llrt/nistec.xo `sign`, which the TLS server's
CertificateVerify uses) took its nonce from the system's generator alone;
a weak or repeating generator could leak the private key. The nonce now
comes from Go's hedged construction, an HMAC_DRBG over SHA-512 seeded
with the generator's bytes, the private key, and the message hash.

## 2026-10-20: feature: public key signatures, keys, PEM, and JWK

- Kind: feature
- Commit: 9559946d to 9943f3c4
- Found by: survey
- Go reference: crypto/rsa, crypto/ecdsa, crypto/ed25519, crypto/x509 (key parsing), encoding/pem (go1.26.1); RFC 7517, RFC 8037
- Test: std/native/crypto_test.xo (pem_test.go's inputs and random ones; 610 public and 531 private keys, generated, altered, and mutated; Go's signatures and 706 verdicts); internal/llvm/testdata/golden/crypto_keys; bench/ct

Decision 0285: RSA PKCS #1 v1.5 and PSS, ECDSA on P-224 to P-521 (DER
and the JWS raw form), and Ed25519 signing and verifying; PKIX, PKCS #1,
PKCS #8, and SEC 1 keys with Go's checks and texts; PEM; JWK public keys;
ECDSA and Ed25519 key generation, on every backend.

## 2026-10-20: feature: Hash, digest, hmac, and a streaming Hasher

- Kind: feature
- Commit: 4c5b0df2
- Found by: survey
- Go reference: crypto/md5/md5.go and md5block.go (go1.26.1), crypto/hmac/hmac.go
- Test: std/native/crypto_test.xo (160 Go digest and HMAC cases); std/native/llrt/md5_test.xo (Go's md5_test.go table); internal/llvm/testdata/golden/crypto_hash

MD5, SHA-1, SHA-224, SHA-256, SHA-384, SHA-512, SHA3-256, and SHA3-512
digests, HMAC over any of them, and a Hasher value for data that
arrives in pieces, with Go's results on every backend. MD5 is ported
from Go; the others are the native TLS stack's own (SHA-224 added).
