kind: race
module: yaml
result: pass
commit: 10b428ec2
source: d910f0f56f6d6ce4bdc0b2cbb808c8b712a5f2f97cbab1826d905e1d2883426d
date: 2026-11-19
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" yaml
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 12m27s

Race evidence (decision 0616): std/yaml's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus yaml: ok: 16 passed, 0 failed, 0 skipped, 5s
- corpus yamlfuzz: ok: 2 passed, 0 failed, 0 skipped, 5s
- stress program under --race, 64 tasks: yaml stress: 77020928 operations by 64 tasks in 10 min, 0 mismatches, rss 107 MB at the mark (median of minutes 5 to 7), 107 MB at the end (median of the last 3 minutes; 0.0%), peak 108 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 267 B per finished task after 6 million (Go's race build: 254 B), 1.05 of Go's (at most 1.25); resident set at 6 million 2106 MB (Go's 1672 MB, shown), 2m8s, pass
