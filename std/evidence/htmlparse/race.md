kind: race
module: htmlparse
result: pass
commit: d9e68db69
source: caa2ebbbe5295ecae4fc68ae0a89aefcbb5d09d0dadc835b1b8162d295bca099
date: 2026-11-17
host: linux/arm64 host 3
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 3" htmlparse
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 18m2s

Race evidence (decision 0616): std/htmlparse's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/htmlparse_test.xo): ok: 18 passed, 0 failed, 0 skipped, 9s
- corpus htmlparse: ok: 6 passed, 0 failed, 0 skipped, 11s
- stress program under --race, 64 tasks: htmlparse stress: 139749696 operations by 64 tasks in 10 min, 0 mismatches, rss 86 MB at the mark, 87 MB at the end (0.0%), peak 89 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 266 B per finished task after 6 million (Go's race build: 252 B), 1.06 of Go's (at most 1.25); resident set at 6 million 2018 MB (Go's 1703 MB, shown), 7m13s, pass
