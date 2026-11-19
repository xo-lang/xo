kind: fuzz
module: debug
result: partial
commit: 8d8741fe5
source: 9afe6d1ee16f6e52ab6aaf9138c510ceba523439bc55eddef4fdf61872e7df4e
date: 2026-11-19
host: linux/arm64 host 3
command: go run ./scripts/stdproven fuzz -cases 1000000 -hours 8 -batch 5000 -backends arm64,llvm -host "linux/arm64 host 3" debug
measure: TestDebugFuzz 265000 cases per backend, 2 backends (arm64,llvm), 54m38s in 1 shards, 0 mismatches
seeds: 2 to 54
ran: TestDebugFuzz=265000
elapsed: 3278
shards: 1

Long fuzz evidence (decision 0616): every harness that names std/debug, against Go's package, with fresh seeds of 5000 cases each.

- TestDebugFuzz seed 2: 5000 cases on arm64,llvm, 1m16s
- TestDebugFuzz seed 3: 5000 cases on arm64,llvm, 1m5s
- TestDebugFuzz seed 4: 5000 cases on arm64,llvm, 58s
- TestDebugFuzz seed 5: 5000 cases on arm64,llvm, 59s
- TestDebugFuzz seed 6: 5000 cases on arm64,llvm, 1m1s
- TestDebugFuzz seed 7: 5000 cases on arm64,llvm, 57s
- TestDebugFuzz seed 8: 5000 cases on arm64,llvm, 1m0s
- TestDebugFuzz seed 9: 5000 cases on arm64,llvm, 1m1s
- TestDebugFuzz seed 10: 5000 cases on arm64,llvm, 59s
- TestDebugFuzz seed 11: 5000 cases on arm64,llvm, 57s
- TestDebugFuzz seed 12: 5000 cases on arm64,llvm, 58s
- TestDebugFuzz seed 13: 5000 cases on arm64,llvm, 59s
- TestDebugFuzz seed 14: 5000 cases on arm64,llvm, 57s
- TestDebugFuzz seed 15: 5000 cases on arm64,llvm, 58s
- TestDebugFuzz seed 16: 5000 cases on arm64,llvm, 56s
- TestDebugFuzz seed 17: 5000 cases on arm64,llvm, 56s
- TestDebugFuzz seed 18: 5000 cases on arm64,llvm, 57s
- TestDebugFuzz seed 19: 5000 cases on arm64,llvm, 57s
- TestDebugFuzz seed 20: 5000 cases on arm64,llvm, 57s
- TestDebugFuzz seed 21: 5000 cases on arm64,llvm, 1m5s
- TestDebugFuzz seed 22: 5000 cases on arm64,llvm, 1m18s
- TestDebugFuzz seed 23: 5000 cases on arm64,llvm, 1m12s
- TestDebugFuzz seed 24: 5000 cases on arm64,llvm, 1m12s
- TestDebugFuzz seed 25: 5000 cases on arm64,llvm, 1m4s
- TestDebugFuzz seed 26: 5000 cases on arm64,llvm, 1m3s
- TestDebugFuzz seed 27: 5000 cases on arm64,llvm, 1m15s
- TestDebugFuzz seed 28: 5000 cases on arm64,llvm, 1m18s
- TestDebugFuzz seed 29: 5000 cases on arm64,llvm, 1m4s
- TestDebugFuzz seed 30: 5000 cases on arm64,llvm, 1m2s
- TestDebugFuzz seed 31: 5000 cases on arm64,llvm, 1m3s
- TestDebugFuzz seed 32: 5000 cases on arm64,llvm, 1m2s
- TestDebugFuzz seed 33: 5000 cases on arm64,llvm, 1m1s
- TestDebugFuzz seed 34: 5000 cases on arm64,llvm, 1m1s
- TestDebugFuzz seed 35: 5000 cases on arm64,llvm, 59s
- TestDebugFuzz seed 36: 5000 cases on arm64,llvm, 58s
- TestDebugFuzz seed 37: 5000 cases on arm64,llvm, 1m0s
- TestDebugFuzz seed 38: 5000 cases on arm64,llvm, 1m0s
- TestDebugFuzz seed 39: 5000 cases on arm64,llvm, 1m0s
- TestDebugFuzz seed 40: 5000 cases on arm64,llvm, 59s
- TestDebugFuzz seed 41: 5000 cases on arm64,llvm, 59s
- TestDebugFuzz seed 42: 5000 cases on arm64,llvm, 59s
- TestDebugFuzz seed 43: 5000 cases on arm64,llvm, 1m0s
- TestDebugFuzz seed 44: 5000 cases on arm64,llvm, 1m3s
- TestDebugFuzz seed 45: 5000 cases on arm64,llvm, 1m1s
- TestDebugFuzz seed 46: 5000 cases on arm64,llvm, 1m1s
- TestDebugFuzz seed 47: 5000 cases on arm64,llvm, 1m0s
- TestDebugFuzz seed 48: 5000 cases on arm64,llvm, 59s
- TestDebugFuzz seed 49: 5000 cases on arm64,llvm, 59s
- TestDebugFuzz seed 50: 5000 cases on arm64,llvm, 1m0s
- TestDebugFuzz seed 51: 5000 cases on arm64,llvm, 1m1s
- TestDebugFuzz seed 52: 5000 cases on arm64,llvm, 1m2s
- TestDebugFuzz seed 53: 5000 cases on arm64,llvm, 1m2s
- TestDebugFuzz seed 54: 5000 cases on arm64,llvm, 1m5s
