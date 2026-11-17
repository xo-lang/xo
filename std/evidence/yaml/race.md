kind: race
module: yaml
result: pass
commit: d9e68db69
source: 51ad9c216133517a40a8a8eb75636f1f208e452ea95f474273b48352abd8ec63
date: 2026-11-17
host: linux/arm64 host 3
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 3" yaml
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 18m27s

Race evidence (decision 0616): std/yaml's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus yaml: ok: 16 passed, 0 failed, 0 skipped, 11s
- corpus yamlfuzz: ok: 2 passed, 0 failed, 0 skipped, 12s
- stress program under --race, 64 tasks: yaml stress: 35077376 operations by 64 tasks in 10 min, 0 mismatches, rss 98 MB at the mark, 99 MB at the end (2.0%), peak 102 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 267 B per finished task after 6 million (Go's race build: 249 B), 1.07 of Go's (at most 1.25); resident set at 6 million 2016 MB (Go's 1681 MB, shown), 7m41s, pass
