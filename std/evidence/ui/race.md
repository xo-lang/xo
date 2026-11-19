kind: race
module: ui
result: pass
commit: 349438929
source: 414ad74647d33f10c82b2fbe386098b64c863eae9ce778b63b33b4a882e9a57e
date: 2026-11-19
host: linux/arm64
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64" ui
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 16m5s

Race evidence (decision 0616): std/ui's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/ui_test.xo): ok: 91 passed, 0 failed, 0 skipped, 33s
- corpus ui: ok: 15 passed, 0 failed, 0 skipped, 8s
- stress program under --race, 64 tasks: ui stress: 2050974 syncs of 3077254 rounds by 64 clients in 10 min (1 reloads), every Msg applied once, every replica converged, rss 251 MB at the mark (median of minutes 5 to 7), 257 MB at the end (median of the last 3 minutes; 2.0%), peak 283 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 277 B per finished task after 6 million (Go's race build: 250 B), 1.11 of Go's (at most 1.25); resident set at 6 million 2119 MB (Go's 1684 MB, shown), 4m22s, pass
