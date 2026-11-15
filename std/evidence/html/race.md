kind: race
module: html
result: pass
commit: eb7b7300e
source: 5747b639da584a7ec48020992f3b0645191623cb176c4d06ef908f4d699be444
date: 2026-11-14
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" html
measure: 2 runs under --race on llvm, 0 data races, 0 failures, 17m11s

Race evidence (decision 0616): std/html's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus htmlfuzz: ok: 2 passed, 0 failed, 0 skipped, 6s
- stress program under --race, 64 tasks: html stress: 1246291584 operations by 64 tasks in 10 min, 0 mismatches, rss 83 MB at the mark, 84 MB at the end (1.0%), peak 88 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 282 B per finished task after 6 million (Go's race build: 252 B), 1.12 of Go's (at most 1.25); resident set at 6 million 2183 MB (Go's 1687 MB, shown), 6m46s, pass
