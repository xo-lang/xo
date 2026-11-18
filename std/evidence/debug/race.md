kind: race
module: debug
result: pass
commit: 81f591c41
source: 15d4b05647042c6360f665e231b0a8231edcde5f588ff65d617547670dfcd0ea
date: 2026-11-18
host: linux/arm64 host 3
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 3" debug
measure: 1 runs under --race on llvm, 0 data races, 0 failures, 17m6s

Race evidence (decision 0616): std/debug's tests, corpora and stress program under --race (ThreadSanitizer).

std/debug has no module tests and no corpus (the test world's http.server serves requests, not the raw connections its tests would need: #120), so its race evidence is the stress program under --race on real loopback alone (lead, 2026-11-16).

- stress program under --race, 64 tasks: debug stress: 225702879 operations by 64 tasks in 10 min, 0 mismatches, rss 198 MB at the mark (median of minutes 5 to 7), 195 MB at the end (median of the last 3 minutes; -1.0%), peak 236 MB, pass, 10m2s
- task churn under --race, 10 million tasks: 264 B per finished task after 6 million (Go's race build: 254 B), 1.04 of Go's (at most 1.25); resident set at 6 million 2009 MB (Go's 1634 MB, shown), 6m42s, pass
