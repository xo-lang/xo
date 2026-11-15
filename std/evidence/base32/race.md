kind: race
module: base32
result: pass
commit: 131b13592
source: bfc2c5affdd311f6576402e0b3cf49e8ac20037dce3796aff1a97f321c9b65a6
date: 2026-11-15
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" base32
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 17m45s

Race evidence (decision 0616): std/base32's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus base32: ok: 3 passed, 0 failed, 0 skipped, 13s
- corpus base32fuzz: ok: 2 passed, 0 failed, 0 skipped, 10s
- stress program under --race, 64 tasks: base32 stress: 450907584 operations by 64 tasks in 10 min, 0 mismatches, rss 78 MB at the mark, 79 MB at the end (2.0%), peak 85 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 277 B per finished task after 6 million (Go's race build: 251 B), 1.10 of Go's (at most 1.25); resident set at 6 million 2223 MB (Go's 1690 MB, shown), 6m51s, pass
