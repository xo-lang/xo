kind: race
module: prototext
result: pass
commit: 52003ef34
source: 6b9dd2969f19f2af7b853a97ab5ac7b189138ebb3ce1c82e18c3bbc2ad602863
date: 2026-11-17
host: linux/arm64 host 4
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 4" prototext
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 18m31s

Race evidence (decision 0616): std/prototext's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/prototext_test.xo): ok: 2 passed, 0 failed, 0 skipped, 5s
- corpus prototext: ok: 3 passed, 0 failed, 0 skipped, 6s
- stress program under --race, 64 tasks: prototext stress: 22645920 operations by 64 tasks in 10 min, 0 mismatches, rss 88 MB at the mark (median of minutes 5 to 7), 88 MB at the end (median of the last 3 minutes; 1.0%), peak 92 MB, pass, 10m2s
- task churn under --race, 10 million tasks: 281 B per finished task after 6 million (Go's race build: 251 B), 1.12 of Go's (at most 1.25); resident set at 6 million 1960 MB (Go's 1682 MB, shown), 7m31s, pass
