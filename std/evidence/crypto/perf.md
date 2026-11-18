kind: perf
module: crypto
result: fail
commit: 09f097a2d
source: 5e5ab18fe732f92c12f33756038fe28853346cf68fff33060b8f3a4968277983
date: 2026-11-18
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" crypto
measure: 41 cases of 8 twin directories, worst Xo / Go 1.858 over the cases above 5 ms (Proven at or under 1.05), 1m8s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/aead, bench/perf/aesgcm, bench/perf/aesmodes, bench/perf/blake2b, bench/perf/chacha, bench/perf/ed25519, bench/perf/mlkem, bench/perf/sha3; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/aead/none | 5.1 | 2.8 | 0.535 |
| bench/perf/aead/gcm_small | 51.7 | 51.6 | 0.997 |
| bench/perf/aead/gcm_big | 37.6 | 17.4 | 0.464 |
| bench/perf/aead/chacha_small | 82.2 | 129.6 | 1.578 |
| bench/perf/aead/chacha_big | 50.6 | 44.8 | 0.886 |
| bench/perf/aead/cbc_small | 308.4 | 144.1 | 0.467 |
| bench/perf/aead/cbc_big | 123.4 | 114.2 | 0.925 |
| bench/perf/aead/gcm_key | 54.8 | 30.1 | 0.550 |
| bench/perf/aead/hkdf | 193.0 | 104.3 | 0.541 |
| bench/perf/aesgcm/seal | 54.6 | 58.9 | 1.079 |
| bench/perf/aesgcm/open | 53.6 | 58.5 | 1.093 |
| bench/perf/aesgcm/seal1 | 45.4 | 55.5 | 1.222 |
| bench/perf/aesgcm/open1 | 43.2 | 55.5 | 1.283 |
| bench/perf/aesgcm/key | 12.9 | 7.1 | 0.547 |
| bench/perf/aesmodes/none (under 5 ms: startup) | 1.7 | 1.4 | 0.867 |
| bench/perf/aesmodes/block | 6.7 | 4.8 | 0.724 |
| bench/perf/aesmodes/cbcenc | 16.8 | 12.5 | 0.746 |
| bench/perf/aesmodes/cbcdec | 10.9 | 13.5 | 1.246 |
| bench/perf/aesmodes/ctr | 8.6 | 6.3 | 0.727 |
| bench/perf/blake2b/none | 5.1 | 2.6 | 0.505 |
| bench/perf/blake2b/big | 54.7 | 32.6 | 0.596 |
| bench/perf/blake2b/big512 | 54.5 | 32.7 | 0.600 |
| bench/perf/blake2b/small | 43.1 | 27.9 | 0.647 |
| bench/perf/blake2b/keyed | 109.1 | 60.8 | 0.558 |
| bench/perf/chacha/seal | 103.7 | 166.6 | 1.607 |
| bench/perf/chacha/open | 104.4 | 168.9 | 1.618 |
| bench/perf/chacha/xor | 297.1 | 319.6 | 1.076 |
| bench/perf/chacha/poly | 66.8 | 83.6 | 1.251 |
| bench/perf/ed25519/none (under 5 ms: startup) | 1.6 | 1.3 | 0.831 |
| bench/perf/ed25519/keygen | 54.1 | 46.3 | 0.855 |
| bench/perf/ed25519/sign | 65.7 | 51.8 | 0.788 |
| bench/perf/ed25519/signkey | 65.5 | 51.4 | 0.785 |
| bench/perf/ed25519/verify | 151.6 | 144.1 | 0.950 |
| bench/perf/mlkem/none (under 5 ms: startup) | 1.7 | 1.3 | 0.799 |
| bench/perf/mlkem/keygen | 127.4 | 87.3 | 0.685 |
| bench/perf/mlkem/parse | 52.1 | 61.2 | 1.176 |
| bench/perf/mlkem/encaps | 127.5 | 41.1 | 0.323 |
| bench/perf/mlkem/decaps | 119.3 | 79.2 | 0.664 |
| bench/perf/sha3/none (under 5 ms: startup) | 4.9 | 2.7 | 0.545 |
| bench/perf/sha3/big | 81.3 | 151.1 | 1.858 |
| bench/perf/sha3/small | 68.6 | 111.1 | 1.619 |
