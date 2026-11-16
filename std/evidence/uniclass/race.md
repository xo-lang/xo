kind: race
module: uniclass
result: pass
commit: 52efc3d15
source: 87fb24873656d07b207bb28a302d371e8afbc3bba56ba417169d9968e843c66e
date: 2026-11-16
host: linux/arm64 host 4
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 4" uniclass
measure: 2 runs under --race on llvm, 0 data races, 0 failures, 17m9s

Race evidence (decision 0616): std/uniclass's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus uniclassfuzz: ok: 1 passed, 0 failed, 0 skipped, 5s
- stress program under --race, 64 tasks: uniclass stress: 998575104 operations by 64 tasks in 10 min, 0 mismatches, rss 88 MB at the mark, 83 MB at the end (-6.0%), peak 90 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 280 B per finished task after 6 million (Go's race build: 250 B), 1.12 of Go's (at most 1.25); resident set at 6 million 1984 MB (Go's 1696 MB, shown), 6m33s, pass
