kind: race
module: gif
result: pass
commit: 9b7287454
source: 215a7b62f899bead7b3204eeae406fdcb8eeb60adbb8736bce24382341e010c4
date: 2026-11-17
host: linux/arm64 host 3
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 3" gif
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 18m51s

Race evidence (decision 0616): std/gif's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus gif: ok: 4 passed, 0 failed, 0 skipped, 6s
- corpus giffuzz: ok: 1 passed, 0 failed, 0 skipped, 4s
- stress program under --race, 64 tasks: gif stress: 1168640 operations by 64 tasks in 10 min, 0 mismatches, rss 184 MB at the mark, 177 MB at the end (-4.0%), peak 185 MB, pass, 10m3s
- task churn under --race, 10 million tasks: 284 B per finished task after 6 million (Go's race build: 255 B), 1.11 of Go's (at most 1.25); resident set at 6 million 2044 MB (Go's 1691 MB, shown), 8m28s, pass
