kind: race
module: random
result: pass
commit: 9b7287454
source: 472f5da38e7e959093060f9f3163e86a91c3fc3d44b8a80aef62c6d59810407b
date: 2026-11-17
host: linux/arm64 host 4
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 4" random
measure: 2 runs under --race on llvm, 0 data races, 0 failures, 17m55s

Race evidence (decision 0616): std/random's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus random: ok: 5 passed, 0 failed, 0 skipped, 7s
- stress program under --race, 64 tasks: random stress: 295152816 operations by 64 tasks in 10 min, 0 mismatches, rss 80 MB at the mark, 80 MB at the end (-0.0%), peak 84 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 267 B per finished task after 6 million (Go's race build: 258 B), 1.04 of Go's (at most 1.25); resident set at 6 million 2035 MB (Go's 1681 MB, shown), 7m25s, pass
