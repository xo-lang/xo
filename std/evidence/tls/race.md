kind: race
module: tls
result: pass
commit: d10c5bae7
source: 28eb37c08e2faf1e67580df7bcce4828ae10967762aa8e5efdd59020a693c553
date: 2026-11-17
host: linux/arm64
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64" tls
measure: 2 runs under --race on llvm, 0 data races, 0 failures, 15m8s

Race evidence (decision 0616): std/tls's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/native/tls_test.xo): ok: 2 passed, 0 failed, 0 skipped, 11s
- stress program under --race, 64 tasks: tls stress: 149724 connections (74846 resumed), 11454 MiB echoed by 64 clients in 10 min, 0 mismatches, rss 254 MB at the mark, 278 MB at the end (9.0%), peak 278 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 270 B per finished task after 6 million (Go's race build: 249 B), 1.08 of Go's (at most 1.25); resident set at 6 million 2139 MB (Go's 1664 MB, shown), 3m49s, pass
