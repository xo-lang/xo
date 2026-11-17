kind: race
module: protodesc
result: pass
commit: d9e68db69
source: b9828a9cc7b15c4100efea24c9f87a9ad9d920925f75adcf0fe4e49dfcf01f9a
date: 2026-11-17
host: linux/arm64 host 3
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 3" protodesc
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 21m20s

Race evidence (decision 0616): std/protodesc's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/protodesc_test.xo): ok: 8 passed, 0 failed, 0 skipped, 5s
- corpus protodesc: ok: 4 passed, 0 failed, 0 skipped, 5s
- stress program under --race, 64 tasks: protodesc stress: 8888520 operations by 64 tasks in 10 min, 0 mismatches, rss 107 MB at the mark, 109 MB at the end (2.0%), peak 109 MB, pass, 10m2s
- task churn under --race, 10 million tasks: 277 B per finished task after 6 million (Go's race build: 248 B), 1.12 of Go's (at most 1.25); resident set at 6 million 2046 MB (Go's 1691 MB, shown), 10m56s, pass
