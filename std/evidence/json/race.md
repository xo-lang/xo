kind: race
module: json
result: pass
commit: 1f41e7c8e
source: 57a1aa4d4abc6193c4bc0845d6619bd8f3fee2e3c5b0a9911ee742d760a72820
date: 2026-11-13
host: linux/arm64
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -host "linux/arm64" json
measure: 8 runs under --race on llvm, 0 data races, 0 failures, 10m51s

Race evidence (decision 0616): std/json's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/native/json_test.xo): ok: 16 passed, 0 failed, 0 skipped, 6s
- corpus json: ok: 1 passed, 0 failed, 0 skipped, 5s
- corpus jsondecode: ok: 12 passed, 0 failed, 0 skipped, 4s
- corpus jsonencode: ok: 16 passed, 0 failed, 0 skipped, 4s
- corpus jsonfuzz: ok: 2223 passed, 0 failed, 0 skipped, 11s
- corpus jsonstring: ok: 5 passed, 0 failed, 0 skipped, 3s
- corpus jsontoken: ok: 9 passed, 0 failed, 0 skipped, 9s
- stress program under --race, 64 tasks: json stress: 626300160 operations by 64 tasks in 10 min, 0 mismatches, rss 121 MB at the mark, 125 MB at the end (3.0%), peak 131 MB, pass, 10m1s
