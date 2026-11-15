kind: race
module: crc64
result: pass
commit: 39e2b1bbe
source: 3bd2152457fa866fea42d7ce226f5e0ed10c3d399dff43376e90ef16b1c1f199
date: 2026-11-15
host: linux/arm64
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64" crc64
measure: 1 runs under --race on llvm, 0 data races, 0 failures, 2m47s

Race evidence (decision 0616): std/crc64's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus crc64: ok: 5 passed, 0 failed, 0 skipped, 3s
- task churn under --race, 10 million tasks: 267 B per finished task after 6 million (Go's race build: 252 B), 1.06 of Go's (at most 1.25); resident set at 6 million 2136 MB (Go's 1686 MB, shown), 2m38s, pass
