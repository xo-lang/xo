kind: race
module: go
result: pass
commit: 9d705c665
source: 20ba3b9d9243d8ac8566f8ec0af9a8291d6eacd35b73ddc158a88cc18b5e7b66
date: 2026-11-18
host: linux/arm64 host 3
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 3" go
measure: 1 runs under --race on llvm, 0 data races, 0 failures, 7m6s

Race evidence (decision 0616): std/go's tests, corpora and stress program under --race (ThreadSanitizer).

- corpus gomath: ok: 1 passed, 0 failed, 0 skipped, 7s
- task churn under --race, 10 million tasks: 271 B per finished task after 6 million (Go's race build: 251 B), 1.08 of Go's (at most 1.25); resident set at 6 million 2024 MB (Go's 1685 MB, shown), 6m51s, pass
