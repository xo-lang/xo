kind: race
module: metrics
result: pass
commit: 2fe5c76b4
source: e4f884194f350b38ba4f4eb4129002b9e0d3ffa680b0da920c00ad97a3b0ac51
date: 2026-11-18
host: linux/amd64 host
race runtime: xo's (lib/xo/race/linux-amd64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/amd64 host" metrics
measure: 4 runs under --race on llvm, 0 data races, 0 failures, 20m55s

Race evidence (decision 0616): std/metrics's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/metrics_test.xo): ok: 40 passed, 0 failed, 0 skipped, 1m6s
- corpus metrics: ok: 14 passed, 0 failed, 4 skipped (4 uses clock, fs, net, proc: run with --real), 7s
- corpus metricsfuzz: ok: 60 passed, 0 failed, 0 skipped, 6s
- stress program under --race, 64 tasks: metrics stress: 22083684 operations by 64 tasks in 10 min, 0 mismatches, rss 127 MB at the mark (median of minutes 5 to 7), 128 MB at the end (median of the last 3 minutes; 1.0%), peak 130 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 265 B per finished task after 6 million (Go's race build: 252 B), 1.05 of Go's (at most 1.25); resident set at 6 million 2267 MB (Go's 1665 MB, shown), 8m53s, pass
