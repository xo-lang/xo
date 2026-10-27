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

## After the AES-GCM port (same machine and method, the same day)

cpu.aes_gcm8 lowered on amd64 (compiler/ssa x86aesgcm8.xo, Go's
gcmAesEnc structure) and the key schedule's SubWord with AES-NI
(std/changes/crypto.md 2026-10-27). A profile of the old seal1 had every
sample in the bulk (aes_ctr_by4 55%, ghash4_soft 45%), so the bulk was
the ceiling. The twin's seal and open now make the key each record, as
Xo's loop does (before, only seal1 and open1 compared the same work),
and key (a key, then a seal of nothing) times the key setup alone.

| workload | Go wall ms | Xo wall ms | Xo / Go (95% interval) | before |
|---|---|---|---|---|
| aesgcm seal1 (key once) | 62.71 | 68.26 | 1.089 (1.077 to 1.097) | 3.671 |
| aesgcm open1 (key once) | 65.79 | 67.17 | 1.021 (1.004 to 1.04) | 3.486 |
| aesgcm seal (key a record) | 72.98 | 82.66 | 1.133 (1.114 to 1.153) | not like for like |
| aesgcm open (key a record) | 77.03 | 88.41 | 1.148 (1.13 to 1.165) | not like for like |
| aesgcm key (key, empty seal) | 16.61 | 21.2 | 1.276 (1.252 to 1.291) | 2.755 without SubWord's change |

SubWord with AES-NI alone (A the port without it, B with): key 0.455
(0.447 to 0.457), seal 0.743 (0.733 to 0.753). Key setup is still 1.28
times Go's, about 0.23 us a key more, which a TLS handshake pays once
per direction.

A first version that ran the eight blocks' rounds and then their GHASH
reached 1.662 (seal1) and 1.335 (open1): sealing's GHASH waited on each
block's rounds. A first measurement of the SubWord change read "no
change" because the std file was edited while the previous kit was being
built from the same tree, so both binaries had it; the A/B above is
between two clean builds.

## A second CPU: Windows CI, Xeon Platinum 8488C (2026-10-27)

The two approved Windows CI runs (40525427807 and 40532204416, suite
script, runner xo-windows; scripts/ci/l4-x86-check.sh on the CI branch,
not merged) built the twins natively with xo 0.32.0 windows/amd64 and Go,
N 20000 records, 11 rounds interleaved, the median wall ms of each side.
"AVX2 off" is the same build from a copy of std whose cpu.hw() masks
HW_AVX2, so ChaCha20-Poly1305 takes the SSE loop. Run 40532204416:

| workload | Go ms | Xo ms | Xo / Go | AVX2 off ms | AVX2 off / Go |
|---|---|---|---|---|---|
| aesgcm seal1 | 83 | 100 | 1.205 | 99 | 1.193 |
| aesgcm open1 | 79 | 98 | 1.241 | 96 | 1.215 |
| aesgcm seal | 91 | 108 | 1.187 | 109 | 1.198 |
| aesgcm open | 90 | 116 | 1.289 | 117 | 1.300 |
| aesgcm key | 41 | 45 | 1.098 | 45 | 1.098 |
| chacha seal | 182 | 225 | 1.236 | 367 | 2.016 |
| chacha open | 184 | 234 | 1.272 | 375 | 2.038 |
| chacha xor | 528 | 348 | 0.659 | 346 | 0.655 |
| chacha poly | 134 | 180 | 1.343 | 179 | 1.336 |

Run 40525427807 gave the same ratios within 0.04. On this CPU the AVX2
loop is 1.6 times the SSE one (the i7-9750H's A/B: 0.638). The aead_diff
golden (seal and open against Go at every block edge, 188 cases) passed
with std as is, with HW_AVX2 masked and with HW_AVX2|HW_SSSE3 masked, in
both runs.
