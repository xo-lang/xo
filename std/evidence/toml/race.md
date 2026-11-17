kind: race
module: toml
result: pass
commit: 9b7287454
source: fffc703b90a3cb47f4761c390aa6ed56084d18900db89b2c6fe1c376f365b997
date: 2026-11-17
host: linux/arm64 host 3
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 3" toml
measure: 5 runs under --race on llvm, 0 data races, 0 failures, 17m30s

Race evidence (decision 0616): std/toml's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/toml_test.xo): ok: 15 passed, 0 failed, 0 skipped, 4s
- corpus toml: ok: 2 passed, 0 failed, 0 skipped, 4s
- corpus tomlfuzz: ok: 2000 passed, 0 failed, 0 skipped, 7s
- corpus tomltest: ok: 543 passed, 0 failed, 0 skipped, 4s
- stress program under --race, 64 tasks: toml stress: 109948000 operations by 64 tasks in 10 min, 0 mismatches, rss 84 MB at the mark, 84 MB at the end (-1.0%), peak 88 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 279 B per finished task after 6 million (Go's race build: 254 B), 1.10 of Go's (at most 1.25); resident set at 6 million 2079 MB (Go's 1659 MB, shown), 6m52s, pass
