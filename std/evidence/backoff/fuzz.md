kind: fuzz
module: backoff
result: partial
commit: fa469d872
source: ed36ec73bdcd2752842d9887f3d675a6bdaf916ac7e4d52ab04f8b6a00b4b691
date: 2026-11-18
host: linux/arm64 host 4
command: go run ./scripts/stdproven fuzz -cases 1000000 -hours 8 -batch 5000 -backends arm64,llvm -host "linux/arm64 host 4" backoff
measure: TestBackoffFuzz 270000 cases per backend, 2 backends (arm64,llvm), 3h20m29s in 1 shards, 0 mismatches
seeds: 2 to 55
ran: TestBackoffFuzz=270000
elapsed: 12028
shards: 1

Long fuzz evidence (decision 0616): every harness that names std/backoff, against Go's package, with fresh seeds of 5000 cases each.

- TestBackoffFuzz seed 2: 5000 cases on arm64,llvm, 4m43s
- TestBackoffFuzz seed 3: 5000 cases on arm64,llvm, 3m34s
- TestBackoffFuzz seed 4: 5000 cases on arm64,llvm, 3m31s
- TestBackoffFuzz seed 5: 5000 cases on arm64,llvm, 3m27s
- TestBackoffFuzz seed 6: 5000 cases on arm64,llvm, 3m40s
- TestBackoffFuzz seed 7: 5000 cases on arm64,llvm, 3m51s
- TestBackoffFuzz seed 8: 5000 cases on arm64,llvm, 3m34s
- TestBackoffFuzz seed 9: 5000 cases on arm64,llvm, 3m28s
- TestBackoffFuzz seed 10: 5000 cases on arm64,llvm, 3m23s
- TestBackoffFuzz seed 11: 5000 cases on arm64,llvm, 3m49s
- TestBackoffFuzz seed 12: 5000 cases on arm64,llvm, 3m59s
- TestBackoffFuzz seed 13: 5000 cases on arm64,llvm, 4m9s
- TestBackoffFuzz seed 14: 5000 cases on arm64,llvm, 4m10s
- TestBackoffFuzz seed 15: 5000 cases on arm64,llvm, 6m44s
- TestBackoffFuzz seed 16: 5000 cases on arm64,llvm, 5m3s
- TestBackoffFuzz seed 17: 5000 cases on arm64,llvm, 4m34s
- TestBackoffFuzz seed 18: 5000 cases on arm64,llvm, 4m50s
- TestBackoffFuzz seed 19: 5000 cases on arm64,llvm, 3m14s
- TestBackoffFuzz seed 20: 5000 cases on arm64,llvm, 3m11s
- TestBackoffFuzz seed 21: 5000 cases on arm64,llvm, 3m22s
- TestBackoffFuzz seed 22: 5000 cases on arm64,llvm, 3m52s
- TestBackoffFuzz seed 23: 5000 cases on arm64,llvm, 5m34s
- TestBackoffFuzz seed 24: 5000 cases on arm64,llvm, 4m16s
- TestBackoffFuzz seed 25: 5000 cases on arm64,llvm, 4m10s
- TestBackoffFuzz seed 26: 5000 cases on arm64,llvm, 4m24s
- TestBackoffFuzz seed 27: 5000 cases on arm64,llvm, 4m26s
- TestBackoffFuzz seed 28: 5000 cases on arm64,llvm, 4m9s
- TestBackoffFuzz seed 29: 5000 cases on arm64,llvm, 3m46s
- TestBackoffFuzz seed 30: 5000 cases on arm64,llvm, 3m55s
- TestBackoffFuzz seed 31: 5000 cases on arm64,llvm, 3m44s
- TestBackoffFuzz seed 32: 5000 cases on arm64,llvm, 3m54s
- TestBackoffFuzz seed 33: 5000 cases on arm64,llvm, 3m48s
- TestBackoffFuzz seed 34: 5000 cases on arm64,llvm, 3m49s
- TestBackoffFuzz seed 35: 5000 cases on arm64,llvm, 3m39s
- TestBackoffFuzz seed 36: 5000 cases on arm64,llvm, 3m39s
- TestBackoffFuzz seed 37: 5000 cases on arm64,llvm, 3m38s
- TestBackoffFuzz seed 38: 5000 cases on arm64,llvm, 3m31s
- TestBackoffFuzz seed 39: 5000 cases on arm64,llvm, 3m33s
- TestBackoffFuzz seed 40: 5000 cases on arm64,llvm, 3m46s
- TestBackoffFuzz seed 41: 5000 cases on arm64,llvm, 3m2s
- TestBackoffFuzz seed 42: 5000 cases on arm64,llvm, 2m56s
- TestBackoffFuzz seed 43: 5000 cases on arm64,llvm, 2m53s
- TestBackoffFuzz seed 44: 5000 cases on arm64,llvm, 2m49s
- TestBackoffFuzz seed 45: 5000 cases on arm64,llvm, 2m56s
- TestBackoffFuzz seed 46: 5000 cases on arm64,llvm, 2m49s
- TestBackoffFuzz seed 47: 5000 cases on arm64,llvm, 2m55s
- TestBackoffFuzz seed 48: 5000 cases on arm64,llvm, 3m4s
- TestBackoffFuzz seed 49: 5000 cases on arm64,llvm, 2m55s
- TestBackoffFuzz seed 50: 5000 cases on arm64,llvm, 3m4s
- TestBackoffFuzz seed 51: 5000 cases on arm64,llvm, 3m0s
- TestBackoffFuzz seed 52: 5000 cases on arm64,llvm, 2m55s
- TestBackoffFuzz seed 53: 5000 cases on arm64,llvm, 2m58s
- TestBackoffFuzz seed 54: 5000 cases on arm64,llvm, 3m4s
- TestBackoffFuzz seed 55: 5000 cases on arm64,llvm, 3m17s
