kind: race
module: png
result: pass
commit: 8a92f83bb
source: cad5f0247dc67b5ad275d7fca67324ef80e6490c7f1581621d50c6691af9b73a
date: 2026-11-15
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" png
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 18m7s

Race evidence (decision 0616): std/png's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus png: ok: 6 passed, 0 failed, 0 skipped, 6s
- corpus pngfuzz: ok: 1000 passed, 0 failed, 0 skipped, 7s
- stress program under --race, 64 tasks: png stress: 4193920 operations by 64 tasks in 10 min, 0 mismatches, rss 162 MB at the mark, 161 MB at the end (-1.0%), peak 176 MB, pass, 10m3s
- task churn under --race, 10 million tasks: 279 B per finished task after 6 million (Go's race build: 255 B), 1.10 of Go's (at most 1.25); resident set at 6 million 2190 MB (Go's 1696 MB, shown), 7m32s, pass
