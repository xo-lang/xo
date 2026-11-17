kind: race
module: charset
result: pass
commit: 9b7287454
source: 0406c05bb9ad615c3799e31356da05ce77f23d292f7a1e605836330c588ed86c
date: 2026-11-17
host: linux/arm64 host 3
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 3" charset
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 18m45s

Race evidence (decision 0616): std/charset's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/charset_test.xo): ok: 6 passed, 0 failed, 0 skipped, 3s
- corpus charset: ok: 12 passed, 0 failed, 0 skipped, 5s
- stress program under --race, 64 tasks: charset stress: 1429154560 operations by 64 tasks in 10 min, 0 mismatches, rss 75 MB at the mark, 73 MB at the end (-3.0%), peak 76 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 275 B per finished task after 6 million (Go's race build: 250 B), 1.10 of Go's (at most 1.25); resident set at 6 million 2044 MB (Go's 1720 MB, shown), 8m28s, pass
