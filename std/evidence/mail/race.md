kind: race
module: mail
result: pass
commit: 9b7287454
source: bfd3f03dd684f371c1cb23aa0374b1e2c21ffc8668479e54e91a56587e8f303f
date: 2026-11-17
host: linux/arm64 host 4
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 4" mail
measure: 4 runs under --race on llvm, 0 data races, 0 failures, 18m57s

Race evidence (decision 0616): std/mail's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/mail_test.xo): ok: 4 passed, 0 failed, 0 skipped, 6s
- corpus mail: ok: 4 passed, 0 failed, 0 skipped, 9s
- corpus mailfuzz: ok: 2000 passed, 0 failed, 0 skipped, 11s
- stress program under --race, 64 tasks: mail stress: 16937285 messages by 64 tasks in 10 min, 0 identities broken, rss 90 MB at the mark, 91 MB at the end (1.0%), peak 91 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 275 B per finished task after 6 million (Go's race build: 251 B), 1.10 of Go's (at most 1.25); resident set at 6 million 2079 MB (Go's 1676 MB, shown), 8m16s, pass
