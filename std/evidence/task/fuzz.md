kind: fuzz
module: task
result: partial
commit: 8137a5f35
source: ad80318929b903120251a1ccac0703c27d3cdff228b2c975974b31d113840193
date: 2026-11-19
host: linux/arm64 host 3
command: go run ./scripts/stdproven fuzz -cases 1000000 -hours 8 -batch 5000 -backends arm64,llvm -host "linux/arm64 host 3" task
measure: TestTaskFuzz 485000 cases per backend, 2 backends (arm64,llvm), 1h49m13s in 2 shards, 0 mismatches
seeds: 2 to 98
ran: TestTaskFuzz=485000
elapsed: 6552
shards: 2

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
- TestTaskFuzz seed 50: 5000 cases on arm64,llvm, 1m11s
- TestTaskFuzz seed 51: 5000 cases on arm64,llvm, 1m12s
- TestTaskFuzz seed 52: 5000 cases on arm64,llvm, 1m5s
- TestTaskFuzz seed 53: 5000 cases on arm64,llvm, 1m3s
- TestTaskFuzz seed 54: 5000 cases on arm64,llvm, 1m4s
- TestTaskFuzz seed 55: 5000 cases on arm64,llvm, 54s
- TestTaskFuzz seed 56: 5000 cases on arm64,llvm, 1m2s
- TestTaskFuzz seed 57: 5000 cases on arm64,llvm, 1m4s
- TestTaskFuzz seed 58: 5000 cases on arm64,llvm, 1m1s
- TestTaskFuzz seed 59: 5000 cases on arm64,llvm, 1m5s
- TestTaskFuzz seed 60: 5000 cases on arm64,llvm, 53s
- TestTaskFuzz seed 61: 5000 cases on arm64,llvm, 55s
- TestTaskFuzz seed 62: 5000 cases on arm64,llvm, 1m2s
- TestTaskFuzz seed 63: 5000 cases on arm64,llvm, 1m4s
- TestTaskFuzz seed 64: 5000 cases on arm64,llvm, 56s
- TestTaskFuzz seed 65: 5000 cases on arm64,llvm, 1m3s
- TestTaskFuzz seed 66: 5000 cases on arm64,llvm, 1m10s
- TestTaskFuzz seed 67: 5000 cases on arm64,llvm, 1m6s
- TestTaskFuzz seed 68: 5000 cases on arm64,llvm, 1m3s
- TestTaskFuzz seed 69: 5000 cases on arm64,llvm, 1m2s
- TestTaskFuzz seed 70: 5000 cases on arm64,llvm, 1m2s
- TestTaskFuzz seed 71: 5000 cases on arm64,llvm, 1m2s
- TestTaskFuzz seed 72: 5000 cases on arm64,llvm, 57s
- TestTaskFuzz seed 73: 5000 cases on arm64,llvm, 1m5s
- TestTaskFuzz seed 74: 5000 cases on arm64,llvm, 1m5s
- TestTaskFuzz seed 75: 5000 cases on arm64,llvm, 1m5s
- TestTaskFuzz seed 76: 5000 cases on arm64,llvm, 1m2s
- TestTaskFuzz seed 77: 5000 cases on arm64,llvm, 1m2s
- TestTaskFuzz seed 78: 5000 cases on arm64,llvm, 1m3s
- TestTaskFuzz seed 79: 5000 cases on arm64,llvm, 1m4s
- TestTaskFuzz seed 80: 5000 cases on arm64,llvm, 1m4s
- TestTaskFuzz seed 81: 5000 cases on arm64,llvm, 1m5s
- TestTaskFuzz seed 82: 5000 cases on arm64,llvm, 1m16s
- TestTaskFuzz seed 83: 5000 cases on arm64,llvm, 1m14s
- TestTaskFuzz seed 84: 5000 cases on arm64,llvm, 1m10s
- TestTaskFuzz seed 85: 5000 cases on arm64,llvm, 1m12s
- TestTaskFuzz seed 86: 5000 cases on arm64,llvm, 1m12s
- TestTaskFuzz seed 87: 5000 cases on arm64,llvm, 1m24s
- TestTaskFuzz seed 88: 5000 cases on arm64,llvm, 1m5s
- TestTaskFuzz seed 89: 5000 cases on arm64,llvm, 1m26s
- TestTaskFuzz seed 90: 5000 cases on arm64,llvm, 1m27s
- TestTaskFuzz seed 91: 5000 cases on arm64,llvm, 1m10s
- TestTaskFuzz seed 92: 5000 cases on arm64,llvm, 1m13s
- TestTaskFuzz seed 93: 5000 cases on arm64,llvm, 1m12s
- TestTaskFuzz seed 94: 5000 cases on arm64,llvm, 1m6s
- TestTaskFuzz seed 95: 5000 cases on arm64,llvm, 1m14s
- TestTaskFuzz seed 96: 5000 cases on arm64,llvm, 1m19s
- TestTaskFuzz seed 97: 5000 cases on arm64,llvm, 1m11s
- TestTaskFuzz seed 98: 5000 cases on arm64,llvm, 1m38s
