kind: race
module: sse
result: pass
commit: 67b6f3285
source: 3afcdd65c4152d9f7d58c161b33fed652df7945be31a3d62a78a5d80fae1b26e
date: 2026-11-18
host: linux/amd64 host
race runtime: xo's (lib/xo/race/linux-amd64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/amd64 host" sse
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 22m10s

Race evidence (decision 0616): std/sse's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/sse_test.xo): ok: 20 passed, 0 failed, 0 skipped, 53s
- corpus sse: ok: 94 passed, 0 failed, 0 skipped, 1m0s
- stress program under --race, 64 tasks: sse stress: 111736755 events by 64 subscribers in 10 min over 5520377 streams, every event once and in order, rss 173 MB at the mark (median of minutes 5 to 7), 171 MB at the end (median of the last 3 minutes; -1.0%), peak 178 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 267 B per finished task after 6 million (Go's race build: 250 B), 1.07 of Go's (at most 1.25); resident set at 6 million 2271 MB (Go's 1673 MB, shown), 7m53s, pass
