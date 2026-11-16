kind: stress
module: adler32
result: pass
commit: 52efc3d15
source: 3404e8e15f502d2d00e20369198cc5818488606ad73071f781b925f39c579f9e
date: 2026-11-16
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 2" adler32
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h5m10s

Stress evidence (decision 0616): the program std/evidence/adler32/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: adler32 stress: 71878052608 operations by 64 tasks in 30 min, 0 mismatches, rss 6 MB at the mark, 6 MB at the end (2.0%), peak 6 MB, pass, 30m0s
- llvm: adler32 stress: 54991770112 operations by 64 tasks in 30 min, 0 mismatches, rss 6 MB at the mark, 6 MB at the end (2.0%), peak 6 MB, pass, 30m0s
- arm64 --check=all: adler32 stress: 7877799424 operations by 64 tasks in 5 min, 0 mismatches, rss 1178 MB at the mark, 1179 MB at the end (0.0%), peak 1179 MB, pass; xo leakcheck: 0 live of 1969450378 allocations, 5m0s
