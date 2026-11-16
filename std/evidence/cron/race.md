kind: race
module: cron
result: pass
commit: 52efc3d15
source: 12b4ff0f4070fc871ad8adc11f03f5180e8b49b3127b6abb09ccf6f2365e5998
date: 2026-11-16
host: linux/arm64 host 4
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 4" cron
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 16m44s

Race evidence (decision 0616): std/cron's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus cron: ok: 5 passed, 0 failed, 0 skipped, 5s
- corpus cronfuzz: ok: 1 passed, 0 failed, 0 skipped, 4s
- stress program under --race, 64 tasks: cron stress: 405796352 operations by 64 tasks in 10 min, 0 mismatches, rss 76 MB at the mark, 76 MB at the end (1.0%), peak 78 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 274 B per finished task after 6 million (Go's race build: 257 B), 1.07 of Go's (at most 1.25); resident set at 6 million 1972 MB (Go's 1698 MB, shown), 6m12s, pass
