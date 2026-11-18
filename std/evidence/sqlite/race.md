kind: race
module: sqlite
result: pass
commit: b9686bfe8
source: 971880485e9538d26d60947947deb8b4c294ffc3595bd14b4f2340e5acb5bb8f
date: 2026-11-18
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" sqlite
measure: 4 runs under --race on llvm, 0 data races, 0 failures, 16m25s

Race evidence (decision 0616): std/sqlite's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/native/sqlite_test.xo): ok: 10 passed, 0 failed, 0 skipped, 19s
- module tests (std/sqlite_test.xo): ok: 10 passed, 0 failed, 0 skipped, 26s
- corpus sqlite: ok: 1 passed, 0 failed, 0 skipped, 20s
- stress program under --race, 64 tasks: sqlite stress: 1605852 statements by 64 clients over 8 pools in 10 min, 0 mismatches, rss 100 MB at the mark (median of minutes 5 to 7), 101 MB at the end (median of the last 3 minutes; 0.0%), peak 102 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 273 B per finished task after 6 million (Go's race build: 253 B), 1.08 of Go's (at most 1.25); resident set at 6 million 2153 MB (Go's 1656 MB, shown), 4m24s, pass

The vendored C library (std/native/sqlite/sqlite3.c) is compiled without ThreadSanitizer instrumentation (decision 0715): races inside SQLite itself are out of scope; every Xo caller and the bindings are instrumented.
