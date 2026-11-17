kind: race
module: otp
result: pass
commit: 9b7287454
source: 6f898b3267909414c8be4f5128b5ac5e6c4b3f8ab9e39677322615695e9bb6b4
date: 2026-11-17
host: linux/arm64 host 4
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 4" otp
measure: 2 runs under --race on llvm, 0 data races, 0 failures, 19m24s

Race evidence (decision 0616): std/otp's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus otp: ok: 12 passed, 0 failed, 0 skipped, 37s
- stress program under --race, 64 tasks: otp stress: 6947837 keys by 64 tasks in 10 min, 0 identities broken, rss 105 MB at the mark, 107 MB at the end (2.0%), peak 107 MB, pass, 10m4s
- task churn under --race, 10 million tasks: 217 B per finished task after 6 million (Go's race build: 249 B), 0.87 of Go's (at most 1.25); resident set at 6 million 2024 MB (Go's 1686 MB, shown), 8m3s, pass
