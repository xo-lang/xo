kind: race
module: migrate
result: pass
commit: d10c5bae7
source: 4fd7c579cbb58174b82e9abf5cf798c42926ca0c6164711fec31cceea7aaacd5
date: 2026-11-17
host: linux/arm64 host 3
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 3" migrate
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 20m26s

Race evidence (decision 0616): std/migrate's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/migrate_test.xo): ok: 23 passed, 0 failed, 0 skipped, 9s
- corpus migrate: ok: 4 passed, 0 failed, 0 skipped, 9s
- stress program under --race, 64 tasks: migrate stress: 21608664 operations by 64 tasks in 10 min, 0 mismatches, rss 92 MB at the mark, 91 MB at the end (-0.0%), peak 94 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 274 B per finished task after 6 million (Go's race build: 250 B), 1.10 of Go's (at most 1.25); resident set at 6 million 2073 MB (Go's 1688 MB, shown), 9m44s, pass
