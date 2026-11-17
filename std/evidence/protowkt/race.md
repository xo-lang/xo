kind: race
module: protowkt
result: pass
commit: d9e68db69
source: 520db0db4a0c5fe2ddd22c45a3b35070f01cbb7411037af5b533c8c707db3687
date: 2026-11-17
host: linux/arm64 host 3
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 3" protowkt
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 21m22s

Race evidence (decision 0616): std/protowkt's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/protowkt_test.xo): ok: 20 passed, 0 failed, 0 skipped, 4s
- corpus protowkt: ok: 18 passed, 0 failed, 0 skipped, 6s
- stress program under --race, 64 tasks: protowkt stress: 1305707 operations by 64 tasks in 10 min, 0 mismatches, rss 78 MB at the mark, 78 MB at the end (1.0%), peak 86 MB, pass, 10m2s
- task churn under --race, 10 million tasks: 276 B per finished task after 6 million (Go's race build: 261 B), 1.06 of Go's (at most 1.25); resident set at 6 million 2055 MB (Go's 1690 MB, shown), 10m55s, pass
