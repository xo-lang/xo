kind: race
module: rate
result: pass
commit: d2f4e7864
source: 040d9b549cc5779ce322eb5eb777949c947f038704d1cd7783e2a5b6ce6ce104
date: 2026-11-17
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" rate
measure: 4 runs under --race on llvm, 0 data races, 0 failures, 17m35s

Race evidence (decision 0616): std/rate's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/rate_test.xo): ok: 8 passed, 0 failed, 0 skipped, 5s
- corpus rate: ok: 18 passed, 0 failed, 0 skipped, 5s
- corpus ratefuzz: ok: 1 passed, 0 failed, 0 skipped, 6s
- stress program under --race, 64 tasks: rate stress: 1046253511 operations by 64 tasks in 10 min, 0 mismatches, rss 66 MB at the mark, 67 MB at the end (1.0%), peak 67 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 272 B per finished task after 6 million (Go's race build: 253 B), 1.07 of Go's (at most 1.25); resident set at 6 million 2203 MB (Go's 1674 MB, shown), 6m55s, pass
