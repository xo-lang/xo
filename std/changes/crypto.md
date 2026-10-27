# std/crypto changes

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
