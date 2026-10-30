# std/tls changes

## 2026-10-30: feature: TlsConn deadlines

- Kind: feature
- Commit: this commit
- Found by: the backend gap survey of 2026-10-30, gap 5; decision 0430
- Go reference: crypto/tls Conn.SetDeadline, SetReadDeadline, SetWriteDeadline, and Handshake under the conn's deadline
- Test: internal/llvm/testdata/golden/tls_deadline (arm64, LLVM, Go parity): a read past its deadline, then the request goes through; a passed write deadline, and the next write failing too; a handshake against a silent server limited by the Conn's deadline
- Timing: none of its own: the waits are the connection's (prelude.md's conn deadline row); tlsio reads two words per wait

`set_deadline`, `set_read_deadline` and `set_write_deadline` on a
TlsConn set its connection's deadlines. Natively tlsio's waits read the
connection's deadline words at each wait, so one set during a wait
applies, and the bytes of a partly read record stay in the TLS state
across a timeout. A failed write, a timeout included, fails every later
write, as Go's tls.Conn: part of a record may have gone out. The Go
backend sets net.Conn's deadlines under tls.Conn and keeps them across
its own cancellation.


## 2026-10-29: feature: peer_certificates and roots_of

- Kind: feature
- Commit: this commit
- Found by: milestone 0.35.0 S5 (std/x509)
- Go reference: crypto/tls ConnectionState.PeerCertificates, x509.CertPool.AddCert (go1.26.1)
- Test: golden tls_peer_certs (a local std/http server's chain as the client sees it, roots_of trusting its CA, and std/x509's verify of the chain)

`TlsConn.peer_certificates()` is the server's chain as it sent it, leaf
first, as std/x509 Certificates; `roots_of(certs)` makes Roots of parsed
certificates, as `roots(pem)` does of a PEM text.

## 2026-10-23: fix: a server read that ends the handshake and cuts a record no longer loses the record's head

