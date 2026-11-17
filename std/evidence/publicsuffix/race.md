kind: race
module: publicsuffix
result: pass
commit: 9b7287454
source: 04450658eed33d05e18b444831a7f52fd0fe19129103f2e0a0f1278a8b81b873
date: 2026-11-17
host: linux/arm64 host 4
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 4" publicsuffix
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 17m53s

Race evidence (decision 0616): std/publicsuffix's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/publicsuffix_test.xo): ok: 3 passed, 0 failed, 0 skipped, 6s
- corpus publicsuffix: ok: 1 passed, 0 failed, 0 skipped, 6s
- stress program under --race, 64 tasks: publicsuffix stress: 793216000 operations by 64 tasks in 10 min, 0 mismatches, rss 74 MB at the mark, 75 MB at the end (1.0%), peak 75 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 268 B per finished task after 6 million (Go's race build: 248 B), 1.08 of Go's (at most 1.25); resident set at 6 million 1965 MB (Go's 1689 MB, shown), 7m24s, pass
