kind: race
module: adler32
result: pass
commit: 52efc3d15
source: 3404e8e15f502d2d00e20369198cc5818488606ad73071f781b925f39c579f9e
date: 2026-11-16
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" adler32
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 14m34s

Race evidence (decision 0616): std/adler32's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus adler32: ok: 4 passed, 0 failed, 0 skipped, 4s
- corpus adler32fuzz: ok: 2 passed, 0 failed, 0 skipped, 4s
- stress program under --race, 64 tasks: adler32 stress: 1137044224 operations by 64 tasks in 10 min, 0 mismatches, rss 85 MB at the mark, 83 MB at the end (-2.0%), peak 87 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 276 B per finished task after 6 million (Go's race build: 247 B), 1.12 of Go's (at most 1.25); resident set at 6 million 2121 MB (Go's 1656 MB, shown), 3m56s, pass
