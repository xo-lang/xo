kind: race
module: otlp
result: pass
commit: 9d705c665
source: e3a8fc78fecbfc2aa761830679145ef36e8a3ace559656bed4a8379a23125753
date: 2026-11-18
host: linux/arm64 host 3
race runtime: xo's (lib/xo/race/linux-arm64 from scripts/race-runtime.sh bb1022a99dfcaa22, TracePart 128 KB)
go: go1.26.11 (the task churn's Go twin)
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -churn-rounds 50 -host "linux/arm64 host 3" otlp
measure: 3 runs under --race on llvm, 0 data races, 0 failures, 18m6s

Race evidence (decision 0616): std/otlp's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/native/otlp_test.xo): ok: 22 passed, 0 failed, 0 skipped, 41s
- corpus otlp: ok: 2 passed, 0 failed, 0 skipped, 4s
- stress program under --race, 64 tasks: otlp stress: 6721048 exports by 64 tasks in 10 min (6428829 traces and 672079 metrics requests taken), 0 wrong, rss 195 MB at the mark (median of minutes 5 to 7), 204 MB at the end (median of the last 3 minutes; 4.0%), peak 211 MB, pass, 10m1s
- task churn under --race, 10 million tasks: 276 B per finished task after 6 million (Go's race build: 249 B), 1.11 of Go's (at most 1.25); resident set at 6 million 2078 MB (Go's 1686 MB, shown), 6m44s, pass
