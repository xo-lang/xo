kind: race
module: textproto
result: pass
commit: d9e68db69
source: 3ee6c206038dafad864801f91bf39b968b19e5e2a161726cd9d33bc0552b0da8
date: 2026-11-17
host: linux/arm64 host 3
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 3" textproto
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 21m53s

Race evidence (decision 0616): std/textproto's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/textproto_test.xo): ok: 29 passed, 0 failed, 0 skipped, 6s
- corpus textproto: ok: 6 passed, 0 failed, 0 skipped, 9s
- stress program under --race, 64 tasks: textproto stress: 62905957 operations by 64 tasks in 10 min, 0 mismatches, rss 81 MB at the mark, 82 MB at the end (1.0%), peak 82 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 271 B per finished task after 6 million (Go's race build: 251 B), 1.08 of Go's (at most 1.25); resident set at 6 million 2081 MB (Go's 1702 MB, shown), 11m21s, pass
