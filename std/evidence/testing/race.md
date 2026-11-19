kind: race
module: testing
result: pass
commit: 03f2eaad1
source: 2f1df126f3e39aa5ed014752c9f3877759a031665d82d6477781e4b602add083
date: 2026-11-19
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" testing
measure: 2 runs under --race on llvm, 0 data races, 0 failures, 12m0s

Race evidence (decision 0616): std/testing's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/testing_test.xo): ok: 14 passed, 0 failed, 0 skipped, 5s
- stress program under --race, 64 tasks: testing stress: 400 rounds of every fake, then 155280 rounds of MemFs and FakeEnv (9937920 task rounds) by 64 tasks in 540 s, 0 mismatches, memory level from half way, pass, 9m11s
- task churn under --race, 10 million tasks: 267 B per finished task after 6 million (Go's race build: 253 B), 1.05 of Go's (at most 1.25); resident set at 6 million 2101 MB (Go's 1664 MB, shown), 2m33s, pass
