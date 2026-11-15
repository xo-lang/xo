kind: race
module: fnv
result: pass
commit: 783663cf2
source: 5f34cd0ac2005b37cf77ebf324b2f8d539b5827efe52c87aebbb39fc17218cba
date: 2026-11-14
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" fnv
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 16m48s

Race evidence (decision 0616): std/fnv's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus fnv: ok: 2 passed, 0 failed, 0 skipped, 3s
- corpus fnvfuzz: ok: 2 passed, 0 failed, 0 skipped, 3s
- stress program under --race, 64 tasks: fnv stress: 168844032 operations by 64 tasks in 10 min, 0 mismatches, rss 94 MB at the mark, 96 MB at the end (2.0%), peak 97 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 271 B per finished task after 6 million (Go's race build: 263 B), 1.03 of Go's (at most 1.25); resident set at 6 million 2163 MB (Go's 1674 MB, shown), 6m23s, pass
