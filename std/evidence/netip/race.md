kind: race
module: netip
result: pass
commit: d2f4e7864
source: 6aeea21f62d1abfcd2937ac1d0793408dede9586a9305b2d1c3514244e07034b
date: 2026-11-17
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" netip
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 19m10s

Race evidence (decision 0616): std/netip's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/netip_test.xo): ok: 3 passed, 0 failed, 0 skipped, 10s
- corpus netip: ok: 10 passed, 0 failed, 0 skipped, 15s
- stress program under --race, 64 tasks: netip stress: 85656467 addresses by 64 tasks in 10 min, 0 identities broken, rss 92 MB at the mark, 93 MB at the end (1.0%), peak 94 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 276 B per finished task after 6 million (Go's race build: 251 B), 1.10 of Go's (at most 1.25); resident set at 6 million 2097 MB (Go's 1678 MB, shown), 8m15s, pass
