kind: race
module: grpc
result: pass
commit: d10c5bae7
source: a87bddc9bca36b9611761059b73065d745addee9648943ed177154b3c9a4d794
date: 2026-11-17
host: linux/arm64
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64" grpc
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 18m46s

Race evidence (decision 0616): std/grpc's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/grpc_test.xo): ok: 10 passed, 0 failed, 0 skipped, 54s
- corpus grpc: ok: 5 passed, 0 failed, 0 skipped, 5s
- stress program under --race, 64 tasks: grpc stress: 119798 rounds by 64 tasks in 10 min, every answer right, rss 718 MB at the mark, 981 MB at the end (37.0%), peak 981 MB, memory not judged under --race, 10m0s
- task churn under --race, 10 million tasks: 277 B per finished task after 6 million (Go's race build: 254 B), 1.09 of Go's (at most 1.25); resident set at 6 million 2138 MB (Go's 1680 MB, shown), 6m26s, pass
