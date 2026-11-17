kind: race
module: bcrypt
result: pass
commit: 9b7287454
source: 65b06f6a1cb7021bcf1a4f0760a23078339b43d06364f11ac6258bca84bf51ee
date: 2026-11-17
host: linux/arm64 host 4
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 4" bcrypt
measure: 2 runs under --race on llvm, 0 data races, 0 failures, 17m45s

Race evidence (decision 0616): std/bcrypt's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus bcrypt: ok: 5 passed, 0 failed, 0 skipped, 5s
- stress program under --race, 64 tasks: bcrypt stress: 66909 passwords by 64 tasks in 10 min, 0 identities broken, rss 81 MB at the mark, 81 MB at the end (1.0%), peak 81 MB, pass, 10m2s
- task churn under --race, 10 million tasks: 266 B per finished task after 6 million (Go's race build: 252 B), 1.06 of Go's (at most 1.25); resident set at 6 million 2011 MB (Go's 1690 MB, shown), 7m28s, pass
