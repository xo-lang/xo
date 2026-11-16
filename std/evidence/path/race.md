kind: race
module: path
result: pass
commit: 52efc3d15
source: d98d36531b93880e3374d5e8b779068e1df0effc96136de85eb917178eba8218
date: 2026-11-16
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" path
measure: 4 runs under --race on llvm, 0 data races, 0 failures, 18m49s

Race evidence (decision 0616): std/path's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/path_test.xo): ok: 1 passed, 0 failed, 0 skipped, 5s
- corpus path: ok: 9 passed, 0 failed, 0 skipped, 8s
- corpus pathfuzz: ok: 1 passed, 0 failed, 0 skipped, 5s
- stress program under --race, 64 tasks: path stress: 5528325440 operations by 64 tasks in 10 min, 0 mismatches, rss 81 MB at the mark, 81 MB at the end (-0.0%), peak 84 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 273 B per finished task after 6 million (Go's race build: 259 B), 1.05 of Go's (at most 1.25); resident set at 6 million 2126 MB (Go's 1655 MB, shown), 8m0s, pass
