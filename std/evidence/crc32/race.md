kind: race
module: crc32
result: pass
commit: 97132a622
source: 15c9a19a15daad089c8b8979d222710f47020953b8c2ce70497aec1771a3f07b
date: 2026-11-15
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" crc32
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 14m32s

Race evidence (decision 0616): std/crc32's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus crc32: ok: 3 passed, 0 failed, 0 skipped, 4s
- corpus crc32fuzz: ok: 2 passed, 0 failed, 0 skipped, 4s
- stress program under --race, 64 tasks: crc32 stress: 189639936 operations by 64 tasks in 10 min, 0 mismatches, rss 85 MB at the mark, 91 MB at the end (7.0%), peak 91 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 270 B per finished task after 6 million (Go's race build: 253 B), 1.07 of Go's (at most 1.25); resident set at 6 million 2119 MB (Go's 1663 MB, shown), 3m57s, pass
