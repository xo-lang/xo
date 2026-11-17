kind: race
module: udp
result: pass
commit: afaa93e8f
source: 7ecb96d6818c7cb5e9cfc5857e91592323fe38cb6afd8e04e7a88286294f3be0
date: 2026-11-17
host: linux/arm64 host 4
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 4" udp
measure: 2 runs under --race on llvm, 0 data races, 0 failures, 16m42s

Race evidence (decision 0616): std/udp's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/udp_test.xo): ok: 10 passed, 0 failed, 0 skipped, 17s
- stress program under --race, 64 tasks: udp stress: 14941073 datagrams echoed by 64 clients in 10 min, 0 dropped, 0 mismatches, rss 86 MB at the mark, 87 MB at the end (1.0%), peak 91 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 277 B per finished task after 6 million (Go's race build: 252 B), 1.10 of Go's (at most 1.25); resident set at 6 million 2011 MB (Go's 1672 MB, shown), 5m52s, pass
