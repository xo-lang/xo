kind: race
module: sql
result: pass
commit: e5436be27
source: e720cac7167f2d745c857131fc6907c9a7444af55f6210d491fc6c110ee92fa3
date: 2026-11-16
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" sql
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 14m15s

Race evidence (decision 0616): std/sql's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/native/sql_test.xo): ok: 11 passed, 0 failed, 0 skipped, 12s
- corpus sql: ok: 29 passed, 0 failed, 0 skipped, 8s
- stress program under --race, 64 tasks: sql stress: 23968600 statements by 64 clients over 8 pools in 10 min, 0 mismatches, rss 139 MB at the mark, 139 MB at the end (-0.0%), peak 143 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 272 B per finished task after 6 million (Go's race build: 247 B), 1.10 of Go's (at most 1.25); resident set at 6 million 2111 MB (Go's 1668 MB, shown), 2m58s, pass
