kind: fuzz
module: task
result: partial
commit: 8029115ea
source: ad80318929b903120251a1ccac0703c27d3cdff228b2c975974b31d113840193
date: 2026-11-19
host: linux/arm64 host 3
command: go run ./scripts/stdproven fuzz -cases 1000000 -hours 8 -batch 5000 -backends arm64,llvm -host "linux/arm64 host 3" task
measure: TestTaskFuzz 240000 cases per backend, 2 backends (arm64,llvm), 54m15s in 1 shards, 0 mismatches
seeds: 2 to 49
ran: TestTaskFuzz=240000
elapsed: 3254
shards: 1

Long fuzz evidence (decision 0616): every harness that names std/task, against Go's package, with fresh seeds of 5000 cases each.

- TestTaskFuzz seed 2: 5000 cases on arm64,llvm, 1m22s
- TestTaskFuzz seed 3: 5000 cases on arm64,llvm, 1m7s
- TestTaskFuzz seed 4: 5000 cases on arm64,llvm, 1m3s
- TestTaskFuzz seed 5: 5000 cases on arm64,llvm, 1m2s
- TestTaskFuzz seed 6: 5000 cases on arm64,llvm, 1m4s
- TestTaskFuzz seed 7: 5000 cases on arm64,llvm, 1m12s
- TestTaskFuzz seed 8: 5000 cases on arm64,llvm, 1m14s
- TestTaskFuzz seed 9: 5000 cases on arm64,llvm, 1m3s
- TestTaskFuzz seed 10: 5000 cases on arm64,llvm, 1m2s
- TestTaskFuzz seed 11: 5000 cases on arm64,llvm, 1m0s
- TestTaskFuzz seed 12: 5000 cases on arm64,llvm, 1m3s
- TestTaskFuzz seed 13: 5000 cases on arm64,llvm, 1m9s
- TestTaskFuzz seed 14: 5000 cases on arm64,llvm, 1m7s
- TestTaskFuzz seed 15: 5000 cases on arm64,llvm, 1m10s
- TestTaskFuzz seed 16: 5000 cases on arm64,llvm, 1m8s
- TestTaskFuzz seed 17: 5000 cases on arm64,llvm, 1m10s
- TestTaskFuzz seed 18: 5000 cases on arm64,llvm, 1m6s
- TestTaskFuzz seed 19: 5000 cases on arm64,llvm, 1m12s
- TestTaskFuzz seed 20: 5000 cases on arm64,llvm, 1m18s
- TestTaskFuzz seed 21: 5000 cases on arm64,llvm, 1m10s
- TestTaskFuzz seed 22: 5000 cases on arm64,llvm, 1m22s
- TestTaskFuzz seed 23: 5000 cases on arm64,llvm, 1m11s
- TestTaskFuzz seed 24: 5000 cases on arm64,llvm, 1m8s
- TestTaskFuzz seed 25: 5000 cases on arm64,llvm, 1m6s
- TestTaskFuzz seed 26: 5000 cases on arm64,llvm, 1m5s
- TestTaskFuzz seed 27: 5000 cases on arm64,llvm, 1m10s
- TestTaskFuzz seed 28: 5000 cases on arm64,llvm, 1m4s
- TestTaskFuzz seed 29: 5000 cases on arm64,llvm, 1m2s
- TestTaskFuzz seed 30: 5000 cases on arm64,llvm, 1m4s
- TestTaskFuzz seed 31: 5000 cases on arm64,llvm, 1m4s
- TestTaskFuzz seed 32: 5000 cases on arm64,llvm, 1m3s
- TestTaskFuzz seed 33: 5000 cases on arm64,llvm, 1m3s
- TestTaskFuzz seed 34: 5000 cases on arm64,llvm, 1m14s
- TestTaskFuzz seed 35: 5000 cases on arm64,llvm, 1m6s
- TestTaskFuzz seed 36: 5000 cases on arm64,llvm, 1m2s
- TestTaskFuzz seed 37: 5000 cases on arm64,llvm, 1m4s
- TestTaskFuzz seed 38: 5000 cases on arm64,llvm, 1m3s
- TestTaskFuzz seed 39: 5000 cases on arm64,llvm, 1m31s
- TestTaskFuzz seed 40: 5000 cases on arm64,llvm, 1m11s
- TestTaskFuzz seed 41: 5000 cases on arm64,llvm, 1m6s
- TestTaskFuzz seed 42: 5000 cases on arm64,llvm, 1m22s
- TestTaskFuzz seed 43: 5000 cases on arm64,llvm, 1m5s
- TestTaskFuzz seed 44: 5000 cases on arm64,llvm, 1m0s
- TestTaskFuzz seed 45: 5000 cases on arm64,llvm, 1m4s
- TestTaskFuzz seed 46: 5000 cases on arm64,llvm, 1m3s
- TestTaskFuzz seed 47: 5000 cases on arm64,llvm, 1m6s
- TestTaskFuzz seed 48: 5000 cases on arm64,llvm, 1m6s
- TestTaskFuzz seed 49: 5000 cases on arm64,llvm, 1m9s
