kind: stress
module: xml
result: pass
commit: 224654253
source: eabed39f6a91c9b36cec510fac8c0c63dd45c67e8fefa9bed6b80a8a2f872714
date: 2026-11-17
host: linux/arm64 host 4
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 4" xml
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 31m5s

Stress evidence (decision 0616): the program std/evidence/xml/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: xml stress: 4975606656 operations by 64 tasks in 30 min, 0 mismatches, rss 3 MB at the mark, 3 MB at the end (3.0%), peak 3 MB, pass, 30m1s
- llvm: xml stress: 2463404160 operations by 64 tasks in 30 min, 0 mismatches, rss 4 MB at the mark, 4 MB at the end (3.0%), peak 4 MB, pass, 30m0s
- arm64 --check=all: xml stress: 276206208 operations by 64 tasks in 5 min, 0 mismatches, rss 95 MB at the mark, 97 MB at the end (2.0%), peak 97 MB, pass; xo leakcheck: 0 live of 4298459635 allocations, 5m1s
