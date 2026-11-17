kind: race
module: cookiejar
result: pass
commit: 9b7287454
source: eb498f12da0158490c12b32d5bba3690f8b8a8b1b3455519efbc0961e11d635e
date: 2026-11-17
host: linux/arm64 host 4
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 4" cookiejar
measure: 2 runs under --race on llvm, 0 data races, 0 failures, 18m56s

Race evidence (decision 0616): std/cookiejar's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus cookiejar: ok: 8 passed, 0 failed, 0 skipped, 8s
- stress program under --race, 64 tasks: cookiejar stress: 9645991 rounds by 64 tasks in 10 min, 0 identities broken, rss 73 MB at the mark, 77 MB at the end (5.0%), peak 77 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 276 B per finished task after 6 million (Go's race build: 251 B), 1.10 of Go's (at most 1.25); resident set at 6 million 2040 MB (Go's 1683 MB, shown), 8m26s, pass
