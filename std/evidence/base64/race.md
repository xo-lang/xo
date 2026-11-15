kind: race
module: base64
result: pass
commit: 6781cd66a
source: efd9b1731ce03a28532b686126bc1f938006e447a93cc3c6b91f8e9cdee37346
date: 2026-11-14
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" base64
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 17m10s

Race evidence (decision 0616): std/base64's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus base64: ok: 3 passed, 0 failed, 0 skipped, 5s
- corpus base64fuzz: ok: 2 passed, 0 failed, 0 skipped, 4s
- stress program under --race, 64 tasks: base64 stress: 1035294858 operations by 64 tasks in 10 min, 0 mismatches, rss 93 MB at the mark, 94 MB at the end (0.0%), peak 99 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 283 B per finished task after 6 million (Go's race build: 252 B), 1.12 of Go's (at most 1.25); resident set at 6 million 2202 MB (Go's 1687 MB, shown), 6m44s, pass
