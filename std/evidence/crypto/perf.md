kind: perf
module: crypto
result: fail
commit: 541463aee
source: 787e7d60da88b1a295dbb355b972a6013cc944318b8cf1e4a69bd07421bbc35b
date: 2026-11-19
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" crypto
measure: 41 cases of 8 twin directories, worst Xo / Go 1.498 over the cases above 5 ms (Proven at or under 1.05), 1m6s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/aead, bench/perf/aesgcm, bench/perf/aesmodes, bench/perf/blake2b, bench/perf/chacha, bench/perf/ed25519, bench/perf/mlkem, bench/perf/sha3; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/aead/none | 5.0 | 2.7 | 0.537 |
| bench/perf/aead/gcm_small | 52.0 | 51.2 | 0.985 |
| bench/perf/aead/gcm_big | 34.9 | 17.8 | 0.510 |
| bench/perf/aead/chacha_small | 83.5 | 125.1 | 1.498 |
| bench/perf/aead/chacha_big | 48.4 | 29.1 | 0.602 |
| bench/perf/aead/cbc_small | 306.3 | 145.1 | 0.474 |
| bench/perf/aead/cbc_big | 126.4 | 113.4 | 0.897 |
| bench/perf/aead/gcm_key | 54.1 | 28.6 | 0.528 |
| bench/perf/aead/hkdf | 191.6 | 100.6 | 0.525 |
| bench/perf/aesgcm/seal | 55.3 | 58.7 | 1.061 |
| bench/perf/aesgcm/open | 54.0 | 60.3 | 1.116 |
| bench/perf/aesgcm/seal1 | 43.4 | 54.0 | 1.243 |
| bench/perf/aesgcm/open1 | 42.9 | 54.2 | 1.263 |
| bench/perf/aesgcm/key | 13.1 | 7.1 | 0.544 |
| bench/perf/aesmodes/none (under 5 ms: startup) | 1.8 | 1.4 | 0.781 |
| bench/perf/aesmodes/block | 6.9 | 4.9 | 0.715 |
| bench/perf/aesmodes/cbcenc | 17.2 | 12.8 | 0.745 |
| bench/perf/aesmodes/cbcdec | 10.8 | 13.5 | 1.242 |
| bench/perf/aesmodes/ctr | 8.5 | 6.3 | 0.745 |
| bench/perf/blake2b/none | 5.0 | 2.5 | 0.495 |
| bench/perf/blake2b/big | 54.8 | 32.5 | 0.592 |
| bench/perf/blake2b/big512 | 54.8 | 32.4 | 0.591 |
| bench/perf/blake2b/small | 43.2 | 28.3 | 0.655 |
| bench/perf/blake2b/keyed | 107.8 | 64.9 | 0.602 |
| bench/perf/chacha/seal | 103.9 | 105.9 | 1.020 |
| bench/perf/chacha/open | 104.2 | 108.9 | 1.046 |
| bench/perf/chacha/xor | 297.1 | 320.0 | 1.077 |
| bench/perf/chacha/poly | 66.9 | 83.8 | 1.253 |
| bench/perf/ed25519/none (under 5 ms: startup) | 1.6 | 1.4 | 0.872 |
| bench/perf/ed25519/keygen | 54.2 | 46.4 | 0.855 |
| bench/perf/ed25519/sign | 65.7 | 51.8 | 0.789 |
| bench/perf/ed25519/signkey | 65.3 | 51.7 | 0.792 |
| bench/perf/ed25519/verify | 152.1 | 144.2 | 0.948 |
| bench/perf/mlkem/none (under 5 ms: startup) | 1.7 | 1.4 | 0.800 |
| bench/perf/mlkem/keygen | 124.7 | 65.5 | 0.526 |
| bench/perf/mlkem/parse | 49.3 | 42.5 | 0.862 |
| bench/perf/mlkem/encaps | 124.6 | 38.6 | 0.310 |
| bench/perf/mlkem/decaps | 118.4 | 65.9 | 0.556 |
| bench/perf/sha3/none | 5.1 | 2.7 | 0.536 |
| bench/perf/sha3/big | 80.7 | 87.2 | 1.081 |
| bench/perf/sha3/small | 68.3 | 72.0 | 1.055 |
