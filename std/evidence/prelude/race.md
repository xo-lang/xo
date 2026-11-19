kind: race
module: prelude
result: pass
commit: 43088f29d
source: b3e48a195b869e6dfea81b1acfc0a82a2111d0138b773538b577f9064743c444
date: 2026-11-19
host: linux/arm64 host 3
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 3" prelude
measure: 2 runs under --race on llvm, 0 data races, 0 failures, 7m23s

Race evidence (decision 0616): std/prelude's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/native/prelude_test.xo): ok: 65 passed, 0 failed, 0 skipped, 8s
- corpus prelude: ok: 29 passed, 0 failed, 0 skipped, 6s
- task churn under --race, 10 million tasks: 267 B per finished task after 6 million (Go's race build: 252 B), 1.06 of Go's (at most 1.25); resident set at 6 million 2062 MB (Go's 1660 MB, shown), 6m59s, pass
