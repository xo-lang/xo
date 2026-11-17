kind: race
module: archive
result: pass
commit: 9b7287454
source: 3f24b6af9b4c9eea91413ada780bcfa4a867ba1d1d386735136164f2a8b83a84
date: 2026-11-17
host: linux/arm64 host 3
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 3" archive
measure: 4 runs under --race on llvm, 0 data races, 0 failures, 17m6s

Race evidence (decision 0616): std/archive's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/native/archive_test.xo): ok: 8 passed, 0 failed, 0 skipped, 4s
- corpus archive: ok: 21 passed, 0 failed, 0 skipped, 5s
- corpus archivefuzz: ok: 2011 passed, 0 failed, 0 skipped, 9s
- stress program under --race, 64 tasks: archive stress: 51781257 operations by 64 tasks in 10 min, 0 mismatches, rss 101 MB at the mark, 103 MB at the end (2.0%), peak 104 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 273 B per finished task after 6 million (Go's race build: 252 B), 1.09 of Go's (at most 1.25); resident set at 6 million 2059 MB (Go's 1671 MB, shown), 6m36s, pass
