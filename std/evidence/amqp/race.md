kind: race
module: amqp
result: pass
commit: a9fd5864b
source: 579e29f6087c6688d584d0b4fcbcc797f4f844228ef29ad2fa479e6aa30c87b4
date: 2026-11-17
host: linux/arm64
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64" amqp
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 18m53s

Race evidence (decision 0616): std/amqp's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/amqp_test.xo): ok: 34 passed, 0 failed, 0 skipped, 1m4s
- corpus amqp: ok: 4 passed, 0 failed, 0 skipped, 5s
- stress program under --race, 64 tasks: amqp stress: 933797 rounds by 64 tasks in 10 min, every message back as sent, rss 336 MB at the mark, 332 MB at the end (-1.0%), peak 341 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 275 B per finished task after 6 million (Go's race build: 250 B), 1.10 of Go's (at most 1.25); resident set at 6 million 2127 MB (Go's 1684 MB, shown), 6m9s, pass
