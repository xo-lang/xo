kind: perf
module: crypto
result: fail
commit: 374c25851
source: 51a68ecc46bd0a9597ca8afeb31fe10e58659f981d64675f3ca76dc7fa89e6ac
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" crypto
measure: 36 cases of 7 twin directories, worst Xo / Go 1.228 over the cases above 5 ms (Proven at or under 1.05), 53s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/aead, bench/perf/aesgcm, bench/perf/aesmodes, bench/perf/chacha, bench/perf/ed25519, bench/perf/mlkem, bench/perf/sha3; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/aead/none (under 5 ms: startup) | 3.5 | 2.1 | 0.594 |
| bench/perf/aead/gcm_small | 49.1 | 55.0 | 1.119 |
| bench/perf/aead/gcm_big | 37.5 | 20.9 | 0.557 |
| bench/perf/aead/chacha_small | 99.8 | 116.1 | 1.164 |
| bench/perf/aead/chacha_big | 131.6 | 79.9 | 0.607 |
| bench/perf/aead/cbc_small | 207.8 | 181.6 | 0.874 |
| bench/perf/aead/cbc_big | 101.7 | 77.7 | 0.764 |
| bench/perf/aead/gcm_key | 36.9 | 32.0 | 0.866 |
| bench/perf/aead/hkdf | 133.7 | 122.3 | 0.915 |
| bench/perf/aesgcm/seal | 40.9 | 44.9 | 1.097 |
| bench/perf/aesgcm/open | 39.5 | 48.5 | 1.228 |
| bench/perf/aesgcm/seal1 | 36.5 | 40.2 | 1.100 |
| bench/perf/aesgcm/open1 | 34.9 | 38.9 | 1.115 |
| bench/perf/aesgcm/key | 6.8 | 6.3 | 0.933 |
| bench/perf/aesmodes/none (under 5 ms: startup) | 0.7 | 0.7 | 1.018 |
| bench/perf/aesmodes/block | 6.1 | 6.8 | 1.112 |
| bench/perf/aesmodes/cbcenc | 10.2 | 5.3 | 0.519 |
| bench/perf/aesmodes/cbcdec | 6.5 | 5.0 | 0.770 |
| bench/perf/aesmodes/ctr (under 5 ms: startup) | 3.3 | 3.1 | 0.930 |
| bench/perf/chacha/seal | 247.5 | 241.4 | 0.975 |
| bench/perf/chacha/open | 245.8 | 250.3 | 1.019 |
| bench/perf/chacha/xor | 169.7 | 170.2 | 1.003 |
| bench/perf/chacha/poly | 77.3 | 69.2 | 0.894 |
| bench/perf/ed25519/none (under 5 ms: startup) | 0.7 | 0.7 | 1.014 |
| bench/perf/ed25519/keygen | 54.1 | 40.1 | 0.742 |
| bench/perf/ed25519/sign | 68.2 | 81.5 | 1.194 |
| bench/perf/ed25519/signkey | 68.1 | 43.5 | 0.638 |
| bench/perf/ed25519/verify | 145.1 | 111.5 | 0.768 |
| bench/perf/mlkem/none (under 5 ms: startup) | 0.8 | 0.6 | 0.821 |
| bench/perf/mlkem/keygen | 92.1 | 52.9 | 0.574 |
| bench/perf/mlkem/parse | 37.1 | 33.7 | 0.910 |
| bench/perf/mlkem/encaps | 99.4 | 30.8 | 0.310 |
| bench/perf/mlkem/decaps | 91.0 | 55.2 | 0.607 |
| bench/perf/sha3/none (under 5 ms: startup) | 3.5 | 2.0 | 0.577 |
| bench/perf/sha3/big | 74.2 | 75.9 | 1.023 |
| bench/perf/sha3/small | 58.6 | 68.2 | 1.165 |
