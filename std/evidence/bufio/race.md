kind: race
module: bufio
result: pass
commit: 59824eb7c
source: 13cbd58b5b5557a2df32a7e72254609d0933160feed319f48e32f60d6c9bf9a4
date: 2026-11-17
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" bufio
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 18m45s

Race evidence (decision 0616): std/bufio's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus bufio: ok: 6 passed, 0 failed, 0 skipped, 16s
- corpus bufiofuzz: ok: 1 passed, 0 failed, 0 skipped, 9s
- stress program under --race, 64 tasks: bufio stress: 204441448 operations by 64 tasks in 10 min, 0 mismatches, rss 102 MB at the mark, 99 MB at the end (-3.0%), peak 104 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 276 B per finished task after 6 million (Go's race build: 249 B), 1.11 of Go's (at most 1.25); resident set at 6 million 2143 MB (Go's 1686 MB, shown), 7m23s, pass
