kind: perf
module: crypto
result: fail
commit: 8ea9d8269
source: a4b65f271df516bff2d6174587e5b80557457ebf71478845b9d7d0b0fb89b664
date: 2026-11-19
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" crypto
measure: 41 cases of 8 twin directories, worst Xo / Go 1.473 over the cases above 5 ms (Proven at or under 1.05), 1m5s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/aead, bench/perf/aesgcm, bench/perf/aesmodes, bench/perf/blake2b, bench/perf/chacha, bench/perf/ed25519, bench/perf/mlkem, bench/perf/sha3; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/aead/none | 5.0 | 2.7 | 0.531 |
| bench/perf/aead/gcm_small | 50.9 | 53.6 | 1.053 |
| bench/perf/aead/gcm_big | 36.9 | 15.7 | 0.426 |
| bench/perf/aead/chacha_small | 83.8 | 123.3 | 1.473 |
| bench/perf/aead/chacha_big | 47.4 | 28.7 | 0.606 |
| bench/perf/aead/cbc_small | 304.5 | 128.1 | 0.421 |
| bench/perf/aead/cbc_big | 125.0 | 80.5 | 0.644 |
| bench/perf/aead/gcm_key | 54.6 | 35.5 | 0.650 |
| bench/perf/aead/hkdf | 190.7 | 101.0 | 0.530 |
| bench/perf/aesgcm/seal | 55.3 | 51.8 | 0.937 |
| bench/perf/aesgcm/open | 53.3 | 52.1 | 0.977 |
| bench/perf/aesgcm/seal1 | 43.5 | 45.3 | 1.040 |
| bench/perf/aesgcm/open1 | 42.7 | 45.8 | 1.072 |
| bench/perf/aesgcm/key | 13.0 | 8.7 | 0.669 |
| bench/perf/aesmodes/none (under 5 ms: startup) | 1.6 | 1.4 | 0.846 |
| bench/perf/aesmodes/block | 7.1 | 4.9 | 0.692 |
| bench/perf/aesmodes/cbcenc | 16.7 | 12.9 | 0.771 |
| bench/perf/aesmodes/cbcdec | 10.9 | 2.4 | 0.220 |
| bench/perf/aesmodes/ctr | 8.9 | 6.9 | 0.771 |
| bench/perf/blake2b/none | 5.5 | 2.7 | 0.493 |
| bench/perf/blake2b/big | 54.4 | 32.2 | 0.593 |
| bench/perf/blake2b/big512 | 54.4 | 32.5 | 0.598 |
| bench/perf/blake2b/small | 43.2 | 28.2 | 0.653 |
| bench/perf/blake2b/keyed | 108.4 | 68.8 | 0.635 |
| bench/perf/chacha/seal | 103.4 | 106.0 | 1.025 |
| bench/perf/chacha/open | 104.5 | 107.6 | 1.030 |
| bench/perf/chacha/xor | 296.7 | 319.4 | 1.077 |
| bench/perf/chacha/poly | 66.6 | 61.5 | 0.924 |
| bench/perf/ed25519/none (under 5 ms: startup) | 1.6 | 1.3 | 0.831 |
| bench/perf/ed25519/keygen | 54.2 | 46.2 | 0.852 |
| bench/perf/ed25519/sign | 65.6 | 51.6 | 0.786 |
| bench/perf/ed25519/signkey | 65.7 | 51.4 | 0.782 |
| bench/perf/ed25519/verify | 151.4 | 144.1 | 0.952 |
| bench/perf/mlkem/none (under 5 ms: startup) | 1.7 | 1.3 | 0.751 |
| bench/perf/mlkem/keygen | 125.2 | 66.0 | 0.527 |
| bench/perf/mlkem/parse | 49.5 | 42.4 | 0.857 |
| bench/perf/mlkem/encaps | 125.5 | 38.1 | 0.304 |
| bench/perf/mlkem/decaps | 118.3 | 66.0 | 0.558 |
| bench/perf/sha3/none | 5.1 | 2.6 | 0.517 |
| bench/perf/sha3/big | 82.1 | 87.1 | 1.061 |
| bench/perf/sha3/small | 68.2 | 73.0 | 1.071 |
