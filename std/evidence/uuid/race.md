kind: race
module: uuid
result: pass
commit: f9aacc150
source: c3cacf57c4b2235e9fff8cf0544f04a5c6fba12342c82c322019488bc35be316
date: 2026-11-15
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" uuid
measure: 2 runs under --race on llvm, 0 data races, 0 failures, 13m9s

Race evidence (decision 0616): std/uuid's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus uuid: ok: 2 passed, 0 failed, 0 skipped, 3s
- stress program under --race, 64 tasks: uuid stress: 1473549264 operations by 64 tasks in 10 min, 0 mismatches, rss 79 MB at the mark, 80 MB at the end (1.0%), peak 80 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 268 B per finished task after 6 million (Go's race build: 252 B), 1.06 of Go's (at most 1.25); resident set at 6 million 2108 MB (Go's 1659 MB, shown), 2m52s, pass
