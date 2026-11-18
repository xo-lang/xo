kind: race
module: task
result: pass
commit: 534c02b0d
source: ad80318929b903120251a1ccac0703c27d3cdff228b2c975974b31d113840193
date: 2026-11-18
host: linux/arm64 host 3
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 3" task
measure: 1 runs under --race on llvm, 0 data races, 0 failures, 15m35s

Race evidence (decision 0616): std/task's tests, corpora and stress program under --race (ThreadSanitizer).

std/task has no module tests and no corpus (the test world's http.server serves requests, not the raw connections its tests would need: #120), so its race evidence is the stress program under --race on real loopback alone (lead, 2026-11-16).

- stress program under --race, 64 tasks: task stress: 24132309 operations by 64 tasks in 10 min, 0 mismatches, rss 6227 MB at the mark (median of minutes 5 to 7), 8102 MB at the end (median of the last 3 minutes; 30.0%), peak 9056 MB, memory not judged under --race, 10m2s
- task churn under --race, 10 million tasks: 274 B per finished task after 6 million (Go's race build: 259 B), 1.06 of Go's (at most 1.25); resident set at 6 million 2079 MB (Go's 1716 MB, shown), 5m19s, pass
