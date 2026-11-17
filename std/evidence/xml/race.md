kind: race
module: xml
result: pass
commit: 9e25a6645
source: eabed39f6a91c9b36cec510fac8c0c63dd45c67e8fefa9bed6b80a8a2f872714
date: 2026-11-17
host: linux/arm64 host 4
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 4" xml
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 23m56s

Race evidence (decision 0616): std/xml's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus xml: ok: 9 passed, 0 failed, 0 skipped, 16s
- corpus xmlfuzz: ok: 2000 passed, 0 failed, 0 skipped, 26s
- stress program under --race, 64 tasks: xml stress: 232456320 operations by 64 tasks in 10 min, 0 mismatches, rss 79 MB at the mark, 74 MB at the end (-6.0%), peak 79 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 273 B per finished task after 6 million (Go's race build: 253 B), 1.08 of Go's (at most 1.25); resident set at 6 million 2017 MB (Go's 1680 MB, shown), 12m49s, pass
