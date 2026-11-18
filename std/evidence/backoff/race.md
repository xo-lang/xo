kind: race
module: backoff
result: pass
commit: a9af4426e
source: ed36ec73bdcd2752842d9887f3d675a6bdaf916ac7e4d52ab04f8b6a00b4b691
date: 2026-11-18
host: linux/arm64 host 3
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 3" backoff
measure: 4 runs under --race on llvm, 0 data races, 0 failures, 17m21s

Race evidence (decision 0616): std/backoff's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/backoff_test.xo): ok: 7 passed, 0 failed, 0 skipped, 3s
- corpus backoff: ok: 10 passed, 0 failed, 0 skipped, 3s
- corpus backofffuzz: ok: 1 passed, 0 failed, 0 skipped, 5s
- stress program under --race, 64 tasks: backoff stress: 3311153445 operations by 64 tasks in 10 min, 0 mismatches, rss 78 MB at the mark (median of minutes 5 to 7), 77 MB at the end (median of the last 3 minutes; -1.0%), peak 79 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 287 B per finished task after 6 million (Go's race build: 253 B), 1.13 of Go's (at most 1.25); resident set at 6 million 1926 MB (Go's 1657 MB, shown), 6m51s, pass
