# amd64 crypto throughput against Go (2026-10-27)

Milestone 0.33.0 criterion 2, gap 3 of
[amd64-parity-by-target-2026-10-27.md](amd64-parity-by-target-2026-10-27.md):
the wide crypto intrinsics (aes x4, aes_ctr, aes_gcm, chacha4, ghash4)
are NEON-only, so every amd64 target runs their portable Xo bodies
(single-block AES-NI, PCLMULQDQ and SHA-NI are done). This measures the
size of that gap before a port is decided.

Setup:
- Machine: the owner's Intel MacBook (i7-9750H, Coffee Lake: AES-NI,
  PCLMULQDQ, AVX2; no VAES or VPCLMULQDQ), macOS 26.5.2.
- Programs: bench/perf/aesgcm and bench/perf/chacha (16 KiB records, the
  TLS bulk path through internal.crypto_op) and their Go twins
  (twin.go.txt: crypto/aes and crypto/cipher; golang.org/x/crypto v0.57.0
  for chacha). Their outputs agree (scripts/twincheck, 2026-10-24).
- Xo: `xo build --backend=amd64 --targets=darwin/amd64`, cross built on
  the Mac by xo at main aaf2541c3 plus the codegen tool change (e27f18043;
  no compiler or std change). Go 1.26.1, GOOS=darwin GOARCH=amd64.
- `xo bench --ab --rounds=11 --metrics=wall_ms,cpu_ms`, A the Go twin, B
  Xo, N=20000 records, load 1.5 to 2.5 (the laptop's background).
- linux/amd64 is not measured: Docker on the arm64 Mac runs amd64 under
  emulation, which does not time AES-NI or PCLMULQDQ the way a real x86
  core does. The lead asked the owner for one CI run on real x86 Linux.

| workload | Go wall ms | Xo wall ms | Xo / Go (95% interval) |
|---|---|---|---|
| aesgcm seal1 (key made once) | 62.44 | 229.2 | 3.671 (3.581 to 3.719) |
| aesgcm open1 | 65.98 | 230.0 | 3.486 (3.442 to 3.535) |
| aesgcm seal (key per record) | 61.72 | 270.0 | 4.375 (4.306 to 4.407) |
| aesgcm open | 66.91 | 281.0 | 4.2 (4.112 to 4.284) |
| chacha seal (ChaCha20-Poly1305) | 144.7 | 2951 | 20.39 (20.23 to 20.53) |
| chacha open | 144.6 | 2960 | 20.47 (20.28 to 20.71) |
| chacha xor (ChaCha20 alone) | 532.2 | 2753 | 5.172 (5.147 to 5.231) |
| chacha poly (Poly1305 alone) | 106.0 | 189.5 | 1.788 (1.75 to 1.858) |

Reading:
- AES-GCM is 3.5 to 4.4 times Go: the single-block AES-NI and PCLMULQDQ
  paths run one block at a time. Go's gcm assembly interleaves 8 blocks of
  AES-NI and aggregates GHASH over 8 blocks.
- ChaCha20-Poly1305 is about 20 times Go. Xo's ChaCha20 runs one block at
  a time in scalar code; Go's chacha20poly1305 uses its AVX2 assembly
  (eight blocks). The xor workload's Go side is x/crypto's chacha20
  package, which on amd64 is Go code, hence the 5.2 there.
- Poly1305 alone is 1.8 times Go: scalar on both sides, with mulq pairs.
- A port's ceiling on this CPU: AES-GCM at Go's speed is about 3.5 times
  faster TLS bulk encryption with AES; ChaCha20-Poly1305 about 20 times.
  Both need SSE2/AVX2 vector code in the x86 emitter; the arm64 NEON
  versions (compiler/ssa call.xo:218-232, decisions 0329, 0339) give the
  structure.
