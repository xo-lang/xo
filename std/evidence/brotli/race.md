kind: race
module: brotli
result: pass
commit: 914d52cd9
source: e65c9d538a438b75cb28cb2dd91ecc0c7cfe0fe30d021abde3bfbbfb0ca81883
date: 2026-11-17
host: linux/arm64 host 4
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 4" brotli
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 19m45s

Race evidence (decision 0616): std/brotli's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/brotli_test.xo): ok: 7 passed, 0 failed, 0 skipped, 55s
- corpus brotli: ok: 13 passed, 0 failed, 0 skipped, 9s
- stress program under --race, 64 tasks: brotli stress: 88749 operations by 64 tasks in 10 min, 0 mismatches, rss 4886 MB at the mark, 3600 MB at the end (-26.0%), peak 7056 MB, pass, 10m36s
- task churn under --race, 10 million tasks: 269 B per finished task after 6 million (Go's race build: 253 B), 1.07 of Go's (at most 1.25); resident set at 6 million 2054 MB (Go's 1744 MB, shown), 7m43s, pass
