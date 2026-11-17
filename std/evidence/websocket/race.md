kind: race
module: websocket
result: pass
commit: 32c89673e
source: 4b49bceb3cc0b43eea1f6b17d8f65e385955c9159e3e387f47130a40eb851b7d
date: 2026-11-17
host: linux/arm64 host 4
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 4" websocket
measure: 2 runs under --race on llvm, 0 data races, 0 failures, 18m20s

Race evidence (decision 0616): std/websocket's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/websocket_test.xo): ok: 5 passed, 0 failed, 0 skipped, 44s
- stress program under --race, 64 tasks: websocket stress: 1716762 messages echoed by 64 clients in 10 min, 0 mismatches, rss 314 MB at the mark, 329 MB at the end (5.0%), peak 331 MB, pass, 10m3s
- task churn under --race, 10 million tasks: 269 B per finished task after 6 million (Go's race build: 254 B), 1.06 of Go's (at most 1.25); resident set at 6 million 2081 MB (Go's 1712 MB, shown), 6m51s, pass
