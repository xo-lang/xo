kind: race
module: url
result: pass
commit: 719edcf26
source: 828c07e20050977f91ae4bedd1c8a18ed0bfc2a4e83f6bf20f743cf38fda169f
date: 2026-11-14
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" url
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 17m49s

Race evidence (decision 0616): std/url's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus url: ok: 2 passed, 0 failed, 0 skipped, 6s
- corpus urlfuzz: ok: 4 passed, 0 failed, 0 skipped, 10s
- stress program under --race, 64 tasks: url stress: 711937728 operations by 64 tasks in 10 min, 0 mismatches, rss 93 MB at the mark, 94 MB at the end (1.0%), peak 94 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 277 B per finished task after 6 million (Go's race build: 253 B), 1.09 of Go's (at most 1.25); resident set at 6 million 2232 MB (Go's 1686 MB, shown), 7m12s, pass
