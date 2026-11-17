kind: race
module: asn1
result: pass
commit: 9b7287454
source: 88ff0df8b850c2eeb1e60b0afe9cca8bef340ab4ec6f8c8866eb07bbd8425eff
date: 2026-11-17
host: linux/arm64 host 4
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 4" asn1
measure: 4 runs under --race on llvm, 0 data races, 0 failures, 18m9s

Race evidence (decision 0616): std/asn1's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/asn1_test.xo): ok: 11 passed, 0 failed, 0 skipped, 11s
- corpus asn1: ok: 4 passed, 0 failed, 0 skipped, 14s
- corpus asn1fields: ok: 2 passed, 0 failed, 0 skipped, 18s
- stress program under --race, 64 tasks: asn1 stress: 146650449 values by 64 tasks in 10 min, 0 identities broken, rss 84 MB at the mark, 85 MB at the end (1.0%), peak 93 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 270 B per finished task after 6 million (Go's race build: 247 B), 1.09 of Go's (at most 1.25); resident set at 6 million 1980 MB (Go's 1695 MB, shown), 7m10s, pass
