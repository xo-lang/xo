kind: stress
module: protodesc
result: pass
commit: 224654253
source: 6b5f20bb8e303b54506e6b6b4d456d20f2ad6a1d35b1992925fdbb4abe0fdbd3
date: 2026-11-17
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 3" protodesc
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 30m31s

Stress evidence (decision 0616): the program std/evidence/protodesc/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: protodesc stress: 326613660 operations by 64 tasks in 30 min, 0 mismatches, rss 13 MB at the mark, 13 MB at the end (4.0%), peak 13 MB, pass, 30m0s
- llvm: protodesc stress: 210128940 operations by 64 tasks in 30 min, 0 mismatches, rss 12 MB at the mark, 12 MB at the end (5.0%), peak 12 MB, pass, 30m0s
- arm64 --check=all: protodesc stress: 20152740 operations by 64 tasks in 5 min, 0 mismatches, rss 521 MB at the mark, 523 MB at the end (0.0%), peak 523 MB, pass; xo leakcheck: 0 live of 2545291583 allocations, 5m1s
