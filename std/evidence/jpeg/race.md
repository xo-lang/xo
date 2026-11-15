kind: race
module: jpeg
result: pass
commit: b16f09732
source: 5f0d82e205789ddcdafa775bcd5797960a6fca94b5a193128bcba9c40c8e83e2
date: 2026-11-15
host: linux/arm64 host 2
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 2" jpeg
measure: 4 runs under --race on llvm, 0 data races, 0 failures, 12m52s

Race evidence (decision 0616): std/jpeg's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus jpeg: ok: 2 passed, 0 failed, 0 skipped, 3s
- corpus jpegenc: ok: 3 passed, 0 failed, 0 skipped, 3s
- corpus jpegfuzz: ok: 350 passed, 0 failed, 0 skipped, 3s
- stress program under --race, 64 tasks: jpeg stress: 1339008 operations by 64 tasks in 10 min, 0 mismatches, rss 175 MB at the mark, 177 MB at the end (1.0%), peak 185 MB, pass, 10m4s
- task churn under --race, 10 million tasks: 267 B per finished task after 6 million (Go's race build: 251 B), 1.06 of Go's (at most 1.25); resident set at 6 million 2089 MB (Go's 1673 MB, shown), 2m27s, pass
