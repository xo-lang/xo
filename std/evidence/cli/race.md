kind: race
module: cli
result: pass
commit: 10b428ec2
source: 06436b6976603b015b6652c833d7421c05016815a644e9e402eabcc9cad840d0
date: 2026-11-19
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" cli
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 14m14s

Race evidence (decision 0616): std/cli's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus cli: ok: 6 passed, 0 failed, 0 skipped, 4s
- corpus clifuzz: ok: 1 passed, 0 failed, 0 skipped, 3s
- stress program under --race, 64 tasks: cli stress: 314816384 operations by 64 tasks in 10 min, 0 mismatches, rss 84 MB at the mark (median of minutes 5 to 7), 84 MB at the end (median of the last 3 minutes; 0.0%), peak 86 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 273 B per finished task after 6 million (Go's race build: 259 B), 1.05 of Go's (at most 1.25); resident set at 6 million 2195 MB (Go's 1697 MB, shown), 3m59s, pass
