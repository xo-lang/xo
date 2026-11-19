kind: race
module: xmlcodec
result: pass
commit: 10b428ec2
source: 600ba0019840b9c63a4230402a8dbd539a5bb1a640e15bdfd4ae2c1de309e44e
date: 2026-11-19
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" xmlcodec
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 12m24s

Race evidence (decision 0616): std/xmlcodec's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/xmlcodec_test.xo): ok: 8 passed, 0 failed, 0 skipped, 3s
- corpus xmlcodec: ok: 7 passed, 0 failed, 0 skipped, 5s
- stress program under --race, 64 tasks: xmlcodec stress: 159684000 operations by 64 tasks in 10 min, 0 mismatches, rss 103 MB at the mark (median of minutes 5 to 7), 103 MB at the end (median of the last 3 minutes; 0.0%), peak 103 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 267 B per finished task after 6 million (Go's race build: 252 B), 1.06 of Go's (at most 1.25); resident set at 6 million 2082 MB (Go's 1688 MB, shown), 2m8s, pass
