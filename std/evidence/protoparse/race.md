kind: race
module: protoparse
result: pass
commit: 9f490a22d
source: 40054218c95d567c567c0b5c642e44120b8469a4737ced296d70ff2941e0f9a3
date: 2026-11-18
host: linux/arm64 host 3
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 3" protoparse
measure: 4 runs under --race on llvm, 0 data races, 0 failures, 20m9s

Race evidence (decision 0616): std/protoparse's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/protoparse_test.xo): ok: 32 passed, 0 failed, 0 skipped, 8s
- corpus protoparse: ok: 4 passed, 0 failed, 0 skipped, 6s
- corpus protoparsefuzz: ok: 2 passed, 0 failed, 0 skipped, 5s
- stress program under --race, 64 tasks: protoparse stress: 14385984 operations by 64 tasks in 10 min, 0 mismatches, rss 126 MB at the mark (median of minutes 5 to 7), 127 MB at the end (median of the last 3 minutes; 0.0%), peak 128 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 275 B per finished task after 6 million (Go's race build: 254 B), 1.08 of Go's (at most 1.25); resident set at 6 million 2053 MB (Go's 1684 MB, shown), 9m35s, pass
