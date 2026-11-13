kind: race
module: json
result: pass
commit: a6211ca4e
source: d489d07c0378b0289a0bc3bafcda807e108ed06a84a62ad880ad8d5dec7dda29
date: 2026-11-13
host: linux/arm64 host 2
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -host "linux/arm64 host 2" json
measure: 8 runs under --race on llvm, 0 data races, 0 failures, 10m41s

Race evidence (decision 0616): std/json's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/native/json_test.xo): ok: 16 passed, 0 failed, 0 skipped, 5s
- corpus json: ok: 1 passed, 0 failed, 0 skipped, 4s
- corpus jsondecode: ok: 12 passed, 0 failed, 0 skipped, 3s
- corpus jsonencode: ok: 16 passed, 0 failed, 0 skipped, 3s
- corpus jsonfuzz: ok: 2022 passed, 0 failed, 0 skipped, 9s
- corpus jsonstring: ok: 5 passed, 0 failed, 0 skipped, 3s
- corpus jsontoken: ok: 9 passed, 0 failed, 0 skipped, 7s
- stress program under --race, 64 tasks: json stress: 700115200 operations by 64 tasks in 10 min, 0 mismatches, rss 130 MB at the mark, 131 MB at the end (1.0%), peak 132 MB, pass, 10m1s
