kind: perf
module: crypto
result: fail
commit: 7b9e4938c
source: e50f06bcfb0ee6c7a28da2038c10a019cd59f89051e4378ff7310d3900996b56
date: 2026-11-19
host: linux/arm64 host 2
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 2" crypto
measure: 41 cases of 8 twin directories, worst Xo / Go 1.093 over the cases above 5 ms (Proven at or under 1.05), 52s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/aead, bench/perf/aesgcm, bench/perf/aesmodes, bench/perf/blake2b, bench/perf/chacha, bench/perf/ed25519, bench/perf/mlkem, bench/perf/sha3; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/aead/none (under 5 ms: startup) | 3.3 | 1.8 | 0.539 |
| bench/perf/aead/gcm_small | 48.0 | 35.9 | 0.747 |
| bench/perf/aead/gcm_big | 30.8 | 14.7 | 0.478 |
| bench/perf/aead/chacha_small | 94.2 | 83.4 | 0.885 |
| bench/perf/aead/chacha_big | 101.8 | 68.0 | 0.669 |
| bench/perf/aead/cbc_small | 195.3 | 134.9 | 0.690 |
| bench/perf/aead/cbc_big | 85.6 | 53.2 | 0.622 |
| bench/perf/aead/gcm_key | 34.3 | 32.6 | 0.951 |
| bench/perf/aead/hkdf | 120.6 | 97.7 | 0.810 |
| bench/perf/aesgcm/seal | 38.2 | 37.4 | 0.977 |
| bench/perf/aesgcm/open | 37.0 | 36.4 | 0.984 |
| bench/perf/aesgcm/seal1 | 34.0 | 32.4 | 0.954 |
| bench/perf/aesgcm/open1 | 32.7 | 31.5 | 0.964 |
| bench/perf/aesgcm/key | 6.2 | 6.7 | 1.091 |
| bench/perf/aesmodes/none (under 5 ms: startup) | 0.5 | 0.5 | 1.107 |
| bench/perf/aesmodes/block | 6.3 | 6.9 | 1.093 |
| bench/perf/aesmodes/cbcenc | 9.8 | 5.1 | 0.523 |
| bench/perf/aesmodes/cbcdec | 5.5 | 1.1 | 0.202 |
| bench/perf/aesmodes/ctr (under 5 ms: startup) | 3.0 | 3.4 | 1.123 |
| bench/perf/blake2b/none (under 5 ms: startup) | 3.4 | 1.8 | 0.533 |
| bench/perf/blake2b/big | 32.5 | 28.2 | 0.869 |
| bench/perf/blake2b/big512 | 32.4 | 28.3 | 0.872 |
| bench/perf/blake2b/small | 25.7 | 27.8 | 1.080 |
| bench/perf/blake2b/keyed | 55.9 | 50.8 | 0.909 |
| bench/perf/chacha/seal | 235.1 | 229.4 | 0.976 |
| bench/perf/chacha/open | 232.8 | 235.5 | 1.012 |
| bench/perf/chacha/xor | 162.8 | 162.4 | 0.998 |
| bench/perf/chacha/poly | 70.3 | 66.3 | 0.944 |
| bench/perf/ed25519/none (under 5 ms: startup) | 0.7 | 0.6 | 0.965 |
| bench/perf/ed25519/keygen | 50.0 | 35.6 | 0.713 |
| bench/perf/ed25519/sign | 62.6 | 38.6 | 0.617 |
| bench/perf/ed25519/signkey | 62.6 | 38.7 | 0.617 |
| bench/perf/ed25519/verify | 131.0 | 98.5 | 0.752 |
| bench/perf/mlkem/none (under 5 ms: startup) | 0.5 | 0.5 | 0.912 |
| bench/perf/mlkem/keygen | 86.4 | 47.8 | 0.553 |
| bench/perf/mlkem/parse | 36.0 | 30.8 | 0.857 |
| bench/perf/mlkem/encaps | 92.8 | 29.4 | 0.317 |
| bench/perf/mlkem/decaps | 85.5 | 52.3 | 0.612 |
| bench/perf/sha3/none (under 5 ms: startup) | 3.4 | 1.9 | 0.559 |
| bench/perf/sha3/big | 69.3 | 70.0 | 1.010 |
| bench/perf/sha3/small | 53.9 | 54.5 | 1.011 |
