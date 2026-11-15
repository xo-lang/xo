kind: race
module: deque
result: pass
commit: 39e2b1bbe
source: f17015181d59f9bb234d9c0e9dbd788aaa75883cc549df60d555ea829cb6c49a
date: 2026-11-15
host: linux/arm64
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64" deque
measure: 1 runs under --race on llvm, 0 data races, 0 failures, 2m35s

Race evidence (decision 0616): std/deque's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus deque: ok: 7 passed, 0 failed, 0 skipped, 3s
- task churn under --race, 10 million tasks: 274 B per finished task after 6 million (Go's race build: 253 B), 1.08 of Go's (at most 1.25); resident set at 6 million 2107 MB (Go's 1677 MB, shown), 2m28s, pass
