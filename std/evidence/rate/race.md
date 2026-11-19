kind: race
module: rate
result: pass
commit: 10b428ec2
source: eede7c3d501b5b87715b454ed321faf04dd5b4afd759c33507da498fbf2ebc01
date: 2026-11-19
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" rate
measure: 4 runs under --race on llvm, 0 data races, 0 failures, 12m25s

Race evidence (decision 0616): std/rate's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/rate_test.xo): ok: 8 passed, 0 failed, 0 skipped, 3s
- corpus rate: ok: 18 passed, 0 failed, 0 skipped, 3s
- corpus ratefuzz: ok: 1 passed, 0 failed, 0 skipped, 3s
- stress program under --race, 64 tasks: rate stress: 3315264762 operations by 64 tasks in 10 min, 0 mismatches, rss 90 MB at the mark (median of minutes 5 to 7), 90 MB at the end (median of the last 3 minutes; 1.0%), peak 91 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 272 B per finished task after 6 million (Go's race build: 252 B), 1.08 of Go's (at most 1.25); resident set at 6 million 2068 MB (Go's 1680 MB, shown), 2m10s, pass
