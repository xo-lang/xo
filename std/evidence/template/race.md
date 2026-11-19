kind: race
module: template
result: pass
commit: 10b428ec2
source: f84c0c6875cd1a9e395f6233654bda53ef2708d778362fa9d0408ccc54c2893d
date: 2026-11-19
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" template
measure: 4 runs under --race on llvm, 0 data races, 0 failures, 12m32s

Race evidence (decision 0616): std/template's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/native/template_test.xo): ok: 20 passed, 0 failed, 0 skipped, 5s
- module tests (std/template_test.xo): ok: 10 passed, 0 failed, 0 skipped, 4s
- corpus template: ok: 2 passed, 0 failed, 0 skipped, 5s
- stress program under --race, 64 tasks: template stress: 107365600 operations by 64 tasks in 10 min, 0 mismatches, rss 106 MB at the mark (median of minutes 5 to 7), 107 MB at the end (median of the last 3 minutes; 1.0%), peak 107 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 265 B per finished task after 6 million (Go's race build: 255 B), 1.04 of Go's (at most 1.25); resident set at 6 million 2091 MB (Go's 1679 MB, shown), 2m8s, pass
