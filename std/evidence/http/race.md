kind: race
module: http
result: pass
commit: e2b3faafb
source: 4635acf403baa1e0624a1650dbe4bf25ff4fb275a2334fdf2c4fbccbea7ee6ae
date: 2026-11-14
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" http
measure: 4 runs under --race on llvm, 0 data races, 0 failures, 18m38s

Race evidence (decision 0616): std/http's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/native/http_test.xo): ok: 23 passed, 0 failed, 0 skipped, 21s
- corpus http: ok: 15 passed, 0 failed, 0 skipped, 20s
- corpus httpurl: ok: 4 passed, 0 failed, 0 skipped, 4s
- stress program under --race, 64 tasks: http stress: 25620450 requests by 64 clients and 9050 hostile connections by 8 in 10 min, 0 mismatches, rss 250 MB at the mark, 260 MB at the end (4.0%), peak 269 MB, pass, 10m2s
- task churn under --race, 10 million tasks: 271 B per finished task after 6 million (Go's race build: 257 B), 1.05 of Go's (at most 1.25); resident set at 6 million 2145 MB (Go's 1717 MB, shown), 7m22s, pass
STDPROVEN-END
