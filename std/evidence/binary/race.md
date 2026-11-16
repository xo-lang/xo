kind: race
module: binary
result: pass
commit: 52efc3d15
source: b1ef0316227b0da7f955a50d989db596dc6b5bd32a4dc947b60dd076f3069803
date: 2026-11-16
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" binary
measure: 4 runs under --race on llvm, 0 data races, 0 failures, 18m2s

Race evidence (decision 0616): std/binary's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/binary_test.xo): ok: 4 passed, 0 failed, 0 skipped, 4s
- corpus binary: ok: 2 passed, 0 failed, 0 skipped, 6s
- corpus binaryfuzz: ok: 2 passed, 0 failed, 0 skipped, 6s
- stress program under --race, 64 tasks: binary stress: 5892650240 operations by 64 tasks in 10 min, 0 mismatches, rss 76 MB at the mark, 78 MB at the end (3.0%), peak 78 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 274 B per finished task after 6 million (Go's race build: 261 B), 1.05 of Go's (at most 1.25); resident set at 6 million 2154 MB (Go's 1734 MB, shown), 7m23s, pass
