kind: race
module: htmltok
result: pass
commit: 9e25a6645
source: cf5697dc818c7ef93b838ac9cb3ad1fe6ac8b0ab67d555e21b425139a33cc611
date: 2026-11-17
host: linux/arm64 host 4
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 4" htmltok
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 20m57s

Race evidence (decision 0616): std/htmltok's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/htmltok_test.xo): ok: 8 passed, 0 failed, 0 skipped, 6s
- corpus htmltok: ok: 6 passed, 0 failed, 0 skipped, 6s
- stress program under --race, 64 tasks: htmltok stress: 481798912 operations by 64 tasks in 10 min, 0 mismatches, rss 74 MB at the mark, 75 MB at the end (0.0%), peak 75 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 309 B per finished task after 6 million (Go's race build: 255 B), 1.21 of Go's (at most 1.25); resident set at 6 million 1952 MB (Go's 1686 MB, shown), 9m59s, pass
