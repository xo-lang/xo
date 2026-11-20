kind: perf
module: crypto
result: pass
commit: 7f7a594dd
source: e5b3cc2513809c504b55928b095eae795a4f9a66b4838af3f3f6d61b368ab6db
date: 2026-11-20
host: linux/arm64 host 2
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 2" crypto
measure: 41 cases of 8 twin directories, worst Xo / Go 1.006 over the cases above 5 ms (Proven at or under 1.05), 51s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/aead, bench/perf/aesgcm, bench/perf/aesmodes, bench/perf/blake2b, bench/perf/chacha, bench/perf/ed25519, bench/perf/mlkem, bench/perf/sha3; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/aead/none (under 5 ms: startup) | 3.3 | 1.9 | 0.561 |
| bench/perf/aead/gcm_small | 47.7 | 35.5 | 0.744 |
| bench/perf/aead/gcm_big | 26.2 | 14.8 | 0.564 |
| bench/perf/aead/chacha_small | 94.8 | 81.3 | 0.858 |
| bench/perf/aead/chacha_big | 86.5 | 67.4 | 0.779 |
| bench/perf/aead/cbc_small | 208.8 | 136.8 | 0.655 |
| bench/perf/aead/cbc_big | 81.5 | 52.8 | 0.648 |
| bench/perf/aead/gcm_key | 34.0 | 21.6 | 0.636 |
| bench/perf/aead/hkdf | 123.4 | 98.5 | 0.798 |
| bench/perf/aesgcm/seal | 38.2 | 34.8 | 0.911 |
| bench/perf/aesgcm/open | 36.8 | 33.8 | 0.919 |
| bench/perf/aesgcm/seal1 | 34.0 | 32.5 | 0.954 |
| bench/perf/aesgcm/open1 | 32.7 | 31.4 | 0.962 |
| bench/perf/aesgcm/key | 6.1 | 4.5 | 0.740 |
| bench/perf/aesmodes/none (under 5 ms: startup) | 0.5 | 0.5 | 0.956 |
| bench/perf/aesmodes/block | 6.3 | 6.4 | 1.006 |
| bench/perf/aesmodes/cbcenc | 10.1 | 5.2 | 0.514 |
| bench/perf/aesmodes/cbcdec | 5.7 | 1.1 | 0.187 |
| bench/perf/aesmodes/ctr (under 5 ms: startup) | 3.1 | 2.8 | 0.913 |
| bench/perf/blake2b/none (under 5 ms: startup) | 3.4 | 1.8 | 0.523 |
| bench/perf/blake2b/big | 32.3 | 28.4 | 0.877 |
| bench/perf/blake2b/big512 | 32.3 | 28.2 | 0.873 |
| bench/perf/blake2b/small | 25.9 | 23.9 | 0.921 |
| bench/perf/blake2b/keyed | 58.5 | 50.1 | 0.855 |
| bench/perf/chacha/seal | 236.0 | 226.7 | 0.961 |
| bench/perf/chacha/open | 232.3 | 232.8 | 1.002 |
| bench/perf/chacha/xor | 162.5 | 162.1 | 0.998 |
| bench/perf/chacha/poly | 70.7 | 65.3 | 0.924 |
| bench/perf/ed25519/none (under 5 ms: startup) | 0.6 | 0.6 | 1.064 |
| bench/perf/ed25519/keygen | 49.9 | 35.3 | 0.706 |
| bench/perf/ed25519/sign | 62.6 | 38.3 | 0.612 |
| bench/perf/ed25519/signkey | 62.6 | 38.3 | 0.612 |
| bench/perf/ed25519/verify | 130.4 | 98.3 | 0.753 |
| bench/perf/mlkem/none (under 5 ms: startup) | 0.6 | 0.5 | 0.837 |
| bench/perf/mlkem/keygen | 87.0 | 46.8 | 0.538 |
| bench/perf/mlkem/parse | 36.0 | 29.9 | 0.829 |
| bench/perf/mlkem/encaps | 92.4 | 27.8 | 0.301 |
| bench/perf/mlkem/decaps | 85.3 | 49.5 | 0.581 |
| bench/perf/sha3/none (under 5 ms: startup) | 3.4 | 1.9 | 0.549 |
| bench/perf/sha3/big | 69.3 | 68.4 | 0.987 |
| bench/perf/sha3/small | 53.9 | 51.5 | 0.956 |
