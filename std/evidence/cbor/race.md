kind: race
module: cbor
result: pass
commit: 10b428ec2
source: 462e7b400b749d5020fd3263e6390aeb64314c97daa11c05d66b537a170e56de
date: 2026-11-19
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" cbor
measure: 5 runs under --race on llvm, 0 data races, 0 failures, 14m42s

Race evidence (decision 0616): std/cbor's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/native/cbor_test.xo): ok: 3 passed, 0 failed, 0 skipped, 3s
- module tests (std/cbor_test.xo): ok: 13 passed, 0 failed, 0 skipped, 7s
- corpus cbor: ok: 28 passed, 0 failed, 0 skipped, 11s
- corpus cborfuzz: ok: 1500 passed, 0 failed, 0 skipped, 6s
- stress program under --race, 64 tasks: cbor stress: 3767848384 operations by 64 tasks in 10 min, 0 mismatches, rss 92 MB at the mark (median of minutes 5 to 7), 92 MB at the end (median of the last 3 minutes; 1.0%), peak 92 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 279 B per finished task after 6 million (Go's race build: 251 B), 1.11 of Go's (at most 1.25); resident set at 6 million 2181 MB (Go's 1678 MB, shown), 3m59s, pass
