kind: race
module: term
result: pass
commit: d10c5bae7
source: b2f0878794d79e2590af1b88f27549b65878f9ef9b2b2eb83cb76e9478347a46
date: 2026-11-17
host: linux/arm64 host 3
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 3" term
measure: 2 runs under --race on llvm, 0 data races, 0 failures, 20m41s

Race evidence (decision 0616): std/term's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus term: ok: 4 passed, 0 failed, 0 skipped, 5s
- stress program under --race, 64 tasks: term stress: 14871321 operations by 64 tasks in 10 min, 0 mismatches, rss 92 MB at the mark, 92 MB at the end (0.0%), peak 94 MB, pass, 10m2s
- task churn under --race, 10 million tasks: 273 B per finished task after 6 million (Go's race build: 249 B), 1.10 of Go's (at most 1.25); resident set at 6 million 2043 MB (Go's 1693 MB, shown), 10m21s, pass
