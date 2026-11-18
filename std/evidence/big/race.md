kind: race
module: big
result: pass
commit: 745cbde2b
source: 276f4d1d331a09cc6f16824f37afbf73047651917b08e6db6c18a17a6102b468
date: 2026-11-18
host: linux/arm64 host 3
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 3" big
measure: 5 runs under --race on llvm, 0 data races, 0 failures, 19m28s

Race evidence (decision 0616): std/big's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/big_test.xo): ok: 2 passed, 0 failed, 0 skipped, 8s
- corpus big: ok: 4 passed, 0 failed, 0 skipped, 9s
- corpus bigfloat: ok: 21 passed, 0 failed, 0 skipped, 12s
- corpus bigrat: ok: 3 passed, 0 failed, 0 skipped, 11s
- stress program under --race, 64 tasks: big stress: 187675200 operations by 64 tasks in 10 min, 0 mismatches, rss 87 MB at the mark (median of minutes 5 to 7), 86 MB at the end (median of the last 3 minutes; -0.0%), peak 88 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 272 B per finished task after 6 million (Go's race build: 249 B), 1.09 of Go's (at most 1.25); resident set at 6 million 2037 MB (Go's 1694 MB, shown), 8m40s, pass
