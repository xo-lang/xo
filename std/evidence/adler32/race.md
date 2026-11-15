kind: race
module: adler32
result: pass
commit: 39e2b1bbe
source: 3404e8e15f502d2d00e20369198cc5818488606ad73071f781b925f39c579f9e
date: 2026-11-15
host: linux/arm64
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64" adler32
measure: 1 runs under --race on llvm, 0 data races, 0 failures, 4m12s

Race evidence (decision 0616): std/adler32's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus adler32: ok: 4 passed, 0 failed, 0 skipped, 4s
- task churn under --race, 10 million tasks: 278 B per finished task after 6 million (Go's race build: 247 B), 1.12 of Go's (at most 1.25); resident set at 6 million 2148 MB (Go's 1676 MB, shown), 4m0s, pass
