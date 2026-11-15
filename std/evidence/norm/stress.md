kind: stress
module: norm
result: pass
commit: 936746703
source: 2bcf53a513f181d80ee492f3bbda04e23d9091a43a8ee5414aca65937bff9f63
date: 2026-11-15
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 2" norm
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h5m12s

Stress evidence (decision 0616): the program std/evidence/norm/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: norm stress: 3347883520 operations by 64 tasks in 30 min, 0 mismatches, rss 6 MB at the mark, 6 MB at the end (2.0%), peak 6 MB, pass, 30m0s
- llvm: norm stress: 3681522816 operations by 64 tasks in 30 min, 0 mismatches, rss 6 MB at the mark, 6 MB at the end (2.0%), peak 6 MB, pass, 30m0s
- arm64 --check=all: norm stress: 238508288 operations by 64 tasks in 5 min, 0 mismatches, rss 342 MB at the mark, 351 MB at the end (3.0%), peak 351 MB, pass; xo leakcheck: 0 live of 18959549840 allocations, 5m0s
