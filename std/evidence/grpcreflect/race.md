kind: race
module: grpcreflect
result: pass
commit: 448c1f7e9
source: b0ab971fcb41b79eef55462ae00b6a05ed63e9240f7259cc56ac884f814fcf92
date: 2026-11-17
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" grpcreflect
measure: 2 runs under --race on llvm, 0 data races, 0 failures, 19m7s

Race evidence (decision 0616): std/grpcreflect's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/grpcreflect_test.xo): ok: 5 passed, 0 failed, 0 skipped, 38s
- stress program under --race, 64 tasks: grpcreflect stress: 2457217 rounds by 64 tasks in 10 min, every answer as expected, rss 2268 MB at the mark (median of minutes 5 to 7), 2936 MB at the end (median of the last 3 minutes; 29.0%), peak 3163 MB, memory not judged under --race, 10m1s
- task churn under --race, 10 million tasks: 271 B per finished task after 6 million (Go's race build: 253 B), 1.07 of Go's (at most 1.25); resident set at 6 million 2171 MB (Go's 1682 MB, shown), 6m52s, pass
