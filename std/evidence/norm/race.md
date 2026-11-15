kind: race
module: norm
result: pass
commit: 936746703
source: 2bcf53a513f181d80ee492f3bbda04e23d9091a43a8ee5414aca65937bff9f63
date: 2026-11-15
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" norm
measure: 2 runs under --race on llvm, 0 data races, 0 failures, 18m47s

Race evidence (decision 0616): std/norm's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus norm: ok: 4 passed, 0 failed, 0 skipped, 12s
- stress program under --race, 64 tasks: norm stress: 15443072 operations by 64 tasks in 10 min, 0 mismatches, rss 94 MB at the mark, 96 MB at the end (2.0%), peak 96 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 284 B per finished task after 6 million (Go's race build: 253 B), 1.12 of Go's (at most 1.25); resident set at 6 million 2141 MB (Go's 1681 MB, shown), 8m15s, pass
