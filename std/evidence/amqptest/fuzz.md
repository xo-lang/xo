kind: fuzz
module: amqptest
result: partial
commit: b61e03baf
source: c6784f6482fd491b08976d9483818dab2e043a242fdc402bcb09fe7e7205a959
date: 2026-11-18
host: linux/arm64 host 3
command: go run ./scripts/stdproven fuzz -cases 1000000 -hours 8 -batch 5000 -backends arm64,llvm -host "linux/arm64 host 3" amqptest
measure: TestAmqptestFuzz 385000 cases per backend, 2 backends (arm64,llvm), 3h16m40s in 1 shards, 0 mismatches
seeds: 2 to 78
ran: TestAmqptestFuzz=385000
elapsed: 11799
shards: 1

Long fuzz evidence (decision 0616): every harness that names std/amqptest, against Go's package, with fresh seeds of 5000 cases each.

- TestAmqptestFuzz seed 2: 5000 cases on arm64,llvm, 2m38s
- TestAmqptestFuzz seed 3: 5000 cases on arm64,llvm, 2m11s
- TestAmqptestFuzz seed 4: 5000 cases on arm64,llvm, 2m30s
- TestAmqptestFuzz seed 5: 5000 cases on arm64,llvm, 2m24s
- TestAmqptestFuzz seed 6: 5000 cases on arm64,llvm, 2m44s
- TestAmqptestFuzz seed 7: 5000 cases on arm64,llvm, 2m23s
- TestAmqptestFuzz seed 8: 5000 cases on arm64,llvm, 3m37s
- TestAmqptestFuzz seed 9: 5000 cases on arm64,llvm, 2m52s
- TestAmqptestFuzz seed 10: 5000 cases on arm64,llvm, 2m35s
- TestAmqptestFuzz seed 11: 5000 cases on arm64,llvm, 2m11s
- TestAmqptestFuzz seed 12: 5000 cases on arm64,llvm, 2m24s
- TestAmqptestFuzz seed 13: 5000 cases on arm64,llvm, 2m27s
- TestAmqptestFuzz seed 14: 5000 cases on arm64,llvm, 2m31s
- TestAmqptestFuzz seed 15: 5000 cases on arm64,llvm, 2m20s
- TestAmqptestFuzz seed 16: 5000 cases on arm64,llvm, 2m13s
- TestAmqptestFuzz seed 17: 5000 cases on arm64,llvm, 2m37s
- TestAmqptestFuzz seed 18: 5000 cases on arm64,llvm, 2m55s
- TestAmqptestFuzz seed 19: 5000 cases on arm64,llvm, 3m5s
- TestAmqptestFuzz seed 20: 5000 cases on arm64,llvm, 3m25s
- TestAmqptestFuzz seed 21: 5000 cases on arm64,llvm, 3m10s
- TestAmqptestFuzz seed 22: 5000 cases on arm64,llvm, 4m0s
- TestAmqptestFuzz seed 23: 5000 cases on arm64,llvm, 2m56s
- TestAmqptestFuzz seed 24: 5000 cases on arm64,llvm, 3m22s
- TestAmqptestFuzz seed 25: 5000 cases on arm64,llvm, 2m46s
- TestAmqptestFuzz seed 26: 5000 cases on arm64,llvm, 2m34s
- TestAmqptestFuzz seed 27: 5000 cases on arm64,llvm, 2m58s
- TestAmqptestFuzz seed 28: 5000 cases on arm64,llvm, 2m53s
- TestAmqptestFuzz seed 29: 5000 cases on arm64,llvm, 3m0s
- TestAmqptestFuzz seed 30: 5000 cases on arm64,llvm, 2m55s
- TestAmqptestFuzz seed 31: 5000 cases on arm64,llvm, 2m22s
- TestAmqptestFuzz seed 32: 5000 cases on arm64,llvm, 2m18s
- TestAmqptestFuzz seed 33: 5000 cases on arm64,llvm, 2m18s
- TestAmqptestFuzz seed 34: 5000 cases on arm64,llvm, 2m11s
- TestAmqptestFuzz seed 35: 5000 cases on arm64,llvm, 2m24s
- TestAmqptestFuzz seed 36: 5000 cases on arm64,llvm, 2m47s
- TestAmqptestFuzz seed 37: 5000 cases on arm64,llvm, 2m49s
- TestAmqptestFuzz seed 38: 5000 cases on arm64,llvm, 2m41s
- TestAmqptestFuzz seed 39: 5000 cases on arm64,llvm, 2m36s
- TestAmqptestFuzz seed 40: 5000 cases on arm64,llvm, 2m37s
- TestAmqptestFuzz seed 41: 5000 cases on arm64,llvm, 2m37s
- TestAmqptestFuzz seed 42: 5000 cases on arm64,llvm, 2m35s
- TestAmqptestFuzz seed 43: 5000 cases on arm64,llvm, 2m9s
- TestAmqptestFuzz seed 44: 5000 cases on arm64,llvm, 2m20s
- TestAmqptestFuzz seed 45: 5000 cases on arm64,llvm, 2m44s
- TestAmqptestFuzz seed 46: 5000 cases on arm64,llvm, 2m42s
- TestAmqptestFuzz seed 47: 5000 cases on arm64,llvm, 2m41s
- TestAmqptestFuzz seed 48: 5000 cases on arm64,llvm, 2m40s
- TestAmqptestFuzz seed 49: 5000 cases on arm64,llvm, 2m37s
- TestAmqptestFuzz seed 50: 5000 cases on arm64,llvm, 2m39s
- TestAmqptestFuzz seed 51: 5000 cases on arm64,llvm, 2m39s
- TestAmqptestFuzz seed 52: 5000 cases on arm64,llvm, 2m39s
- TestAmqptestFuzz seed 53: 5000 cases on arm64,llvm, 2m38s
- TestAmqptestFuzz seed 54: 5000 cases on arm64,llvm, 2m35s
- TestAmqptestFuzz seed 55: 5000 cases on arm64,llvm, 2m34s
- TestAmqptestFuzz seed 56: 5000 cases on arm64,llvm, 2m38s
- TestAmqptestFuzz seed 57: 5000 cases on arm64,llvm, 2m39s
- TestAmqptestFuzz seed 58: 5000 cases on arm64,llvm, 2m46s
- TestAmqptestFuzz seed 59: 5000 cases on arm64,llvm, 2m38s
- TestAmqptestFuzz seed 60: 5000 cases on arm64,llvm, 2m33s
- TestAmqptestFuzz seed 61: 5000 cases on arm64,llvm, 2m33s
- TestAmqptestFuzz seed 62: 5000 cases on arm64,llvm, 2m42s
- TestAmqptestFuzz seed 63: 5000 cases on arm64,llvm, 2m35s
- TestAmqptestFuzz seed 64: 5000 cases on arm64,llvm, 2m34s
- TestAmqptestFuzz seed 65: 5000 cases on arm64,llvm, 2m38s
- TestAmqptestFuzz seed 66: 5000 cases on arm64,llvm, 2m37s
- TestAmqptestFuzz seed 67: 5000 cases on arm64,llvm, 1m47s
- TestAmqptestFuzz seed 68: 5000 cases on arm64,llvm, 1m47s
- TestAmqptestFuzz seed 69: 5000 cases on arm64,llvm, 1m39s
- TestAmqptestFuzz seed 70: 5000 cases on arm64,llvm, 1m41s
- TestAmqptestFuzz seed 71: 5000 cases on arm64,llvm, 1m38s
- TestAmqptestFuzz seed 72: 5000 cases on arm64,llvm, 1m38s
- TestAmqptestFuzz seed 73: 5000 cases on arm64,llvm, 1m39s
- TestAmqptestFuzz seed 74: 5000 cases on arm64,llvm, 1m45s
- TestAmqptestFuzz seed 75: 5000 cases on arm64,llvm, 1m56s
- TestAmqptestFuzz seed 76: 5000 cases on arm64,llvm, 2m41s
- TestAmqptestFuzz seed 77: 5000 cases on arm64,llvm, 2m38s
- TestAmqptestFuzz seed 78: 5000 cases on arm64,llvm, 2m35s
