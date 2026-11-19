kind: race
module: htmltok
result: pass
commit: 10b428ec2
source: cf5697dc818c7ef93b838ac9cb3ad1fe6ac8b0ab67d555e21b425139a33cc611
date: 2026-11-19
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" htmltok
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 14m16s

Race evidence (decision 0616): std/htmltok's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/htmltok_test.xo): ok: 8 passed, 0 failed, 0 skipped, 4s
- corpus htmltok: ok: 6 passed, 0 failed, 0 skipped, 4s
- stress program under --race, 64 tasks: htmltok stress: 994445696 operations by 64 tasks in 10 min, 0 mismatches, rss 81 MB at the mark (median of minutes 5 to 7), 81 MB at the end (median of the last 3 minutes; 1.0%), peak 83 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 274 B per finished task after 6 million (Go's race build: 249 B), 1.10 of Go's (at most 1.25); resident set at 6 million 2166 MB (Go's 1686 MB, shown), 4m0s, pass
