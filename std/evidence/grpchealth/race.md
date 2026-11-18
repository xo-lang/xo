kind: race
module: grpchealth
result: pass
commit: 9d705c665
source: facc0759234b56eb97e36a88058094997f18078d884718162977b9b629ea6e5c
date: 2026-11-18
host: linux/arm64 host 3
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 3" grpchealth
measure: 1 runs under --race on llvm, 0 data races, 0 failures, 8m3s

Race evidence (decision 0616): std/grpchealth's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/grpchealth_test.xo): ok: 5 passed, 0 failed, 0 skipped, 1m3s
- task churn under --race, 10 million tasks: 271 B per finished task after 6 million (Go's race build: 258 B), 1.05 of Go's (at most 1.25); resident set at 6 million 2023 MB (Go's 1678 MB, shown), 6m59s, pass
