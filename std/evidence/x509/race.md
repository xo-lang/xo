kind: race
module: x509
result: pass
commit: e36a1deed
source: b5e2af4c9a964fcc73d6ee82e930f56d7ded1223e51bbba26f87d5c9f722aea2
date: 2026-11-13
host: linux/arm64
command: go run ./scripts/stdproven race -backend llvm -stress-minutes 10 -host "linux/arm64" x509
measure: 5 runs under --race on llvm, 0 data races, 0 failures, 11m16s

Race evidence (decision 0616): std/x509's tests, corpora and stress program under --race (ThreadSanitizer).

- module tests (std/native/x509_test.xo): ok: 6 passed, 0 failed, 0 skipped, 17s
- corpus x509: ok: 5 passed, 0 failed, 0 skipped, 14s
- corpus x509create: ok: 7 passed, 0 failed, 0 skipped, 17s
- corpus x509fuzz: ok: 1 passed, 0 failed, 0 skipped, 10s
- stress program under --race, 64 tasks: x509 stress: 21111305 operations by 64 tasks in 10 min, 0 mismatches, rss 131 MB at the mark, 129 MB at the end (-1.0%), peak 131 MB, pass, 10m1s
