kind: perf
module: crypto
result: fail
commit: 28c12562a
source: f7886cec473a071ce56ffe01c14383e08620088e839a2d506c1ea77c92b0f2eb
date: 2026-11-16
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" crypto
measure: 41 cases of 8 twin directories, worst Xo / Go 1.177 over the cases above 5 ms (Proven at or under 1.05), 57s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/aead, bench/perf/aesgcm, bench/perf/aesmodes, bench/perf/blake2b, bench/perf/chacha, bench/perf/ed25519, bench/perf/mlkem, bench/perf/sha3; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/aead/none (under 5 ms: startup) | 3.6 | 2.1 | 0.576 |
| bench/perf/aead/gcm_small | 46.2 | 45.2 | 0.979 |
| bench/perf/aead/gcm_big | 38.7 | 21.1 | 0.546 |
| bench/perf/aead/chacha_small | 101.5 | 116.7 | 1.149 |
| bench/perf/aead/chacha_big | 142.8 | 81.2 | 0.569 |
| bench/perf/aead/cbc_small | 209.5 | 176.3 | 0.842 |
| bench/perf/aead/cbc_big | 101.4 | 79.1 | 0.780 |
| bench/perf/aead/gcm_key | 38.5 | 31.7 | 0.823 |
| bench/perf/aead/hkdf | 131.8 | 114.7 | 0.870 |
| bench/perf/aesgcm/seal | 41.4 | 41.9 | 1.013 |
| bench/perf/aesgcm/open | 39.9 | 41.7 | 1.046 |
| bench/perf/aesgcm/seal1 | 36.4 | 37.6 | 1.032 |
| bench/perf/aesgcm/open1 | 35.0 | 37.2 | 1.063 |
| bench/perf/aesgcm/key | 6.7 | 6.3 | 0.941 |
| bench/perf/aesmodes/none (under 5 ms: startup) | 0.7 | 0.7 | 0.999 |
| bench/perf/aesmodes/block | 6.2 | 6.4 | 1.040 |
| bench/perf/aesmodes/cbcenc | 10.0 | 5.9 | 0.591 |
| bench/perf/aesmodes/cbcdec | 6.4 | 5.1 | 0.799 |
| bench/perf/aesmodes/ctr (under 5 ms: startup) | 3.0 | 3.1 | 1.012 |
| bench/perf/blake2b/none (under 5 ms: startup) | 3.5 | 2.0 | 0.589 |
| bench/perf/blake2b/big | 33.4 | 33.1 | 0.993 |
| bench/perf/blake2b/big512 | 33.4 | 33.1 | 0.992 |
| bench/perf/blake2b/small | 26.9 | 29.1 | 1.081 |
| bench/perf/blake2b/keyed | 58.8 | 69.3 | 1.177 |
| bench/perf/chacha/seal | 250.9 | 243.8 | 0.972 |
| bench/perf/chacha/open | 248.4 | 252.4 | 1.016 |
| bench/perf/chacha/xor | 171.5 | 171.5 | 1.000 |
| bench/perf/chacha/poly | 77.6 | 69.1 | 0.891 |
| bench/perf/ed25519/none (under 5 ms: startup) | 0.7 | 0.8 | 1.013 |
| bench/perf/ed25519/keygen | 54.0 | 39.9 | 0.739 |
| bench/perf/ed25519/sign | 68.0 | 43.7 | 0.643 |
| bench/perf/ed25519/signkey | 68.1 | 43.7 | 0.641 |
| bench/perf/ed25519/verify | 145.1 | 111.9 | 0.772 |
| bench/perf/mlkem/none (under 5 ms: startup) | 0.7 | 0.6 | 0.811 |
| bench/perf/mlkem/keygen | 92.4 | 52.6 | 0.569 |
| bench/perf/mlkem/parse | 37.5 | 33.3 | 0.889 |
| bench/perf/mlkem/encaps | 96.9 | 31.1 | 0.321 |
| bench/perf/mlkem/decaps | 90.1 | 55.3 | 0.614 |
| bench/perf/sha3/none (under 5 ms: startup) | 3.5 | 2.0 | 0.576 |
| bench/perf/sha3/big | 74.1 | 73.6 | 0.994 |
| bench/perf/sha3/small | 58.3 | 59.6 | 1.023 |
