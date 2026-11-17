kind: race
module: timego
result: pass
commit: 9e25a6645
source: 78fa66e93943b04c6493f4560aec422d9f5584624f8984e9e6780ea9a3fcd2c2
date: 2026-11-17
host: linux/arm64 host 4
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 4" timego
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 20m59s

Race evidence (decision 0616): std/timego's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/timego_test.xo): ok: 4 passed, 0 failed, 0 skipped, 4s
- corpus timego: ok: 2 passed, 0 failed, 0 skipped, 8s
- stress program under --race, 64 tasks: timego stress: 474155136 operations by 64 tasks in 10 min, 0 mismatches, rss 70 MB at the mark, 70 MB at the end (0.0%), peak 70 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 277 B per finished task after 6 million (Go's race build: 252 B), 1.10 of Go's (at most 1.25); resident set at 6 million 2013 MB (Go's 1685 MB, shown), 10m5s, pass
