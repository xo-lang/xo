# std/tls changes

## 2026-10-22: speedup: AES-GCM rounds four blocks at once, TLS 1.2 records sealed in one string

- Kind: speedup
- Commit: this commit
- Found by: tlsrate bulk on main 35d244cce (quiet Docker linux/arm64): TLS 1.3 AES-GCM 0.50 of Go, TLS 1.2 AES-GCM 0.35 (arm64) and 0.41 (LLVM) (Xo Lang 3; bench/perf/aesgcm splits it)
- Go reference: crypto/aes and crypto/cipher's GCM (assembly, CTR and GHASH fused)
- Test: tls_test (seal and open over record sizes), tls12_client, http_tls and crypto_prims goldens (arm64, LLVM with Go parity); TestTLSDiff, TestCryptoXoDiff, TestConstantTimeLLVM

`enc_at` calls `cpu.aes_encNNx4` (Xo Lang 1, 8e0a72d4c) for a full chunk of
four counter blocks: four independent AESE chains instead of one
dependent chain four times. TLS 1.2 `seal` writes the header, explicit
nonce, ciphertext and tag into one string (`Gcm.seal_into_aad`, before:
`Gcm.seal`, `cat2` of the nonce and `asn1.cat` of the record, two 16 KiB
copies) and returns a lone record as is; `tls12srv.send` skips the
`cat2` onto an empty out buffer. `xo bench --ab` on the Mac (B / A):
bench/perf/aesgcm seal wall 0.879, open 0.874 (instructions 0.90); TLS
1.2 AES-GCM 256 MiB bulk through the tlsrate server (wall, 10 rounds)
0.939. Still 3 to 4x Go's instruction count per record: GHASH (scalar
carry-less multiplies through general registers) and the counter and xor
loop are next (a fused NEON kernel).
bench/results/aesgcm-2026-10-22.md.

## 2026-10-22: speedup: Poly1305 absorbs two blocks per step

- Kind: speedup
- Commit: 83af5520d
- Found by: tlsrate bulk TLS 1.2 ChaCha20-Poly1305 at 0.73 of Go (Xo Lang 3; bench/perf/chacha splits it)
- Go reference: golang.org/x/crypto/poly1305 (64 bit limbs); the limbs stay poly1305-donna's 26 bit
- Test: crypto_prims (RFC 8439 vectors, every length to 200), chacha_blocks4, tls12_client goldens (arm64, LLVM with Go parity); TestCryptoXoDiff, TestConstantTimeLLVM, TestTLSDiff; test/codegen intrinsics

Poly1305 had Go's instruction count but ran 1.5x slower: each block
waited for the last one's multiply and carries. `absorb` now takes two
blocks per step, h = (h + m0) r^2 + m1 r (r^2 computed in `poly_new`),
and one block for an odd tail. Constant time as before (no branch on
data). bench/perf/chacha, 16 KiB records, `xo bench --ab` on the Mac
(B / A wall): seal 0.836, open 0.844, poly1305 alone 0.571; Linux
arm64: poly1305 0.126 s to 0.072 s (Go 0.079), seal 0.337 to 0.292 s
(Go 0.258). TLS 1.2 ChaCha bulk 0.73 before, 0.82 to 0.86 after (loaded).
bench/results/chacha-poly1305-2026-10-22.md.

## 2026-10-22: perf: AES-GCM goes back to its own GHASH body (cpu.ghash4 was slower in the loop)

- Kind: perf
- Commit: 344764528
- Found by: tlsrate bulk (Xo Lang 3, quiet Docker linux/arm64)
- Go reference: none
- Test: std/native/llrt/tlsrec_test.xo; tls, crypto goldens (arm64)

