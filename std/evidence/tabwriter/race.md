kind: race
module: tabwriter
result: pass
commit: 9b7287454
source: 3c8c7d20fed3259169c9750ba55aabe00a7dc7c4e8dd820355f9d4221827d6a0
date: 2026-11-17
host: linux/arm64 host 3
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 3" tabwriter
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 19m1s

Race evidence (decision 0616): std/tabwriter's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus tabwriter: ok: 2 passed, 0 failed, 0 skipped, 4s
- corpus tabwriterfuzz: ok: 1 passed, 0 failed, 0 skipped, 5s
- stress program under --race, 64 tasks: tabwriter stress: 193651200 operations by 64 tasks in 10 min, 0 mismatches, rss 81 MB at the mark, 82 MB at the end (1.0%), peak 82 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 277 B per finished task after 6 million (Go's race build: 253 B), 1.09 of Go's (at most 1.25); resident set at 6 million 2047 MB (Go's 1698 MB, shown), 8m43s, pass
