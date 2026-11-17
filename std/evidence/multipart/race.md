kind: race
module: multipart
result: pass
commit: d9e68db69
source: 907bb4b11915093de24d728706fe3cfccee080f056471a2c6160ebee3d8ef2b1
date: 2026-11-17
host: linux/arm64 host 3
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 3" multipart
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 18m3s

Race evidence (decision 0616): std/multipart's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/multipart_test.xo): ok: 15 passed, 0 failed, 0 skipped, 8s
- corpus multipart: ok: 7 passed, 0 failed, 0 skipped, 12s
- stress program under --race, 64 tasks: multipart stress: 251760 forms by 64 tasks in 10 min (19172 MB written and read back 3 ways), 0 differences, rss 241 MB at the mark, 246 MB at the end (2.0%), peak 249 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 269 B per finished task after 6 million (Go's race build: 255 B), 1.06 of Go's (at most 1.25); resident set at 6 million 2018 MB (Go's 1681 MB, shown), 7m17s, pass
