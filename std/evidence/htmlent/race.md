kind: race
module: htmlent
result: pass
commit: 52efc3d15
source: 4f9a032d3b0a0ee5bf26a6615f0fe4876054f639cda3edb92e682393cb61a702
date: 2026-11-16
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" htmlent
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 16m0s

Race evidence (decision 0616): std/htmlent's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/htmlent_test.xo): ok: 8 passed, 0 failed, 0 skipped, 12s
- corpus htmlentfuzz: ok: 2 passed, 0 failed, 0 skipped, 14s
- stress program under --race, 64 tasks: htmlent stress: 753454464 operations by 64 tasks in 10 min, 0 mismatches, rss 70 MB at the mark, 70 MB at the end (0.0%), peak 73 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 267 B per finished task after 6 million (Go's race build: 253 B), 1.05 of Go's (at most 1.25); resident set at 6 million 2131 MB (Go's 1687 MB, shown), 4m50s, pass
