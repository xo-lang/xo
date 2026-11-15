kind: race
module: png
result: pass
commit: 6781cd66a
source: 55deeec6c4c71321c3b5898464da5887ca6835c68a46c1a2e9f59a572b5d108d
date: 2026-11-14
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" png
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 16m49s

Race evidence (decision 0616): std/png's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus png: ok: 6 passed, 0 failed, 0 skipped, 9s
- corpus pngfuzz: ok: 1000 passed, 0 failed, 0 skipped, 9s
- stress program under --race, 64 tasks: png stress: 3561600 operations by 64 tasks in 10 min, 0 mismatches, rss 167 MB at the mark, 164 MB at the end (-1.0%), peak 180 MB, pass, 10m3s
- task churn under --race, 10 million tasks: 290 B per finished task after 6 million (Go's race build: 253 B), 1.15 of Go's (at most 1.25); resident set at 6 million 2198 MB (Go's 1689 MB, shown), 5m54s, pass
