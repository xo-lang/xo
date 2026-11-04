# std twin jwt: host rows (2026-11-03)

Host 1 (linux/arm64), `xo bench --ab --early`, n=6, Xo at cacd5306a. Go is github.com/golang-jwt/jwt/v5 v5.3.1 (bench/perf/jwt/twin.go.txt, vendored). Ratio is Xo wall over Go wall. Setup (key parsing, one signed token per algorithm) is in every workload.

| workload | Go ms | Xo ms | ratio | interval | verdict |
|---|---|---|---|---|---|
| hs256_sign (20,000) | 22.83 | 15.19 | 0.666 | 0.643-0.678 | better |
| hs256_verify (2,000) | 5.754 | 3.901 | 0.678 | 0.665-0.688 | better |
| rs256_verify (2,000) | 48.64 | 54.6 | 1.123 | 1.074-1.138 | worse |
| es256_verify (2,000) | 73.13 | 135.3 | 1.850 | 1.842-1.865 | worse |
| eddsa_verify (2,000) | 61.14 | 45.17 | 0.739 | 0.729-0.748 | better |

The two slow rows are the signature check, not the token code: `crypto.verify` for RSA (PKCS1v15, 2048 bit) and `crypto.verify_raw_ecdsa` for P-256 (std/crypto, Lang 1's area). Next: profile es256_verify (XO_PROF on host 1) to see whether it is the field arithmetic or the ECDSA verify's point multiplication; Go's P-256 has assembly (nistec) for both.

After "nistec verify: variable-time point multiplication": es256_verify 0.954 of before on host 1 (135.8 to 129.6 ms, A/B n=6), about 1.76 of Go. A mask select was a small part; the field multiplications dominate (nistec.mul4p 38 percent, sub_p256 18, pt_add256 14 in the Mac profile).
