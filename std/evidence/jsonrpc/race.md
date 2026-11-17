kind: race
module: jsonrpc
result: pass
commit: d10c5bae7
source: c33b3a8a8f186bd359ca52c450aa2b15ac01e66233d5a45b9e5df644bf394594
date: 2026-11-17
host: linux/arm64
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64" jsonrpc
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 17m16s

Race evidence (decision 0616): std/jsonrpc's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/jsonrpc_test.xo): ok: 1 passed, 0 failed, 0 skipped, 3s
- corpus jsonrpc: ok: 139 passed, 0 failed, 0 skipped, 4s
- stress program under --race, 64 tasks: jsonrpc stress: 3566398 calls by 128 tasks in 10 min, 222889 notifications, every answer right, rss 1391 MB at the mark, 2199 MB at the end (58.0%), peak 2199 MB, memory not judged under --race, 10m1s
- task churn under --race, 10 million tasks: 270 B per finished task after 6 million (Go's race build: 256 B), 1.05 of Go's (at most 1.25); resident set at 6 million 2162 MB (Go's 1726 MB, shown), 7m0s, pass
