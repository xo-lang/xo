kind: race
module: amqptest
result: pass
commit: a9fd5864b
source: 78f9db079482fe4033c0a16b6cbe28f5c750d0aba79b37c730c901381ee13f71
date: 2026-11-17
host: linux/arm64
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64" amqptest
measure: 2 runs under --race on llvm, 0 data races, 0 failures, 23m28s

Race evidence (decision 0616): std/amqptest's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/amqptest_test.xo): ok: 35 passed, 0 failed, 0 skipped, 1m35s
- stress program under --race, 64 tasks: amqptest stress: 96899 messages by 64 producers, 96166 accepted once and 733 dead lettered by 64 consumers in 10 min, none lost, rss 384 MB at the mark, 416 MB at the end (8.0%), peak 468 MB, pass, 13m52s
- task churn under --race, 10 million tasks: 276 B per finished task after 6 million (Go's race build: 252 B), 1.10 of Go's (at most 1.25); resident set at 6 million 2160 MB (Go's 1705 MB, shown), 6m45s, pass
