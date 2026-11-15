kind: race
module: csv
result: pass
commit: 191613523
source: 728c1ed0ac1a3f1926661234020f83f631735f01f06c88547d140fb6b4eb5027
date: 2026-11-15
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" csv
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 14m11s

Race evidence (decision 0616): std/csv's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus csv: ok: 11 passed, 0 failed, 0 skipped, 12s
- corpus csvfuzz: ok: 2000 passed, 0 failed, 0 skipped, 11s
- stress program under --race, 64 tasks: csv stress: 1153038696 operations by 64 tasks in 10 min, 0 mismatches, rss 68 MB at the mark, 67 MB at the end (-0.0%), peak 70 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 266 B per finished task after 6 million (Go's race build: 254 B), 1.05 of Go's (at most 1.25); resident set at 6 million 2094 MB (Go's 1638 MB, shown), 3m29s, pass
