kind: race
module: mime
result: pass
commit: d10c5bae7
source: d0439fe7b222c44fb20a3d7d02b323ed27904633d8c939350b2be475999371d8
date: 2026-11-17
host: linux/arm64
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64" mime
measure: 4 runs under --race on llvm, 0 data races, 0 failures, 13m31s

Race evidence (decision 0616): std/mime's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/mime_test.xo): ok: 10 passed, 0 failed, 0 skipped, 11s
- corpus mime: ok: 5 passed, 0 failed, 0 skipped, 13s
- corpus mimefuzz: ok: 2000 passed, 0 failed, 0 skipped, 22s
- stress program under --race, 64 tasks: mime stress: 51123960 operations by 64 tasks in 10 min, 0 mismatches, rss 91 MB at the mark, 92 MB at the end (1.0%), peak 93 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 272 B per finished task after 6 million (Go's race build: 255 B), 1.06 of Go's (at most 1.25); resident set at 6 million 2074 MB (Go's 1680 MB, shown), 2m16s, pass
