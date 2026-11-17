kind: race
module: xmlcodec
result: pass
commit: d9e68db69
source: b01096b6b9bb64d1454849936dd115c187d34647ac34d698ec8d6d6f91801f2b
date: 2026-11-17
host: linux/arm64 host 3
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 3" xmlcodec
measure: 2 runs under --race on llvm, 0 data races, 0 failures, 18m17s

Race evidence (decision 0616): std/xmlcodec's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus xmlcodec: ok: 7 passed, 0 failed, 0 skipped, 9s
- stress program under --race, 64 tasks: xmlcodec stress: 73872000 operations by 64 tasks in 10 min, 0 mismatches, rss 93 MB at the mark, 94 MB at the end (2.0%), peak 96 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 270 B per finished task after 6 million (Go's race build: 253 B), 1.07 of Go's (at most 1.25); resident set at 6 million 2012 MB (Go's 1677 MB, shown), 7m51s, pass
