kind: race
module: binary
result: pass
commit: 39e2b1bbe
source: b1ef0316227b0da7f955a50d989db596dc6b5bd32a4dc947b60dd076f3069803
date: 2026-11-15
host: linux/arm64
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64" binary
measure: 1 runs under --race on llvm, 0 data races, 0 failures, 3m58s

Race evidence (decision 0616): std/binary's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus binary: ok: 2 passed, 0 failed, 0 skipped, 3s
- task churn under --race, 10 million tasks: 274 B per finished task after 6 million (Go's race build: 254 B), 1.08 of Go's (at most 1.25); resident set at 6 million 2151 MB (Go's 1659 MB, shown), 3m51s, pass
