kind: perf
module: crypto
result: fail
commit: 931899a77
source: a4b65f271df516bff2d6174587e5b80557457ebf71478845b9d7d0b0fb89b664
date: 2026-11-19
host: linux/arm64 host 2
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 2" crypto
measure: 41 cases of 8 twin directories, worst Xo / Go 1.322 over the cases above 5 ms (Proven at or under 1.05), 52s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/aead, bench/perf/aesgcm, bench/perf/aesmodes, bench/perf/blake2b, bench/perf/chacha, bench/perf/ed25519, bench/perf/mlkem, bench/perf/sha3; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/aead/none (under 5 ms: startup) | 3.3 | 1.9 | 0.572 |
| bench/perf/aead/gcm_small | 46.9 | 37.6 | 0.801 |
| bench/perf/aead/gcm_big | 29.3 | 14.8 | 0.504 |
| bench/perf/aead/chacha_small | 94.3 | 83.2 | 0.882 |
| bench/perf/aead/chacha_big | 107.6 | 67.2 | 0.624 |
| bench/perf/aead/cbc_small | 193.0 | 127.6 | 0.661 |
| bench/perf/aead/cbc_big | 82.1 | 52.6 | 0.641 |
| bench/perf/aead/gcm_key | 34.7 | 45.9 | 1.322 |
| bench/perf/aead/hkdf | 123.6 | 96.9 | 0.784 |
| bench/perf/aesgcm/seal | 38.3 | 38.0 | 0.993 |
| bench/perf/aesgcm/open | 36.7 | 36.8 | 1.003 |
| bench/perf/aesgcm/seal1 | 34.0 | 32.7 | 0.961 |
| bench/perf/aesgcm/open1 | 32.6 | 31.4 | 0.963 |
| bench/perf/aesgcm/key | 6.2 | 7.8 | 1.263 |
| bench/perf/aesmodes/none (under 5 ms: startup) | 0.5 | 0.5 | 1.054 |
| bench/perf/aesmodes/block | 6.2 | 6.9 | 1.110 |
| bench/perf/aesmodes/cbcenc | 10.0 | 5.3 | 0.532 |
| bench/perf/aesmodes/cbcdec | 5.6 | 1.1 | 0.202 |
| bench/perf/aesmodes/ctr (under 5 ms: startup) | 2.8 | 3.4 | 1.186 |
| bench/perf/blake2b/none (under 5 ms: startup) | 3.3 | 1.8 | 0.534 |
| bench/perf/blake2b/big | 32.3 | 28.2 | 0.873 |
| bench/perf/blake2b/big512 | 32.5 | 28.3 | 0.872 |
| bench/perf/blake2b/small | 25.7 | 27.7 | 1.077 |
| bench/perf/blake2b/keyed | 56.0 | 50.9 | 0.909 |
| bench/perf/chacha/seal | 241.3 | 231.2 | 0.958 |
| bench/perf/chacha/open | 231.1 | 232.4 | 1.005 |
| bench/perf/chacha/xor | 162.3 | 162.4 | 1.001 |
| bench/perf/chacha/poly | 70.1 | 66.2 | 0.944 |
| bench/perf/ed25519/none (under 5 ms: startup) | 0.5 | 0.6 | 1.071 |
| bench/perf/ed25519/keygen | 49.9 | 35.3 | 0.707 |
| bench/perf/ed25519/sign | 62.7 | 38.6 | 0.616 |
| bench/perf/ed25519/signkey | 62.7 | 38.6 | 0.616 |
| bench/perf/ed25519/verify | 131.5 | 98.9 | 0.752 |
| bench/perf/mlkem/none (under 5 ms: startup) | 0.6 | 0.6 | 0.928 |
| bench/perf/mlkem/keygen | 86.9 | 47.9 | 0.551 |
| bench/perf/mlkem/parse | 35.6 | 30.7 | 0.861 |
| bench/perf/mlkem/encaps | 92.5 | 29.6 | 0.320 |
| bench/perf/mlkem/decaps | 85.1 | 52.0 | 0.611 |
| bench/perf/sha3/none (under 5 ms: startup) | 3.4 | 1.8 | 0.548 |
| bench/perf/sha3/big | 69.1 | 69.6 | 1.007 |
| bench/perf/sha3/small | 53.7 | 54.2 | 1.009 |
