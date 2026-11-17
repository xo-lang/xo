kind: race
module: hpke
result: pass
commit: 15d43ebe2
source: 3d5a09d9bbb60ba147f9f522e9174279de3d19e27a0437a496c070adc733fd77
date: 2026-11-17
host: linux/arm64
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64" hpke
measure: 2 runs under --race on llvm, 0 data races, 0 failures, 16m40s

Race evidence (decision 0616): std/hpke's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus hpke: ok: 7 passed, 0 failed, 0 skipped, 16s
- stress program under --race, 64 tasks: hpke stress: 3480824 keys by 64 tasks in 10 min, 0 identities broken, rss 108 MB at the mark, 106 MB at the end (-1.0%), peak 110 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 275 B per finished task after 6 million (Go's race build: 254 B), 1.08 of Go's (at most 1.25); resident set at 6 million 2156 MB (Go's 1684 MB, shown), 5m57s, pass
