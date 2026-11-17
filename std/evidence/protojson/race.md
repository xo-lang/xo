kind: race
module: protojson
result: pass
commit: d10c5bae7
source: f6b7c2b16bab5db6c7b459f4fd22011a507d18c5cf72de6498af6997a3bc4eb8
date: 2026-11-17
host: linux/arm64
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64" protojson
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 16m24s

Race evidence (decision 0616): std/protojson's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/protojson_test.xo): ok: 2 passed, 0 failed, 0 skipped, 3s
- corpus protojson: ok: 6 passed, 0 failed, 0 skipped, 4s
- stress program under --race, 64 tasks: protojson stress: 69330 operations by 64 tasks in 10 min, 0 mismatches, rss 135 MB at the mark, 136 MB at the end (1.0%), peak 138 MB, pass, 10m3s
- task churn under --race, 10 million tasks: 273 B per finished task after 6 million (Go's race build: 257 B), 1.07 of Go's (at most 1.25); resident set at 6 million 2126 MB (Go's 1652 MB, shown), 6m6s, pass
