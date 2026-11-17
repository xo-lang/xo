kind: race
module: protobuf
result: pass
commit: d10c5bae7
source: 60304805bf37b3c5deea63383cba965beb1f1a614dd538624505c54449ececc3
date: 2026-11-17
host: linux/arm64
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64" protobuf
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 12m27s

Race evidence (decision 0616): std/protobuf's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/native/protobuf_test.xo): ok: 7 passed, 0 failed, 0 skipped, 3s
- corpus protobuf: ok: 3 passed, 0 failed, 0 skipped, 4s
- stress program under --race, 64 tasks: protobuf stress: 2508174968 operations by 64 tasks in 10 min, 0 mismatches, rss 145 MB at the mark, 146 MB at the end (0.0%), peak 146 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 271 B per finished task after 6 million (Go's race build: 255 B), 1.06 of Go's (at most 1.25); resident set at 6 million 2075 MB (Go's 1673 MB, shown), 2m8s, pass
