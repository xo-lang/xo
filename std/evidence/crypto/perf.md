kind: perf
module: crypto
result: fail
commit: b232ddf22
source: 8f1505bf339b431500fbc5047a44cb072d9ff4c1446265296fd4b4bb57014641
date: 2026-11-18
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" crypto
measure: 41 cases of 8 twin directories, worst Xo / Go 2.545 over the cases above 5 ms (Proven at or under 1.05), 56s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/aead, bench/perf/aesgcm, bench/perf/aesmodes, bench/perf/blake2b, bench/perf/chacha, bench/perf/ed25519, bench/perf/mlkem, bench/perf/sha3; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/aead/none (under 5 ms: startup) | 3.6 | 2.0 | 0.567 |
| bench/perf/aead/gcm_small | 48.4 | 46.6 | 0.962 |
| bench/perf/aead/gcm_big | 34.9 | 20.9 | 0.597 |
| bench/perf/aead/chacha_small | 98.1 | 94.3 | 0.961 |
| bench/perf/aead/chacha_big | 133.7 | 79.1 | 0.592 |
| bench/perf/aead/cbc_small | 202.8 | 162.8 | 0.803 |
| bench/perf/aead/cbc_big | 99.8 | 75.7 | 0.758 |
| bench/perf/aead/gcm_key | 37.0 | 34.6 | 0.934 |
| bench/perf/aead/hkdf | 133.3 | 115.4 | 0.866 |
| bench/perf/aesgcm/seal | 40.3 | 43.0 | 1.066 |
| bench/perf/aesgcm/open | 39.1 | 42.0 | 1.074 |
| bench/perf/aesgcm/seal1 | 36.2 | 37.7 | 1.042 |
| bench/perf/aesgcm/open1 | 34.8 | 37.4 | 1.076 |
| bench/perf/aesgcm/key | 6.3 | 7.1 | 1.128 |
| bench/perf/aesmodes/none (under 5 ms: startup) | 0.7 | 0.6 | 0.902 |
| bench/perf/aesmodes/block | 3.7 | 7.5 | 2.034 |
| bench/perf/aesmodes/cbcenc | 9.9 | 5.3 | 0.533 |
| bench/perf/aesmodes/cbcdec | 6.4 | 16.3 | 2.545 |
| bench/perf/aesmodes/ctr (under 5 ms: startup) | 3.2 | 3.7 | 1.172 |
| bench/perf/blake2b/none (under 5 ms: startup) | 3.5 | 2.1 | 0.594 |
| bench/perf/blake2b/big | 33.6 | 29.9 | 0.890 |
| bench/perf/blake2b/big512 | 33.4 | 29.9 | 0.894 |
| bench/perf/blake2b/small | 27.1 | 29.2 | 1.078 |
| bench/perf/blake2b/keyed | 59.6 | 56.5 | 0.948 |
| bench/perf/chacha/seal | 244.1 | 241.2 | 0.988 |
| bench/perf/chacha/open | 243.3 | 251.8 | 1.035 |
| bench/perf/chacha/xor | 167.9 | 168.6 | 1.004 |
| bench/perf/chacha/poly | 77.1 | 68.9 | 0.893 |
| bench/perf/ed25519/none (under 5 ms: startup) | 0.7 | 0.7 | 0.961 |
| bench/perf/ed25519/keygen | 54.1 | 40.1 | 0.740 |
| bench/perf/ed25519/sign | 68.2 | 43.7 | 0.641 |
| bench/perf/ed25519/signkey | 68.1 | 43.9 | 0.645 |
| bench/perf/ed25519/verify | 145.0 | 110.8 | 0.764 |
| bench/perf/mlkem/none (under 5 ms: startup) | 0.7 | 0.6 | 0.875 |
| bench/perf/mlkem/keygen | 92.0 | 52.3 | 0.568 |
| bench/perf/mlkem/parse | 37.1 | 33.2 | 0.895 |
| bench/perf/mlkem/encaps | 99.2 | 33.3 | 0.336 |
| bench/perf/mlkem/decaps | 90.3 | 57.8 | 0.640 |
| bench/perf/sha3/none (under 5 ms: startup) | 3.5 | 2.0 | 0.576 |
| bench/perf/sha3/big | 74.2 | 72.9 | 0.983 |
| bench/perf/sha3/small | 58.5 | 59.8 | 1.022 |
