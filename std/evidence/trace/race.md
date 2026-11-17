kind: race
module: trace
result: pass
commit: 9e25a6645
source: fbc31db785aae56fc2129a2b31f4c66ae8c049b87739a1adeb7047b1468b78c9
date: 2026-11-17
host: linux/arm64 host 4
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 4" trace
measure: 2 runs under --race on llvm, 0 data races, 0 failures, 23m51s

Race evidence (decision 0616): std/trace's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/trace_test.xo): ok: 4 passed, 0 failed, 1 skipped (1 uses clock, rand: run with --real), 7s
- stress program under --race, 64 tasks: trace stress: 1088043 spans recorded and 31961040 read back by 64 tasks in 10 min, 0 promises broken, rss 93 MB at the mark, 87 MB at the end (-7.0%), peak 98 MB, pass, 10m2s
- task churn under --race, 10 million tasks: 271 B per finished task after 6 million (Go's race build: 251 B), 1.08 of Go's (at most 1.25); resident set at 6 million 2116 MB (Go's 1683 MB, shown), 12m37s, pass
