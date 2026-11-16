kind: race
module: heap
result: pass
commit: 52efc3d15
source: e168e8eed85b199d9a02ee6d0c6f1982fb69cc99d3569c5740a630a5882a0cf0
date: 2026-11-16
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" heap
measure: 4 runs under --race on llvm, 0 data races, 0 failures, 18m7s

Race evidence (decision 0616): std/heap's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus heap: ok: 4 passed, 0 failed, 0 skipped, 3m9s
- corpus heapfuzz: ok: 2 passed, 0 failed, 0 skipped, 3s
- corpus heapops: ok: 2 passed, 0 failed, 0 skipped, 8s
- stress program under --race, 64 tasks: heap stress: 1325051910 operations by 64 tasks in 10 min, 0 mismatches, rss 97 MB at the mark, 96 MB at the end (-0.0%), peak 98 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 283 B per finished task after 6 million (Go's race build: 253 B), 1.12 of Go's (at most 1.25); resident set at 6 million 2151 MB (Go's 1686 MB, shown), 4m30s, pass
