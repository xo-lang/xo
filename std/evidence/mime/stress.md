kind: stress
module: mime
result: pass
commit: 65469d283
source: d0439fe7b222c44fb20a3d7d02b323ed27904633d8c939350b2be475999371d8
date: 2026-11-17
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 3" mime
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 30m24s

Stress evidence (decision 0616): the program std/evidence/mime/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: mime stress: 1377922049 operations by 64 tasks in 30 min, 0 mismatches, rss 4 MB at the mark, 4 MB at the end (2.0%), peak 4 MB, pass, 30m0s
- llvm: mime stress: 1469269231 operations by 64 tasks in 30 min, 0 mismatches, rss 4 MB at the mark, 4 MB at the end (2.0%), peak 4 MB, pass, 30m0s
- arm64 --check=all: mime stress: 32041695 operations by 64 tasks in 5 min, 0 mismatches, rss 133 MB at the mark, 135 MB at the end (2.0%), peak 135 MB, pass; xo leakcheck: 0 live of 2350273950 allocations, 5m0s
