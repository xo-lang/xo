kind: race
module: crc64
result: pass
commit: 52efc3d15
source: 3bd2152457fa866fea42d7ce226f5e0ed10c3d399dff43376e90ef16b1c1f199
date: 2026-11-16
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" crc64
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 16m52s

Race evidence (decision 0616): std/crc64's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus crc64: ok: 5 passed, 0 failed, 0 skipped, 7s
- corpus crc64fuzz: ok: 2 passed, 0 failed, 0 skipped, 4s
- stress program under --race, 64 tasks: crc64 stress: 220978944 operations by 64 tasks in 10 min, 0 mismatches, rss 85 MB at the mark, 90 MB at the end (6.0%), peak 90 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 279 B per finished task after 6 million (Go's race build: 248 B), 1.12 of Go's (at most 1.25); resident set at 6 million 2149 MB (Go's 1691 MB, shown), 6m15s, pass
