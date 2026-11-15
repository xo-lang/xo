kind: stress
module: png
result: pass
commit: 572cfa622
source: cad5f0247dc67b5ad275d7fca67324ef80e6490c7f1581621d50c6691af9b73a
date: 2026-11-15
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 2" png
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h5m20s

Stress evidence (decision 0616): the program std/evidence/png/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: png stress: 1364065280 operations by 64 tasks in 30 min, 0 mismatches, rss 49 MB at the mark, 50 MB at the end (3.0%), peak 52 MB, pass, 30m0s
- llvm: png stress: 906371520 operations by 64 tasks in 30 min, 0 mismatches, rss 42 MB at the mark, 44 MB at the end (6.0%), peak 47 MB, pass, 30m0s
- arm64 --check=all: png stress: 8504960 operations by 64 tasks in 5 min, 0 mismatches, rss 996 MB at the mark, 997 MB at the end (0.0%), peak 999 MB, pass; xo leakcheck: 0 live of 505472051 allocations, 5m1s
