kind: race
module: compress
result: pass
commit: b91e3c451
source: c6e28c6b292bd1f66102119f96c9c8a5f26d9474c9ea128482417979e5dba89d
date: 2026-11-14
host: linux/arm64 host 1
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 1" compress
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 13m21s

Race evidence (decision 0616): std/compress's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus compress: ok: 4 passed, 0 failed, 0 skipped, 7s
- corpus compressfuzz: ok: 2000 passed, 0 failed, 0 skipped, 7s
- stress program under --race, 64 tasks: compress stress: 1384200 operations by 64 tasks in 10 min, 0 mismatches, rss 252 MB at the mark, 255 MB at the end (1.0%), peak 290 MB, pass, 10m27s
- task churn under --race, 10 million tasks: 268 B per finished task after 6 million (Go's race build: 253 B), 1.06 of Go's (at most 1.25); resident set at 6 million 1994 MB (Go's 1651 MB, shown), 2m30s, pass
