kind: race
module: log
result: pass
commit: 9b7287454
source: 9980e5649ada9d0f34ff184ecc55d0b585a40fcb83e64b1e46c07b95fe4c6c04
date: 2026-11-17
host: linux/arm64 host 3
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 3" log
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 16m58s

Race evidence (decision 0616): std/log's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/log_test.xo): ok: 1 passed, 0 failed, 0 skipped, 3s
- corpus logfuzz: ok: 1 passed, 0 failed, 0 skipped, 4s
- stress program under --race, 64 tasks: log stress: 115734900 operations by 64 tasks in 10 min, 0 mismatches, rss 87 MB at the mark, 90 MB at the end (3.0%), peak 92 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 276 B per finished task after 6 million (Go's race build: 256 B), 1.08 of Go's (at most 1.25); resident set at 6 million 2043 MB (Go's 1667 MB, shown), 6m40s, pass
