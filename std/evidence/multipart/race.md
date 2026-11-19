kind: race
module: multipart
result: pass
commit: 10b428ec2
source: 88b5f62618177bf9e2c5faaec049db6c2fb7cc845d834242f9fd2467cedca7b6
date: 2026-11-19
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" multipart
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 14m16s

Race evidence (decision 0616): std/multipart's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/multipart_test.xo): ok: 15 passed, 0 failed, 0 skipped, 5s
- corpus multipart: ok: 7 passed, 0 failed, 0 skipped, 5s
- stress program under --race, 64 tasks: multipart stress: 675544 forms by 64 tasks in 10 min (51416 MB written and read back 3 ways), 0 differences, rss 262 MB at the mark (median of minutes 5 to 7), 261 MB at the end (median of the last 3 minutes; -0.0%), peak 272 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 276 B per finished task after 6 million (Go's race build: 259 B), 1.06 of Go's (at most 1.25); resident set at 6 million 2165 MB (Go's 1693 MB, shown), 3m58s, pass
