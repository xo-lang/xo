kind: race
module: dns
result: pass
commit: 1340eb974
source: 0e43ddc0b0d3efd0e3857790c905c11e973bbbc5205444009b1d3fafc6e73c8c
date: 2026-11-17
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" dns
measure: 2 runs under --race on llvm, 0 data races, 0 failures, 13m6s

Race evidence (decision 0616): std/dns's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/dns_test.xo): ok: 8 passed, 0 failed, 0 skipped, 9s
- stress program under --race, 64 tasks: dns stress: 42599802 lookups by 64 clients in 10 min, 0 wrong answers, rss 204 MB at the mark, 293 MB at the end (43.0%), peak 293 MB, memory not judged under --race, 10m0s
- task churn under --race, 10 million tasks: 269 B per finished task after 6 million (Go's race build: 252 B), 1.07 of Go's (at most 1.25); resident set at 6 million 2106 MB (Go's 1668 MB, shown), 2m43s, pass
