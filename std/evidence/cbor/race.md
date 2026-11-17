kind: race
module: cbor
result: pass
commit: 32c89673e
source: fe8e1317bf4eacb3684af4568afb12ccc59fc34d3763b3ec93688922f14335f8
date: 2026-11-17
host: linux/arm64 host 4
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 4" cbor
measure: 5 runs under --race on llvm, 0 data races, 0 failures, 18m24s

Race evidence (decision 0616): std/cbor's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/native/cbor_test.xo): ok: 3 passed, 0 failed, 0 skipped, 5s
- module tests (std/cbor_test.xo): ok: 13 passed, 0 failed, 0 skipped, 15s
- corpus cbor: ok: 28 passed, 0 failed, 0 skipped, 16s
- corpus cborfuzz: ok: 1500 passed, 0 failed, 0 skipped, 10s
- stress program under --race, 64 tasks: cbor stress: 2200553304 operations by 64 tasks in 10 min, 0 mismatches, rss 88 MB at the mark, 88 MB at the end (0.0%), peak 88 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 265 B per finished task after 6 million (Go's race build: 252 B), 1.05 of Go's (at most 1.25); resident set at 6 million 2087 MB (Go's 1682 MB, shown), 7m5s, pass
