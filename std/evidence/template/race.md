kind: race
module: template
result: pass
commit: d9e68db69
source: ac4c3d141156db3ab1bca9881fe29f81ba5d025788bf9578b494ad58b4b16adf
date: 2026-11-17
host: linux/arm64 host 3
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 3" template
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 22m22s

Race evidence (decision 0616): std/template's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/native/template_test.xo): ok: 20 passed, 0 failed, 0 skipped, 12s
- corpus template: ok: 2 passed, 0 failed, 0 skipped, 10s
- stress program under --race, 64 tasks: template stress: 42695200 operations by 64 tasks in 10 min, 0 mismatches, rss 90 MB at the mark, 91 MB at the end (2.0%), peak 91 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 271 B per finished task after 6 million (Go's race build: 247 B), 1.10 of Go's (at most 1.25); resident set at 6 million 2048 MB (Go's 1666 MB, shown), 11m13s, pass
