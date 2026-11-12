# std/crypto changes

## 2026-11-12: security: the RSA private operation's fault check compares in constant time

- Kind: security
- Commit: this push
- Found by: the 0616 RSA review's second reader (F7)
- Go reference: crypto/internal/fips140/rsa decrypt (the check against the public operation with bigmod's constant time Equal), go1.26.1
- Test: std/native/crypto_test.xo's RSA tests and golden crypto_rsa_encrypt (unchanged output)
- Shipped in: not yet released

The private operation checks its result by running the public operation on it; that compare used `==` on Bytes, which may stop at the first differing byte, so a fault in the CRT could take a time that depended on the secret result. It is now `ct.bytes_equal`, as Go's. The result was never released on a mismatch.

## 2026-11-12: fix: encrypt_pkcs1v15 refuses a small key before a long message

- Kind: fix
- Commit: this push
- Found by: the 0616 RSA review's second reader (F11)
- Go reference: crypto/rsa EncryptPKCS1v15 (checkPublicKeySize, then ErrMessageTooLong), go1.26.1
- Test: std/native/crypto_test.xo "rsa: a key below 1024 bits parses but every operation refuses it" (native, LLVM); internal/stdtest TestRsaFuzz now also tries every ciphertext length from 0 to k + 2 (F4)
- Shipped in: not yet released

`crypto.encrypt_pkcs1v15` with a key below 1024 bits and a message too long for it returned "message too long"; it now returns the insecure key error, as Go and as `encrypt_oaep` do.

## 2026-11-12: security: secret key operations under Arm's DIT

- Kind: security
- Commit: this push
- Found by: the 0616 RSA review's dudect of the padding checks alone (std/native/llrt/rsa_test.xo): LLVM's -O0 test build leaked on Apple silicon (|t| 81 between PKCS #1 v1.5 padding classes) with no data dependent branch (gotchas/dudect-o0-llvm.md)
- Go reference: crypto/subtle WithDataIndependentTiming and crypto/internal/fips140deps/cpu (go1.24, DIT on arm64)
- Test: native/llrt/cpu_test.xo (dit_set sets and returns the bit); rsa_test.xo's dudect, 10^5 a pair on the Mac: LLVM -O0 max |t| 3.8 (was 81), native 1.9, LLVM -O2 3.1, controls seen
- Shipped in: not yet released

On arm64 CPUs with FEAT_DIT, the RSA private key operation (the CRT
exponentiation and its check) and OAEP's and PKCS #1 v1.5's padding checks
set Arm's DIT bit (cpu.dit_set through ct.dit_on and dit_off, decision
0619) and restore it after; so do X25519 (scalar_mult, base), the NIST
curves' public_key, ecdh and sign_hedged, Ed25519's public_key and sign,
and ML-KEM's encapsulation and decapsulation. A fiber is not preempted
while it holds DIT.

## 2026-11-12: feature: RSA encryption, OAEP and legacy PKCS #1 v1.5 (Go's crypto/rsa)

- Kind: feature
- Commit: this push
- Found by: the std audit of 2026-11-12 (bench/results/std-audit-2026-11-12.md, crypto/rsa's largest gap group), decision 0618 (owner, 2026-11-12: OAEP recommended, PKCS #1 v1.5 legacy, for interop only)
- Go reference: crypto/internal/fips140/rsa pkcs1v22.go (EncryptOAEP, DecryptOAEP) and crypto/rsa pkcs1v15.go (go1.26.1)
- Test: std/native/crypto_test.xo (Go's rsa_test.go OAEP vectors, RFC 8017, encrypt and decrypt; pkcs1v15_test.go decryption vectors); internal/llvm/testdata/golden/crypto_rsa_encrypt (140 ciphertexts from Go's EncryptOAEP and EncryptPKCS1v15, changed and unchanged, decrypt to Go's outcome on every backend; Go's limits; round trips)
- Shipped in: not yet released

`crypto.encrypt_oaep` and `decrypt_oaep` (RSA-OAEP, the recommended
scheme: JWE RSA-OAEP and RSA-OAEP-256, KMS key wrapping) and the legacy
`encrypt_pkcs1v15` and `decrypt_pkcs1v15`, only for systems that require
them. Decryption checks the padding in constant time (native/llrt/ct
masks) and every failure is the one `crypto/rsa: decryption error`.

## 2026-11-11: perf: AES CTR and CBC encryption through CPU kernels

- Kind: perf
- Commit: this commit
- Found by: bench/results/aesmodes-vs-go-2026-11-11.md (ctr 1.48 and cbcenc 1.13 of Go's wall time: each block's rounds reloaded the round keys)
- Go reference: crypto/internal/fips140/aes ctr_arm64.s and cbc.go (go1.26.1)
- Timing: bench/results/aesmodes-vs-go-2026-11-11.md (second and third tables): ctr 1.48 to 0.86 of Go, cbcenc 1.13 to 0.54
- Test: the aesmodes corpus gains 60 CTR rows whose counter's low 32 bits wrap inside long calls; aesmodes and aead corpora on go, arm64, llvm
- Shipped in: not yet released

`Ctr.xor` runs whole 64 byte chunks through the kernel AES-GCM uses
(round keys held in registers, four blocks interleaved) when the
counter's low 32 bits do not wrap in the call, and block by block
otherwise.
`cbc_encrypt` runs in a new arm64 kernel (`cpu.aes_cbc_enc10`, 12, 14)
that holds the round keys in registers for the whole message; other
targets run its body, one `aes_encNN` a block.

## 2026-11-11: feature: ML-KEM key encapsulation (Go's crypto/mlkem)

- Kind: feature
- Commit: this push
- Found by: the std audit of 2026-11-11 (bench/results/std-audit-2026-11-11.md, eighth gap), decision 0604
- Go reference: crypto/mlkem and crypto/mlkem/mlkemtest (go1.26.1)
- Test: the mlkem corpus on go, arm64, llvm (80 seeds, 80 encapsulations with Go's messages, 160 edited encapsulation keys, 120 tampered or cut ciphertexts, both parameter sets)
- Timing: bench/results/mlkem-vs-go-2026-11-11.md
- Shipped in: not yet released

MlkemParams (Mlkem768, Mlkem1024), MlkemDecapsulationKey and MlkemEncapsulationKey with mlkem_generate_key, mlkem_decapsulation_key, mlkem_encapsulation_key, params, bytes, encapsulation_key, decapsulate and encapsulate, Go's error texts; the native backends run llrt/mlkem.xo (TLS's hybrids), an encapsulation key parsed once (t, A and H(ek), as Go keeps them). spec/std.md 11.
## 2026-11-11: feature: the AES block and the CBC and CTR modes (Go's crypto/aes and crypto/cipher)

- Kind: feature
- Commit: this push
- Found by: the std audit of 2026-11-11 (bench/results/std-audit-2026-11-11.md, ninth gap), decision 0605
- Go reference: crypto/aes, crypto/cipher cbc.go and ctr.go (go1.26.1)
- Test: the aesmodes corpus on go, arm64, llvm (key sizes, 150 blocks, 150 CBC messages, 200 CTR streams split across calls with counters near 2^64 and 2^128, a copied Ctr)
- Timing: bench/results/aesmodes-vs-go-2026-11-11.md
- Shipped in: not yet released

AesBlock with aes_block, encrypt_block and decrypt_block; cbc_encrypt and cbc_decrypt (no padding); Ctr with ctr and xor. Go's texts for a bad key size (KeyErr) and for the faults where Go panics. The native backends run llrt/aesgcm.xo (the hardware AES of decision 0231), CTR four blocks a pass. spec/std.md 11.

## 2026-11-11: feature: crypto.xor (Go's subtle.XORBytes)

- Kind: feature
- Commit: this push
- Found by: std audit 2026-11-11 gap 20
- Go reference: crypto/subtle XORBytes (go1.26.1)
- Test: crypto corpus XORS (240 cases from Go: every length 0 to 80, then unequal lengths; go, arm64, llvm)
- Shipped in: not yet released

`crypto.xor(a, b)` returns a new Bytes of the shorter length (Go writes
into a dst it is given). Native code XORs 32 then 8 bytes at a time, then
the tail. The corpus generator also gains the ed25519 64 byte private key
check that fe1903df9 added to the generated file by hand.

## 2026-11-11: test: a Hasher copy is a clone (Go's hash.Cloner)

- Kind: test
- Commit: this push
- Found by: std audit 2026-11-11 gap 11 ("no Hasher clone")
- Go reference: hash.Cloner (Go 1.25)
- Test: golden hasher_copy_clone (arm64, llvm, Go parity, wasm)
- Shipped in: not yet released

A Hasher is a value, so a copy already goes on independently, as Go's
Clone gives; a golden now checks it for SHA-256, SHA-512, SHA3-256 and
HMAC on every backend, and spec/std.md says so. No code change.

## 2026-11-11: feature: ECDH key agreement (Go's crypto/ecdh)

- Kind: feature
- Commit: this push
- Found by: the std audit of 2026-11-11 (bench/results/std-audit-2026-11-11.md, third gap), decision 0599
- Go reference: crypto/ecdh (go1.26.1)
- Test: the ecdh corpus on go, arm64, llvm (600 private keys, 800 public keys and shared secrets against Go, X25519's low order points); golden crypto_ecdh (RFC 7748 6.1, generated keys agreeing on each curve, every error text) on llvm, Go parity and arm64; `xo test --std-module=native/crypto`
- Timing: none (llrt's X25519 and nistec, which TLS already runs)
- Shipped in: not yet released

EcdhCurve (X25519, P256, P384, P521), EcdhPrivateKey and EcdhPublicKey with ecdh_generate_key, ecdh_private_key, ecdh_public_key, bytes, curve, public_key and ecdh, Go's encodings and error texts. spec/std.md 11.

## 2026-11-11: fix: a P-256 public key with a coordinate at or above p says Go's text

- Kind: fix
- Commit: this push
- Found by: the ecdh corpus of decision 0599 (branch lang3-ecdh)
- Go reference: crypto/internal/fips140/nistec p256_asm.go (go1.26.1)
- Test: `xo test --std-module=native/crypto` ("a public key with a coordinate at or above p has Go's text", the three curves' texts from Go on arm64)
- Timing: none (an error text)
- Shipped in: not yet released

parse_public_key of a P-256 SubjectPublicKeyInfo whose x or y is at or above p failed with `invalid P256Element encoding` (fiat's text); Go on arm64 and amd64, Xo's targets, uses its assembly field, whose text is `invalid P256 element encoding`. P-384 and P-521 keep fiat's text, as in Go.

## 2026-11-11: docs: the header says what the module is for

- Kind: docs
- Commit: this commit
- Found by: pkg.xolang.org and xolang.dev showed `xo doc std`'s summary of std/crypto as a note about the file (the Xo Lead, 2026-11-11)
- Go reference: none
- Test: cmd/xo TestDocStdSummaries
- Shipped in: not yet released

The header comment of std/crypto.xo opens with a sentence that says what the module is for, which `xo doc std` lists as its summary. No declaration changed.

## 2026-11-10: refactor: width bounds as the type constants (decision 0587 point 4)

- Kind: refactor
- Commit: this commit
- Found by: decision 0587 point 4 (std's literal bounds become the type constants)
- Go reference: none (no behavior change; Go's math.MaxInt32, MaxUint16, MaxUint32)
- Test: xo test --std-module=crypto and its corpus where it has one

In std/crypto, the exponent checks (`> 2147483647`) are `> Int(Int32.max)`. No behavior change.

## 2026-11-09: perf: CBC decryption runs on the CPU's AES instructions

- Kind: perf
- Commit: this push
- Found by: bench/perf/aead cbc_big (3.66x Go's time; 79% of its profile in the bitsliced decrypt4)
- Go reference: crypto/aes (decryptBlockAsm: AESD and AESIMC on arm64, AESDEC on amd64)
- Test: cpu_prims golden's dec lines (a decrypted block is the encrypted one back, and the instruction path equals the portable body, on arm64, LLVM and the Go backend); Aes128CbcHmacSha256's vectors
- Timing: bench/perf/aead cbc_big on linux/arm64 host 1, --ab against main, 7 rounds: wall 0.321 (0.317 to 0.323, n=6; 365.2 to 117.1 ms; Go 115.5 ms on the Mac Studio run), no load. amd64 not measured (host down; ~/xo-lead/notes/deferred.md)
- Shipped in: not yet released

New prims `cpu.aes_dec10`, `aes_dec12`, `aes_dec14` (the inverse cipher over the encryption round keys; AESD and AESIMC on arm64 and LLVM, AESDEC with AESIMC on the middle keys on x86) and a portable body; `Cipher.cbc_decrypt` (and `decrypt4`) use them when the CPU has AES.

## 2026-11-09: feature: `Aead.Aes128CbcHmacSha256`, JWE's A128CBC-HS256

- Kind: feature
- Commit: this push
- Found by: Xo Azure 1 (azidentity's persistent token cache reads and writes Go's JWE files)
- Go reference: RFC 7518 5.2.3 and azidentity/cache v0.4.0 internal/aescbc (the tag is HMAC-SHA-256(aad || iv || ciphertext || AL)[:16]; azidentity passes a nil aad)
- Test: std/native/crypto_test.xo (13 seals and 14 opens from Go's oracle, RFC 7518 B.1 among them); golden aead_cbc_hmac (native, LLVM, Go backend)
- Timing: bench/perf/aead (twincheck -time 5, darwin/arm64 Mac Studio, Xo / Go): cbc_small 1.18 (358 and 304 ms), cbc_big 3.66 (422 and 116 ms). Profile of cbc_big: CBC decrypt 79% (the software bitsliced decrypt4: inv_shift_rows, inv_affine, sbox, inv_mix_columns), encrypt 12%, SHA-256 7%. Only encryption has the AES instruction path (hw_n); the next step is an aesd/aesimc (aesdec) prim like the encryption one, then a --ab on host 1
- Shipped in: not yet released

`crypto.Aead` gains `Aes128CbcHmacSha256`: a 32 byte key (the first 16 bytes the MAC key, the last 16 the AES-128 key), a 16 byte nonce (the IV), PKCS#7 padding, and a sealed message of `ciphertext || tag`. `open` checks the tag in constant time before it decrypts, and a bad tag, padding or length is the one `cipher: message authentication failed` error (an empty ciphertext and one that is not whole blocks are refused, where Go's aescbc opens the first as empty and panics on the second). `nonce_size` is 16 for it and `overhead` the maximum, 32.

## 2026-11-07: perf: an AES key's schedule does SubWord in a register

- Kind: perf
- Commit: this push
- Found by: the Xo-vs-Go survey (bench/results/xo-vs-go-survey-2026-11-07.md: aesgcm/key 1.27x Go; sub_word_hw 14.4% flat in its profile)
- Go reference: crypto/internal/fips140/aes expandKeyAsm (aes_arm64.s ks128, ks192, ks256: SubWord with AESE on a duplicated word in a vector register)
- Test: the cpu_prims golden's subword lines (the instruction, the body and a Go S-box) on arm64, LLVM and Go; cpu_prims, crypto_prims, native_crypto, crypto_keys and tls13_rfc8448 on arm64 and LLVM; wasm goldens; the amd64 sample on the linux/amd64 host (AES-NI)
- Timing: bench/perf/aesgcm on linux/arm64 host 1, --ab against main c45abe320 (7 rounds): key 0.717 (0.709 to 0.731, n=7; 8.11 to 5.82 ms), seal 0.944 (0.941 to 0.952, n=6)
- Shipped in: not yet released

The key schedule's SubWord was `cpu.aes_last` on a two word list: two 8 byte stores and a 16 byte load of
both, which store forwarding cannot serve, ten times in a chain for AES-128. The new `cpu.aes_sub_word`
(decision 0231 addendum) puts the word in a vector register's four columns and runs one AESE (arm64) or
AESENCLAST (x86-64) with a zero key. The schedule also loads the key's first 16 bytes as two words and
allocates no scratch lists. Round keys unchanged.

## 2026-11-07: perf: SHA-1 hashes many blocks in one loop

- Kind: perf
- Commit: this push
- Found by: SHA-256's blocks loop the same day (the same per-block call cost)
- Go reference: crypto/sha1 sha1block_arm64.s and sha1block_amd64.go (go1.26.1; one routine over every block)
- Test: std/native/llrt/sha1_test.xo (compress_blocks against compress at 0 to 8 blocks, three offsets, two states; a Hasher written in pieces against one write); crypto and pkcs12 tests on arm64, LLVM and amd64 (SHA-NI natively); sha and crypto goldens on arm64, LLVM and wasm
- Timing: bench/perf/simdscan sha1, linux/arm64 host 1, xo bench --ab against main: 0.884 (0.873 to 0.899, n=6; better); bench/perf/sha1 0.977 (0.973 to 0.984, n=6; better); linux/amd64 (SHA-NI) simdscan sha1 74 to 64 ms, 0.86, by medians of 9 alternating runs
- Shipped in: not yet released

A run of whole blocks (sha1 sums, a Hasher's write, HMAC-SHA1) goes to cpu.sha1_blocks: on arm64 and x86-64 (native and LLVM) one loop with the state in vector registers, where each block was a call that saved the vector registers and packed and unpacked the state; wasm and an older xo a block at a time as before. A write of one block (an HMAC pad) calls the one-block routine as before: the loop cost a call there (perf/sha1 was 1.022 of main before this).

## 2026-11-07: perf: SHA-512 hashes many blocks in one loop on arm64

- Kind: perf
- Commit: 5dc30fa68
- Go reference: crypto/sha512 sha512block_arm64.s (go1.26.1; one routine over every block)
- Test: std/native/llrt/sha256_test.xo (blocks512 against block512 at 0 to 8 blocks and three offsets; a SHA-512 Hasher written in pieces against one write); crypto tests; sha and crypto goldens on arm64, LLVM and wasm
- Timing: bench/perf/simdscan sha512, linux/arm64 host 1, xo bench --ab against main: 0.965 (0.958 to 0.976, n=6; better); simdscan sha256 1.006 (0.972 to 1.022, within noise); HMAC-SHA256 of 32 bytes (simdscan hmac256) 1.002 (0.99 to 1.014, n=7; within noise)
- Shipped in: 0.44.0

A run of whole blocks (crypto's sha512 and sha384 sums, a Hasher's write, HMAC-SHA512) goes to cpu.sha512_blocks: on arm64 (native and LLVM) one loop with the state in four vector registers; elsewhere a block at a time as before. A write of a single block, SHA-256's too, calls the one-block routine as before the SHA-256 loop: going through the loop for one block cost a call (HMAC-SHA1 was 1.022 of main).

## 2026-11-07: perf: Ed25519 hashes with the SHA512 instructions

- Kind: perf
- Commit: 5dc30fa68
- Found by: reading digest.xo sha512_at while adding the SHA-512 loop: it ran the rounds in Xo (digest_tables) even when the CPU has SHA512
- Go reference: crypto/ed25519 through crypto/sha512 (go1.26.1)
- Test: crypto tests (Ed25519 sign and verify vectors) on arm64 and LLVM; ed25519 goldens
- Timing: Ed25519 sign, linux/arm64 host 1, xo bench --ab against main: 0.985 (0.972 to 0.993, n=6; better); verify 0.999 (0.994 to 1.005, within noise)
- Shipped in: 0.44.0

sha512_at, the hash under Ed25519's sign and verify, calls cpu.sha512_block when cpu.hw() has HW_SHA512 and runs the Xo rounds otherwise.

## 2026-11-07: perf: SHA-256 hashes many blocks in one loop on arm64

- Kind: perf
- Commit: this push
- Found by: the SIMD survey of std's byte paths (bench/results/simd-survey-2026-11-06.md: sha256 1.25 of Go, block256 91% of the profile)
- Go reference: crypto/sha256 sha256block_arm64.s (go1.26.1; one routine over every block, the state in registers)
- Test: std/native/llrt/sha256_test.xo (blocks256 against block256 at 0 to 8 blocks and three offsets; a Hasher written in pieces against one write); crypto tests; sha and crypto goldens on arm64, LLVM and wasm
- Timing: bench/perf/simdscan sha256, linux/arm64 host 1, xo bench --ab against main: 0.884 (0.871 to 0.894, n=6; better); linux/amd64 (SHA-NI) 96 to 76 ms, 0.79, by medians of 9 alternating runs
- Shipped in: not yet released

A run of whole blocks (crypto's sha256 and sha224 sums, a Hasher's write, HMAC) goes to cpu.sha256_blocks: on arm64 (native and LLVM) one loop with the state in two vector registers, where each block was a call that saved five vector registers and packed and unpacked the state; elsewhere a block at a time as before.

## 2026-11-05: feature: crypto.generate_key makes RSA keys

- Kind: feature
- Commit: this push
- Found by: the Azure SDK embedding a test key until RSA generation landed (lead's queue, 2026-11-05)
- Go reference: crypto/internal/fips140/rsa keygen.go (go1.26.1): GenerateKey, randomPrime, isPrime (trial division by 255 small primes, then BoringSSL's number of Miller-Rabin rounds), totient (lcm(p - 1, q - 1), a gcd above 32 bits retried), d = e^-1 mod lambda
- Test: std/native/llrt/rsagen_test.xo (Mersenne and large primes, Carmichael numbers and strong pseudoprimes, the gcd and short division), crypto_test.xo (keys of 1024, 1031 and 2048 bits from a seeded stream, pinned from the Go backend and identical on native, LLVM and Go; sign and verify with PKCS #1 v1.5 and PSS; the size refusals) and golden crypto_keys (every backend)
- Timing: Mac arm64, 40 random keys per run, two runs each (the time depends on the primes found): 1024 bits Go 210 and 214 ms, Xo 170 and 168 ms (0.80 of Go); 2048 bits Go 1.54 and 1.33 s, Xo 1.42 and 1.09 s (0.74 to 0.92 of Go)
- Shipped in: not yet released

`crypto.generate_key(r, Rsa, bits = 2048)` returns an RSA key of 1024 to 8192 bits with e = 65537; a size below 1024 is Go 1.26's `crypto/rsa: N-bit keys are insecure` and one above 8192 is `crypto/rsa: N-bit keys are too large` (the largest key std signs with). The prime candidates are read from `r` as Go's `randomPrime` does (top two bits and low bit set), so a seeded `Rand` gives the same key on every backend; the Miller-Rabin bases come from the system's random bytes. The generated key passes the checks a parsed one does. The ECDSA and Ed25519 cases ignore `bits`. `x509.self_signed` and the other callers are unchanged (they use ECDSA).

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
