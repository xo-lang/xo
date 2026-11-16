kind: stress
module: crc64
result: pass
commit: 52efc3d15
source: 3bd2152457fa866fea42d7ce226f5e0ed10c3d399dff43376e90ef16b1c1f199
date: 2026-11-16
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 2" crc64
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h5m12s

Stress evidence (decision 0616): the program std/evidence/crc64/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: crc64 stress: 40251881088 operations by 64 tasks in 30 min, 0 mismatches, rss 6 MB at the mark, 3 MB at the end (-44.0%), peak 6 MB, pass, 30m0s
- llvm: crc64 stress: 53139429504 operations by 64 tasks in 30 min, 0 mismatches, rss 6 MB at the mark, 6 MB at the end (1.0%), peak 6 MB, pass, 30m0s
- arm64 --check=all: crc64 stress: 5206874688 operations by 64 tasks in 5 min, 0 mismatches, rss 843 MB at the mark, 848 MB at the end (1.0%), peak 848 MB, pass; xo leakcheck: 0 live of 1148044073 allocations, 5m0s
