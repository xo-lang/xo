kind: race
module: heap
result: pass
commit: 39e2b1bbe
source: 6634b34bc740fc63ab4dc0d88fb178420402f996360a614b22bf12fef0d3bc48
date: 2026-11-15
host: linux/arm64
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64" heap
measure: 2 runs under --race on llvm, 0 data races, 0 failures, 4m52s

Race evidence (decision 0616): std/heap's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus heap: ok: 4 passed, 0 failed, 0 skipped, 1m53s
- corpus heapops: ok: 2 passed, 0 failed, 0 skipped, 7s
- task churn under --race, 10 million tasks: 267 B per finished task after 6 million (Go's race build: 253 B), 1.06 of Go's (at most 1.25); resident set at 6 million 2127 MB (Go's 1653 MB, shown), 2m48s, pass
