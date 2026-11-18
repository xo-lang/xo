kind: race
module: protowire
result: pass
commit: a9af4426e
source: 766a1f88c93b86eb37a3d7e05959cdc7927e0bbc9d60037f047e2ce1c928b002
date: 2026-11-18
host: linux/arm64 host 3
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 3" protowire
measure: 2 runs under --race on llvm, 0 data races, 0 failures, 16m22s

Race evidence (decision 0616): std/protowire's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/protowire_test.xo): ok: 3 passed, 0 failed, 0 skipped, 4s
- stress program under --race, 64 tasks: protowire stress: 39278848 operations by 64 tasks in 10 min, 0 mismatches, rss 166 MB at the mark (median of minutes 5 to 7), 167 MB at the end (median of the last 3 minutes; 1.0%), peak 168 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 270 B per finished task after 6 million (Go's race build: 252 B), 1.07 of Go's (at most 1.25); resident set at 6 million 2041 MB (Go's 1671 MB, shown), 6m10s, pass
