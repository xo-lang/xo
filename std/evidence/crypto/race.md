kind: race
module: crypto
result: pass
commit: 1b73055b4
source: 01b80d8d3f039f174066dcfc227741aa9e3cb4c2e37434b5ed4e914ee4de948f
date: 2026-11-16
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" crypto
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 17m34s

Race evidence (decision 0616): std/crypto's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/native/crypto_test.xo): ok: 30 passed, 0 failed, 0 skipped, 17s
- corpus crypto: ok: 31 passed, 0 failed, 0 skipped, 12s
- stress program under --race, 64 tasks: crypto stress: 38231680 operations by 64 tasks in 10 min, 0 mismatches, rss 100 MB at the mark, 101 MB at the end (1.0%), peak 101 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 286 B per finished task after 6 million (Go's race build: 252 B), 1.13 of Go's (at most 1.25); resident set at 6 million 2165 MB (Go's 1678 MB, shown), 6m27s, pass
