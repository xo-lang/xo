kind: race
module: time
result: pass
commit: a0c61aef6
source: 615bb2e80abc81996374c87057a298c015893ddcb4f2d2118bbf142de8e94fba
date: 2026-11-14
host: linux/arm64
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64" time
measure: 4 runs under --race on llvm, 0 data races, 0 failures, 14m50s

Race evidence (decision 0616): std/time's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/native/time_test.xo): ok: 12 passed, 0 failed, 0 skipped, 3s
- corpus time: ok: 17 passed, 0 failed, 0 skipped, 4s
- corpus timefuzz: ok: 2 passed, 0 failed, 0 skipped, 3s
- stress program under --race, 64 tasks: time stress: 1658925568 operations by 64 tasks in 10 min, 0 mismatches, rss 78 MB at the mark, 79 MB at the end (1.0%), peak 79 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 265 B per finished task after 6 million (Go's race build: 253 B), 1.05 of Go's (at most 1.25); resident set at 6 million 2007 MB (Go's 1667 MB, shown), 4m28s, pass
