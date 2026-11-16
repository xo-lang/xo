kind: race
module: deque
result: pass
commit: 52efc3d15
source: f17015181d59f9bb234d9c0e9dbd788aaa75883cc549df60d555ea829cb6c49a
date: 2026-11-16
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" deque
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 18m57s

Race evidence (decision 0616): std/deque's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus deque: ok: 7 passed, 0 failed, 0 skipped, 8s
- corpus dequefuzz: ok: 1 passed, 0 failed, 0 skipped, 6s
- stress program under --race, 64 tasks: deque stress: 6156529962 operations by 64 tasks in 10 min, 0 mismatches, rss 96 MB at the mark, 97 MB at the end (1.0%), peak 102 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 275 B per finished task after 6 million (Go's race build: 253 B), 1.09 of Go's (at most 1.25); resident set at 6 million 2170 MB (Go's 1673 MB, shown), 8m15s, pass
