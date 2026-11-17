kind: race
module: httpproxy
result: pass
commit: a9fd5864b
source: 47f3f7a32ce38b1a5241de714188ecd43c087db23bad94c273614fe3e7ca0191
date: 2026-11-17
host: linux/arm64
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64" httpproxy
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 15m41s

Race evidence (decision 0616): std/httpproxy's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/httpproxy_test.xo): ok: 6 passed, 0 failed, 1 skipped (1 uses env: run with --real), 4s
- corpus httpproxy: ok: 1 passed, 0 failed, 0 skipped, 4s
- stress program under --race, 64 tasks: httpproxy stress: 521122816 lookups by 64 tasks in 10 min over 64 configurations (11721 of 16384 answers a proxy), every answer as recorded, rss 81 MB at the mark, 82 MB at the end (0.0%), peak 82 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 287 B per finished task after 6 million (Go's race build: 250 B), 1.15 of Go's (at most 1.25); resident set at 6 million 2162 MB (Go's 1698 MB, shown), 5m23s, pass