- Kind: fix
- Commit: this commit
- Found by: the `http_stream_body_h2` golden hanging in about 1 run in 300 under load (Xo Lang 3's stress loop, ciphertext and record logging)
- Go reference: crypto/tls Conn.readRecord (bytes after the Finished stay in the record buffer)
- Test: golden http_stream_body_h2 (20000 runs, 4 at a time: 66 hangs before, 0 after)
- Regression test: golden tls_coalesced_finish (a proxy joins the client's Finished and its first data in one write, 40 connections: the server before the fix timed out on 3 of 40, after it answers all)

When the client's Finished and its first application data arrived in one
socket read, the server finished the handshake inside that read.
tls13srv.feed and tls12srv.feed keep the head of a record the read cut
in their `rin`, and the server's `pend` was cleared, so after the handshake
the next read's bytes were joined to a record's tail: the next record
header was garbage, no record was complete, and the connection waited
for data that was already read (h2 streamed bodies stalled after the first
64 KiB). The leftover now moves into `pend` once the handshake is done
(tlssrv.take_rin; also tls13srv.take_rin and tls12srv.take_rin).

## 2026-10-23: behavior: TlsErr prints its message (decision 0358)

- Kind: behavior
- Commit: this commit
- Found by: the Intel field report on 0.26.0 (error texts nested Debug output)
- Go reference: net/http, net/smtp and text/template error texts
- Test: golden error_text_http (arm64, LLVM, Go parity)

`Display` of `TlsErr` printed `TlsErr(msg: "...")`; it prints the message.
`Debug` and value comparison are unchanged.

## 2026-10-22: speedup: ChaCha20's NEON kernel loops over the record

- Kind: speedup
- Commit: (this commit)
- Found by: Xo Lang 3's bench/perf/chacha against golang.org/x/crypto (bench/results/chacha-poly1305-2026-10-22.md: xor 1.23x Go)
- Go reference: golang.org/x/crypto/chacha20 chacha_arm64.s xorKeyStreamVX (one loop over every 256 bytes, the counters advanced in a register)
- Test: chacha_blocks4, crypto_prims, tls12_client, tls12_server, tls13 goldens (arm64, LLVM with Go parity), TestConstantTimeLLVM, test/codegen

ChaCha20 called the four block kernel once per 256 bytes, storing the
four counters into its state first. cpu.chacha_stream4 runs the same
kernel as one loop over the whole run of 256 byte groups, the counters
advanced with a vector add (decision 0329 addendum). Docker linux/arm64,
`xo bench --ab`, 20,000 records of 16 KiB: ChaCha20 alone 0.888 of
main's time (1.13x Go's before, 1.005x after), seal 0.919. amd64 and wasm
run the portable body as before.

## 2026-10-22: speedup: AES-GCM records eight blocks a round on arm64

- Kind: speedup
- Commit: 386339233 (the kernel 560fdacae before it)
- Found by: Xo Lang 3's bench/perf/aesgcm against Go's crypto/cipher (Go 1.73x seal, 1.83x open after the four block kernel)
- Go reference: crypto/internal/fips140/aes/gcm/gcm_arm64.s gcmAesEnc and gcmAesDec (round keys in registers, eight blocks, Karatsuba, one reduction per 128 bytes)
- Test: cpu_prims golden (672 aes_gcm8 lines against the portable body and Go's reference), tls*, crypto* goldens (arm64, LLVM with Go parity), TestCryptoXoDiff, TestTLSDiff, TestConstantTimeLLVM

The AES-GCM key now holds H^8 to H (`Gcm.hk`, H^8 first; the four block
kernels and GHASH read from H^4, `HK4`), and on arm64 a record's 128 byte
multiple runs through `cpu.aes_gcm8` (compiler/ssa a64aesgcm8.xo), a last
64 bytes through the four block kernel. `xo bench --ab` on the Mac (16 KiB
records, B / A): seal cpu 0.851 (0.792 to 0.913), instructions 0.918; open
cpu 0.851 (0.833 to 0.955), instructions 0.921; wall within noise (load).
Against Go's twin Xo still runs 1.58x (seal) and 1.66x (open) the
instructions, about a fifth of them the bench's new key per record
(`new_gcm`, which Go's loop does once; TLS keeps its key).

## 2026-10-22: speedup: TLS 1.2 AES-GCM records sealed in place from the caller's buffer

- Kind: speedup
- Commit: 01f86921c
- Found by: XO_PROF=cpu of the tlsrate bulk server in Docker, TLS 1.2 against TLS 1.3 (Xo Lang 3): libcstr.copy_words and asn1.cat only in the 1.2 profile, server CPU 372 against 298 us/MB
- Go reference: crypto/tls conn.go (halfConn.encrypt seals in the record buffer)
- Test: native/llrt/tls12 module test (seal_at equals seal, byte for byte, for both key sizes, 27 lengths up to 64 KiB and three sequence numbers, and the records open), tls12_client, http_tls and http_tls_keys goldens (arm64, LLVM with Go parity), TestTLSDiff

TLS 1.3's server write sealed the response from its address, but the TLS 1.2
server copied the whole write into a string, sliced it into records, and
joined the records. `tls12.seal_at` and `Gcm.seal_into_aad_at` seal each
16 KiB record straight from the caller's address (only a tail under 64 bytes
is copied), build the additional data and nonce words without the small
lists `u64` and `asn1.cat` allocated, and join the records once, as
`tlsrec.seal_at` does. `xo bench --ab` in Docker linux/arm64 (B / A, 4 GiB
TLS 1.2 download from the tlsrate server, 11 rounds, load 6 to 7): wall 0.921
(95% 0.873 to 0.932), cpu_ms 0.910 (0.794 to 0.979); a second run 0.888 and
0.822 (both within noise at that load). The Mac cannot show it: loopback
`sendto` is 84% of the server there. The rest of the gap to Go is the AES
kernel (the same in TLS 1.3) and Go's TLS 1.2 write being faster than its own
TLS 1.3 one.
bench/results/aesgcm-2026-10-22.md.

## 2026-10-22: speedup: AES-GCM seal and open on the interleaved AES and GHASH kernel

- Kind: speedup
- Commit: df2b7ce39
- Found by: bench/perf/aesgcm profile after the fused CTR kernel: GHASH 53% of seal (Xo Lang 3; kernel by Xo Lang 1, 0f5353597)
- Go reference: crypto/cipher's GCM (gcmAesEnc and gcmAesDec interleave the rounds with the products)
- Test: tls_test (seal_at over chunk boundaries), tls12_client, http_tls, http_tls_keys and crypto_prims goldens (arm64, LLVM with Go parity); TestTLSDiff, TestCryptoXoDiff, TestConstantTimeLLVM

`seal_w`, `seal_into`, `seal_into_aad`, `seal_into_at`, `open_w` and
`open_from` hand the whole 64 byte chunks of a record to
`cpu.aes_gcm_enc10/12/14` or `aes_gcm_dec10/12/14`, which run the AES
rounds of one chunk with the GHASH products of the previous one in the same
loop (open hashes the ciphertext before it stores the plaintext); the
tail (under 64 bytes, and the TLS inner plaintext's last byte) and the tag
are as before. `xo bench --ab` on the Mac (B / A), bench/perf/aesgcm 16 KiB
records, 10 rounds: seal wall 0.827 (95% 0.809 to 0.841), open wall 0.790
(0.777 to 0.797); instructions 1.063 and 1.059 (the kernel reloads the round
keys each round), cpu_ms 0.75. Go's crypto/cipher on the same loop is still
1.73x (seal) and 1.83x (open) faster in wall.
bench/results/aesgcm-2026-10-22.md.

## 2026-10-22: speedup: AES-GCM counter, rounds and xor in one fused kernel

- Kind: speedup
- Commit: a804aed61
- Found by: bench/perf/aesgcm after the fused GHASH step: the scalar counter stores and word xor around the per call AES intrinsic (Xo Lang 3; kernel by Xo Lang 1, 47312b2f7)
- Go reference: crypto/cipher's GCM (gcmAesEnc fuses CTR, AES and GHASH)
- Test: tls_test (seal_at over chunk boundaries), tls12_client, http_tls, http_tls_keys and crypto_prims goldens (arm64, LLVM with Go parity); TestTLSDiff, TestCryptoXoDiff, TestConstantTimeLLVM

`ctr_at` and `ctr_at_a` hand the whole 64 byte chunks of a record to
`cpu.aes_ctr10`, `aes_ctr12` or `aes_ctr14` in one call (counters built in
vector registers, four AES chains, the xor with the data, the round keys
held in registers); the unrolled scalar loop is removed and the tail still
goes through `enc_at`. `xo bench --ab` on the Mac (B / A), bench/perf/aesgcm
16 KiB records: seal wall 0.935 (95% 0.919 to 0.940), instructions 0.755;
open wall 0.938, instructions 0.765. bench/results/aesgcm-2026-10-22.md.

## 2026-10-22: speedup: AES-GCM hashes with the fused GHASH loop and unrolls its whole chunks

- Kind: speedup
- Commit: ba0e97a7c
- Found by: bench/perf/aesgcm profile after the four-block rounds: ghash 37%, the counter and xor loop 36% (Xo Lang 3)
- Go reference: crypto/cipher's GCM (assembly)
- Test: tls_test (seal_at over chunk boundaries), tls12_client, http_tls and crypto_prims goldens (arm64, LLVM with Go parity); TestTLSDiff, TestCryptoXoDiff, TestConstantTimeLLVM

`ghash_hw` hashes the multiple of 64 bytes with `cpu.ghash_blocks` (Xo
Lang 1, 7fcaa337b: one NEON loop, state and H powers in vector registers)
instead of a `ghash64` call per chunk, which is removed; TLS 1.3's seal no
longer hashes each chunk inside the CTR loop (the data is hot in L1
either way) and ghashes the record once. `ctr_at` and `ctr_at_a` write the
counter blocks and xor the eight words of a whole chunk unrolled, with
no per-word range test. `xo bench --ab` on the Mac (B / A), bench/perf/aesgcm
16 KiB records: hash step wall 0.951 (seal) and 0.957 (open),
instructions 0.85; unrolled chunks wall 0.841 and 0.821, instructions
0.71 and 0.72 (together 0.80 and 0.78 wall of the four-block step); Go's
instruction count is now 0.47 of Xo's (was 0.25).
bench/results/aesgcm-2026-10-22.md.

## 2026-10-22: speedup: AES-GCM rounds four blocks at once, TLS 1.2 records sealed in one string

- Kind: speedup
- Commit: 75dd849d6
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
