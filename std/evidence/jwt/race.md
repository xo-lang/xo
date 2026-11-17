kind: race
module: jwt
result: pass
commit: 9b7287454
source: 90eda17015be07d1790318f2f57478e05272cfc27909eb5b1480629371264a12
date: 2026-11-17
host: linux/arm64 host 4
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 4" jwt
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 19m3s

Race evidence (decision 0616): std/jwt's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/jwt_test.xo): ok: 7 passed, 0 failed, 0 skipped, 18s
- corpus jwt: ok: 5 passed, 0 failed, 0 skipped, 18s
- stress program under --race, 64 tasks: jwt stress: 1703408 tokens by 64 tasks in 10 min, 0 identities broken, rss 93 MB at the mark, 92 MB at the end (-1.0%), peak 93 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 270 B per finished task after 6 million (Go's race build: 251 B), 1.07 of Go's (at most 1.25); resident set at 6 million 1999 MB (Go's 1679 MB, shown), 8m3s, pass
