kind: stress
module: protoparse
result: pass
commit: 745cbde2b
source: 40054218c95d567c567c0b5c642e44120b8469a4737ced296d70ff2941e0f9a3
date: 2026-11-18
host: linux/arm64 host 4
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 4" protoparse
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h5m12s

Stress evidence (decision 0616): the program std/evidence/protoparse/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: protoparse stress: 914665248 operations by 64 tasks in 30 min, 0 mismatches, rss 12 MB at the mark (median of minutes 5 to 7), 11 MB at the end (median of the last 3 minutes; -5.0%), peak 12 MB, pass, 30m0s
- llvm: protoparse stress: 927536928 operations by 64 tasks in 30 min, 0 mismatches, rss 11 MB at the mark (median of minutes 5 to 7), 12 MB at the end (median of the last 3 minutes; 4.0%), peak 12 MB, pass, 30m0s
- arm64 --check=all: protoparse stress: 31132608 operations by 64 tasks in 5 min, 0 mismatches, rss not sampled, pass; xo leakcheck: 0 live of 5777163433 allocations, 5m0s
