kind: race
module: image
result: pass
commit: 9b7287454
source: 7c927933e66698de2e6b0ba3efbbb523cef4a1b0fb11209429cc1b51bd881c0c
date: 2026-11-17
host: linux/arm64 host 3
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 3" image
measure: 2 runs under --race on llvm, 0 data races, 0 failures, 18m59s

Race evidence (decision 0616): std/image's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus image: ok: 24 passed, 0 failed, 0 skipped, 9s
- stress program under --race, 64 tasks: image stress: 2807870400 operations by 64 tasks in 10 min, 0 mismatches, rss 72 MB at the mark, 72 MB at the end (1.0%), peak 77 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 303 B per finished task after 6 million (Go's race build: 251 B), 1.21 of Go's (at most 1.25); resident set at 6 million 2026 MB (Go's 1685 MB, shown), 8m42s, pass
