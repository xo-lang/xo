kind: race
module: csv
result: pass
commit: 131b13592
source: 826a97e2e454795e6cfe7712d9c7783b379229ab648877cb2ad9a6777ec0029c
date: 2026-11-15
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" csv
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 15m31s

Race evidence (decision 0616): std/csv's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus csv: ok: 11 passed, 0 failed, 0 skipped, 12s
- corpus csvfuzz: ok: 2000 passed, 0 failed, 0 skipped, 12s
- stress program under --race, 64 tasks: csv stress: 838259568 operations by 64 tasks in 10 min, 0 mismatches, rss 84 MB at the mark, 86 MB at the end (3.0%), peak 94 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 283 B per finished task after 6 million (Go's race build: 258 B), 1.10 of Go's (at most 1.25); resident set at 6 million 2167 MB (Go's 1645 MB, shown), 4m51s, pass
