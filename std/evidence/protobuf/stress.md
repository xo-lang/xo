kind: stress
module: protobuf
result: pass
commit: a14be2e62
source: 60304805bf37b3c5deea63383cba965beb1f1a614dd538624505c54449ececc3
date: 2026-11-17
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 3" protobuf
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 30m44s

Stress evidence (decision 0616): the program std/evidence/protobuf/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: protobuf stress: 30113344430 operations by 64 tasks in 30 min, 0 mismatches, rss 54 MB at the mark, 54 MB at the end (0.0%), peak 54 MB, pass, 30m1s
- llvm: protobuf stress: 27794968292 operations by 64 tasks in 30 min, 0 mismatches, rss 46 MB at the mark, 46 MB at the end (0.0%), peak 46 MB, pass, 30m0s
- arm64 --check=all: protobuf stress: 137875101 operations by 64 tasks in 5 min, 0 mismatches, rss not sampled, pass; xo leakcheck: 0 live of 589118755 allocations, 5m1s
