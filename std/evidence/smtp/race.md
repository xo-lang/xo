kind: race
module: smtp
result: pass
commit: afaa93e8f
source: e2a32e3c5c154fa47ae5a336cbf024f327071461c8be0f98fb9e185e9447142c
date: 2026-11-17
host: linux/arm64 host 4
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 4" smtp
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 19m53s

Race evidence (decision 0616): std/smtp's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/smtp_test.xo): ok: 15 passed, 0 failed, 0 skipped, 1m5s
- corpus smtp: ok: 1 passed, 0 failed, 0 skipped, 1m8s
- stress program under --race, 64 tasks: smtp stress: 240064 sends by 64 clients in 10 min (5959 MB of bodies taken), 0 wrong results, rss 226 MB at the mark, 255 MB at the end (13.0%), peak 255 MB, memory not judged under --race, 10m0s
- task churn under --race, 10 million tasks: 269 B per finished task after 6 million (Go's race build: 257 B), 1.05 of Go's (at most 1.25); resident set at 6 million 2003 MB (Go's 1693 MB, shown), 6m10s, pass
