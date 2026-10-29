# std/crypto changes

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
