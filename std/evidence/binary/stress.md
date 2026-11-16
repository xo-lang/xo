kind: stress
module: binary
result: pass
commit: 52efc3d15
source: b1ef0316227b0da7f955a50d989db596dc6b5bd32a4dc947b60dd076f3069803
date: 2026-11-16
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 2" binary
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h5m11s

Stress evidence (decision 0616): the program std/evidence/binary/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: binary stress: 670976363520 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark, 5 MB at the end (1.0%), peak 5 MB, pass, 30m0s
- llvm: binary stress: 456659512320 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark, 5 MB at the end (1.0%), peak 5 MB, pass, 30m0s
- arm64 --check=all: binary stress: 21615471360 operations by 64 tasks in 5 min, 0 mismatches, rss 156 MB at the mark, 156 MB at the end (0.0%), peak 156 MB, pass; xo leakcheck: 0 live of 37995946274 allocations, 5m0s
