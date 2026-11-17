kind: race
module: regexp
result: pass
commit: 9b7287454
source: 008efc3ad36ad39b63b18d87046acd8dc10667ce6ad26d1716e408e813180590
date: 2026-11-17
host: linux/arm64 host 3
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 3" regexp
measure: 4 runs under --race on llvm, 0 data races, 0 failures, 17m30s

Race evidence (decision 0616): std/regexp's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/regexp_test.xo): ok: 5 passed, 0 failed, 0 skipped, 7s
- corpus regexp: ok: 9 passed, 0 failed, 0 skipped, 8s
- corpus regexpfuzz: ok: 1 passed, 0 failed, 0 skipped, 4s
- stress program under --race, 64 tasks: regexp stress: 19829600 operations by 64 tasks in 10 min, 0 mismatches, rss 114 MB at the mark, 117 MB at the end (3.0%), peak 119 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 271 B per finished task after 6 million (Go's race build: 254 B), 1.07 of Go's (at most 1.25); resident set at 6 million 2088 MB (Go's 1666 MB, shown), 6m52s, pass
