kind: race
module: timego
result: pass
commit: 10b428ec2
source: 2ed61c4acaf43c0f6f8b0dc34e8768e9c7b99c8d934291294dce8d33a8acdcbf
date: 2026-11-19
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" timego
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 12m20s

Race evidence (decision 0616): std/timego's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/timego_test.xo): ok: 4 passed, 0 failed, 0 skipped, 3s
- corpus timego: ok: 2 passed, 0 failed, 0 skipped, 3s
- stress program under --race, 64 tasks: timego stress: 1160073472 operations by 64 tasks in 10 min, 0 mismatches, rss 78 MB at the mark (median of minutes 5 to 7), 78 MB at the end (median of the last 3 minutes; 0.0%), peak 81 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 271 B per finished task after 6 million (Go's race build: 254 B), 1.07 of Go's (at most 1.25); resident set at 6 million 2094 MB (Go's 1678 MB, shown), 2m8s, pass
