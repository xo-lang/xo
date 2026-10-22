# std/tls changes

## 2026-10-22: perf: AES-GCM goes back to its own GHASH body (cpu.ghash4 was slower in the loop)

- Kind: perf
- Commit: this commit
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
