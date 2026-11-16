kind: perf
module: crypto
result: fail
commit: 84fa39d87
source: 8a53c549eac09369f0e92af16ba374cb6288043e2a35f6cf1ad3339b41251db9
date: 2026-11-16
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" crypto
measure: 41 cases of 8 twin directories, worst Xo / Go 1.245 over the cases above 5 ms (Proven at or under 1.05), 57s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/aead, bench/perf/aesgcm, bench/perf/aesmodes, bench/perf/blake2b, bench/perf/chacha, bench/perf/ed25519, bench/perf/mlkem, bench/perf/sha3; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/aead/none (under 5 ms: startup) | 3.7 | 2.2 | 0.591 |
| bench/perf/aead/gcm_small | 50.1 | 45.6 | 0.911 |
| bench/perf/aead/gcm_big | 34.5 | 21.2 | 0.614 |
| bench/perf/aead/chacha_small | 101.4 | 117.5 | 1.159 |
| bench/perf/aead/chacha_big | 136.7 | 81.4 | 0.595 |
| bench/perf/aead/cbc_small | 202.3 | 180.4 | 0.892 |
| bench/perf/aead/cbc_big | 104.1 | 79.5 | 0.764 |
| bench/perf/aead/gcm_key | 38.1 | 31.8 | 0.835 |
| bench/perf/aead/hkdf | 136.3 | 118.1 | 0.866 |
| bench/perf/aesgcm/seal | 41.2 | 42.2 | 1.025 |
| bench/perf/aesgcm/open | 39.7 | 47.4 | 1.193 |
| bench/perf/aesgcm/seal1 | 36.4 | 37.4 | 1.030 |
| bench/perf/aesgcm/open1 | 34.8 | 37.3 | 1.075 |
| bench/perf/aesgcm/key | 6.6 | 6.3 | 0.957 |
| bench/perf/aesmodes/none (under 5 ms: startup) | 0.6 | 0.6 | 1.057 |
| bench/perf/aesmodes/block | 6.3 | 7.1 | 1.123 |
| bench/perf/aesmodes/cbcenc | 10.2 | 5.4 | 0.527 |
| bench/perf/aesmodes/cbcdec | 6.6 | 5.1 | 0.778 |
| bench/perf/aesmodes/ctr (under 5 ms: startup) | 3.2 | 3.1 | 0.991 |
| bench/perf/blake2b/none (under 5 ms: startup) | 3.6 | 2.1 | 0.603 |
| bench/perf/blake2b/big | 33.5 | 32.9 | 0.984 |
| bench/perf/blake2b/big512 | 33.4 | 32.9 | 0.985 |
| bench/perf/blake2b/small | 27.0 | 33.5 | 1.241 |
| bench/perf/blake2b/keyed | 59.8 | 74.4 | 1.245 |
| bench/perf/chacha/seal | 255.6 | 246.8 | 0.966 |
| bench/perf/chacha/open | 252.6 | 256.0 | 1.013 |
| bench/perf/chacha/xor | 172.7 | 173.8 | 1.006 |
| bench/perf/chacha/poly | 77.8 | 69.2 | 0.889 |
| bench/perf/ed25519/none (under 5 ms: startup) | 0.7 | 0.7 | 1.040 |
| bench/perf/ed25519/keygen | 54.2 | 40.1 | 0.741 |
| bench/perf/ed25519/sign | 68.2 | 43.7 | 0.641 |
| bench/perf/ed25519/signkey | 68.2 | 43.6 | 0.640 |
| bench/perf/ed25519/verify | 145.4 | 112.2 | 0.772 |
| bench/perf/mlkem/none (under 5 ms: startup) | 0.7 | 0.6 | 0.834 |
| bench/perf/mlkem/keygen | 92.2 | 52.8 | 0.573 |
| bench/perf/mlkem/parse | 37.3 | 33.3 | 0.893 |
| bench/perf/mlkem/encaps | 96.6 | 31.2 | 0.323 |
| bench/perf/mlkem/decaps | 90.1 | 55.4 | 0.615 |
| bench/perf/sha3/none (under 5 ms: startup) | 3.5 | 2.0 | 0.570 |
| bench/perf/sha3/big | 74.2 | 73.8 | 0.995 |
| bench/perf/sha3/small | 58.5 | 60.0 | 1.026 |
