kind: race
module: idna
result: pass
commit: b16f09732
source: 43b4baadf7fdc861b507d754c17e8964dbf7cf90ebef483c2d75810202155e30
date: 2026-11-15
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" idna
measure: 2 runs under --race on llvm, 0 data races, 0 failures, 16m44s

Race evidence (decision 0616): std/idna's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus idna: ok: 2 passed, 0 failed, 0 skipped, 10s
- stress program under --race, 64 tasks: idna stress: 131743232 operations by 64 tasks in 10 min, 0 mismatches, rss 88 MB at the mark, 90 MB at the end (2.0%), peak 90 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 275 B per finished task after 6 million (Go's race build: 250 B), 1.10 of Go's (at most 1.25); resident set at 6 million 2093 MB (Go's 1703 MB, shown), 6m15s, pass