The change that called cpu.ghash4 from ghash_hw and ctr_at_a (b9092b2b6)
cut the instruction count 12.6% (callgrind, 16 MiB TLS 1.3 download) but
cut throughput 24% (bulk TLS 1.3, 256 MiB, two interleaved rounds:
1830 and 1858 MBps before, 1398 and 1405 after). It is reverted: the
inlined body is back in the fused loop. cpu.ghash4 stays in cpu.xo.

## 2026-10-22: speedup: a server read forgets stale readiness first

- Kind: speedup
- Commit: c30b3ddf5
- Found by: strace per request (Xo Lang 3, HTTP/2 one stream: 1.8 EAGAIN reads a request against Go's 1.1)
- Go reference: none (Go's netFD reads first and waits on EAGAIN)
- Test: tls, tls12, tls13, http_tls, websocket_tls, http2, crypto goldens (arm64, LLVM); wasm TLS and Map goldens

The edge that announced data stays set in the descriptor until a wait
consumes it, so a read that had already taken the data left a flag that
made the next wait return at once for a read that found nothing. fill
now clears the flag before its read (an edge after the clear keeps it).
strace, linux/arm64, HTTP/2 one stream, tlsrate: syscalls per request
10.5 to 8.4, reads 2.9 to 2.0 (EAGAIN 1.8 to 1.1), epoll_pwait 2.9 to
2.5, futex 3.6 to 3.0.

## 2026-10-22: speedup: GHASH of a 64 byte chunk written out in the fused CTR loop

- Kind: speedup
- Commit: d38115df3
- Found by: profile (Xo Lang 3, bulk TLS 1.3 download: aesgcm.ghash64 26% of server CPU, called once per chunk)
- Go reference: none (call overhead only)
- Test: std/native/llrt/tlsrec_test.xo; tls, tls12, tls13, http_tls, websocket_tls, crypto goldens (arm64, LLVM); wasm TLS and Map goldens

ghash64's body is now in ctr_at_a's loop (the inliner leaves the call: the
function is too big), so a chunk no longer pays a call and a returned
pair. valgrind callgrind, linux/arm64, 16 MiB TLS 1.3 download: 142.89 M
to 141.34 M instructions (-1.1%); ghash64 is 25.9% of them, so the
four-blocks-a-reduction body is where the rest is (a NEON multiply is the
next step).

## 2026-10-22: speedup: AES-GCM response records sealed from the caller's bytes, GHASH fused with the CTR pass

- Kind: speedup
- Commit: 93f258200
- Found by: profile (Xo Lang 3, bulk TLS 1.3 download: libcstr.copy_words 10%, CTR and GHASH as two passes)
- Go reference: crypto/internal/fips140/aes/gcm gcm_arm64.s (gcmAesEnc: one pass over each block)
- Test: std/native/llrt/tlsrec_test.xo (seal_at equals seal for sizes around the 64 byte chunks, the record opens); tls, tls12, tls13, http_tls, websocket_tls, crypto goldens (arm64, LLVM); wasm TLS and Map goldens

A TLS 1.3 AES-GCM response record was copied into an inner plaintext
string, encrypted into the record, then hashed in a second pass. It is now
encrypted straight from the caller's bytes (no copy) and each 64 byte
chunk of ciphertext is hashed right after it is written, while it is in
cache. Server CPU samples of a 3 x 256 MiB bulk download in Docker
(linux/arm64, loaded machine): 703 ms to 508 ms.

## 2026-10-22: speedup: ML-KEM's NTT loops in NEON on arm64

- Kind: speedup
- Commit: 177602f83
- Found by: decision 0336's alternatives (Xo Lang 4)
- Go reference: none (Go's crypto/mlkem is scalar Go)
- Test: crypto_mlkem, tls13_server, tls13_go, tls_client, tls_mlkem_off, tls13_rfc8448, tls12_server, cpu_prims goldens (arm64 and LLVM on the Mac); compiler/macho tests of the new encodings

The X25519MLKEM768 key share's NTT, inverse NTT and NTT domain products
run four coefficients a NEON register on arm64 (both backends; decision
0336's addendum). The crypto_mlkem golden on the Mac: native
793,576,441 to 634,053,655 instructions, LLVM 849,332,618 to
710,645,372, output identical.

## 2026-10-22: speedup: ML-KEM's NTT loops in SSE2 on x86-64

- Kind: speedup
- Commit: 56e4f2338
- Found by: profile (Xo Lang 4, tls-newconn amd64 Docker callgrind per function)
- Go reference: none (Go's crypto/mlkem is scalar Go)
- Test: crypto_mlkem, tls13_server, tls13_go, tls_client, tls_mlkem_off goldens (arm64 and LLVM on the Mac, native amd64 in Docker)

The X25519MLKEM768 key share's NTT, inverse NTT and NTT domain products
ran as scalar loops on x86-64. Decision 0336: two coefficients an SSE2
register on both x86-64 backends. A tls-newconn handshake per request on
linux/amd64: native 3,202,802 to 3,115,327 instructions, the three loops
43% fewer; arm64 and wasm run the same portable code as before.

## 2026-10-21: speedup: ChaCha20 four blocks at a time in vector registers

- Kind: speedup
- Commit: 7f6e6c203
- Found by: profile (Xo Lang 3, bench/results/tls-h2-vs-go-2026-10-21.md)
- Go reference: golang.org/x/crypto/chacha20 chacha_arm64.s (four blocks in NEON)
- Test: internal/llvm/testdata/golden/chacha_blocks4 (every length class around 256 bytes, against Go's ChaCha20); crypto_prims, tls12_server, tls12_client, tls13 goldens

TLS ChaCha20-Poly1305 records computed one 64 byte block at a time in
scalar code. Decision 0329: four blocks (256 bytes) at a time in vector
registers (NEON on arm64, LLVM vectors), written straight into the
record's buffer. ChaCha20 itself takes a quarter of the instructions on
arm64 (2,000 x 16 KB: 837 to 197 million native, 881 to 192 million
LLVM); amd64 and wasm run the same portable code as before.

## 2026-10-21: speedup: a TLS client handshake on macOS without a zone lookup or slow symbol lookups

- Kind: speedup
- Commit: 7b590102
- Found by: Xo lang 4's profile of the arm64 TLS client (open and readlink in every handshake)
- Go reference: none
- Test: internal/llvm/testdata/golden/tls_client, tls13_go, tls12_client, http_tls (the expired certificate texts in time.Local); a local Go server with a self-signed certificate, 500 dials verified by the system (Mac)

Every client handshake (std/tls and std/http's https) read the
/etc/localtime link and looked up the local zone, which only a failed
verification's message uses; it is found now when that message is made.
On macOS the Security framework's functions are looked up through its
handle, opened once, where each lookup searched every loaded image (40
to 51 us a symbol, a dozen a handshake). 500 dials whose verification
fails on the Mac: 506 to 154 ms of CPU, 1,570 to 394 ms of wall time.

## 2026-10-20: feature: a TLS client on a dialed connection

- Kind: feature
- Commit: dfc2ef4e
- Found by: survey
- Go reference: crypto/tls Client, Config.InsecureSkipVerify and VerifyPeerCertificate; crypto/x509 CertPool.AppendCertsFromPEM (go1.26.1)
- Test: internal/llvm/testdata/golden/tls_client (arm64, LLVM, and Go agree); std/native/tls_test.xo; cmd/xo TestVetVerifyNone; scripts/pgtls/run.sh (postgres:16 in Docker: verify-full, verify-ca, require; arm64 and Go agree)

`tls.client(conn, host)` upgrades a connection the program dialed to TLS
(Postgres's SSLRequest, SMTP STARTTLS, Redis), with `Verify.Full` (chain
and host name), `ChainOnly` (libpq's verify-ca), or `None` (encryption
only, reported by `xo vet` as XO0902), and private roots with
`tls.roots(pem)`.
