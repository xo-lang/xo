kind: perf
module: crypto
result: fail
commit: 554dc5eff
source: 09244048347016a6c06af96d004e01a8d187efad7d354480eacaa18ad323fda1
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" crypto
measure: 36 cases of 7 twin directories, worst Xo / Go 1.732 over the cases above 5 ms (Proven at or under 1.05), 50s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/aead, bench/perf/aesgcm, bench/perf/aesmodes, bench/perf/chacha, bench/perf/ed25519, bench/perf/mlkem, bench/perf/sha3; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/aead/none (under 5 ms: startup) | 3.5 | 2.0 | 0.570 |
| bench/perf/aead/gcm_small | 46.9 | 53.9 | 1.149 |
| bench/perf/aead/gcm_big | 37.3 | 19.7 | 0.527 |
| bench/perf/aead/chacha_small | 92.5 | 106.3 | 1.149 |
| bench/perf/aead/chacha_big | 133.7 | 74.1 | 0.554 |
| bench/perf/aead/cbc_small | 200.8 | 162.5 | 0.809 |
| bench/perf/aead/cbc_big | 94.9 | 72.5 | 0.764 |
| bench/perf/aead/gcm_key | 37.8 | 30.1 | 0.798 |
| bench/perf/aead/hkdf | 124.5 | 107.9 | 0.867 |
| bench/perf/aesgcm/seal | 39.3 | 43.0 | 1.096 |
| bench/perf/aesgcm/open | 37.5 | 45.9 | 1.223 |
| bench/perf/aesgcm/seal1 | 35.1 | 38.8 | 1.108 |
| bench/perf/aesgcm/open1 | 33.8 | 36.9 | 1.090 |
| bench/perf/aesgcm/key | 6.6 | 6.0 | 0.906 |
| bench/perf/aesmodes/none (under 5 ms: startup) | 0.7 | 0.7 | 1.001 |
| bench/perf/aesmodes/block | 3.8 | 6.5 | 1.732 |
| bench/perf/aesmodes/cbcenc | 9.6 | 5.3 | 0.550 |
| bench/perf/aesmodes/cbcdec | 6.2 | 5.0 | 0.802 |
| bench/perf/aesmodes/ctr (under 5 ms: startup) | 3.1 | 3.0 | 0.964 |
| bench/perf/chacha/seal | 233.0 | 225.4 | 0.967 |
| bench/perf/chacha/open | 233.2 | 236.5 | 1.014 |
| bench/perf/chacha/xor | 159.8 | 159.3 | 0.997 |
| bench/perf/chacha/poly | 74.2 | 68.6 | 0.926 |
| bench/perf/ed25519/none (under 5 ms: startup) | 0.7 | 0.7 | 0.988 |
| bench/perf/ed25519/keygen | 54.0 | 38.8 | 0.719 |
| bench/perf/ed25519/sign | 67.6 | 78.4 | 1.160 |
| bench/perf/ed25519/signkey | 67.2 | 41.8 | 0.623 |
| bench/perf/ed25519/verify | 143.5 | 106.9 | 0.745 |
| bench/perf/mlkem/none (under 5 ms: startup) | 0.8 | 0.6 | 0.804 |
| bench/perf/mlkem/keygen | 90.6 | 51.5 | 0.569 |
| bench/perf/mlkem/parse | 36.7 | 33.2 | 0.905 |
| bench/perf/mlkem/encaps | 96.5 | 30.4 | 0.315 |
| bench/perf/mlkem/decaps | 90.5 | 54.8 | 0.606 |
| bench/perf/sha3/none (under 5 ms: startup) | 3.5 | 2.1 | 0.593 |
| bench/perf/sha3/big | 74.1 | 74.8 | 1.009 |
| bench/perf/sha3/small | 58.0 | 66.7 | 1.149 |
