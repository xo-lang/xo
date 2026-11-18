kind: fuzz
module: backoff
result: pass
commit: dfb7bd482
source: ed36ec73bdcd2752842d9887f3d675a6bdaf916ac7e4d52ab04f8b6a00b4b691
date: 2026-11-18
host: linux/arm64 host 4
command: go run ./scripts/stdproven fuzz -cases 1000000 -hours 8 -batch 5000 -backends arm64,llvm -host "linux/arm64 host 4" backoff
measure: TestBackoffFuzz 695000 cases per backend, 2 backends (arm64,llvm), 8h2m53s in 3 shards, 0 mismatches
seeds: 2 to 140
ran: TestBackoffFuzz=695000
elapsed: 28973
shards: 3

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
- TestBackoffFuzz seed 56: 5000 cases on arm64,llvm, 3m14s
- TestBackoffFuzz seed 57: 5000 cases on arm64,llvm, 3m3s
- TestBackoffFuzz seed 58: 5000 cases on arm64,llvm, 3m13s
- TestBackoffFuzz seed 59: 5000 cases on arm64,llvm, 3m11s
- TestBackoffFuzz seed 60: 5000 cases on arm64,llvm, 3m6s
- TestBackoffFuzz seed 61: 5000 cases on arm64,llvm, 3m2s
- TestBackoffFuzz seed 62: 5000 cases on arm64,llvm, 3m11s
- TestBackoffFuzz seed 63: 5000 cases on arm64,llvm, 3m10s
- TestBackoffFuzz seed 64: 5000 cases on arm64,llvm, 3m10s
- TestBackoffFuzz seed 65: 5000 cases on arm64,llvm, 3m9s
- TestBackoffFuzz seed 66: 5000 cases on arm64,llvm, 3m18s
- TestBackoffFuzz seed 67: 5000 cases on arm64,llvm, 3m21s
- TestBackoffFuzz seed 68: 5000 cases on arm64,llvm, 3m31s
- TestBackoffFuzz seed 69: 5000 cases on arm64,llvm, 3m21s
- TestBackoffFuzz seed 70: 5000 cases on arm64,llvm, 3m17s
- TestBackoffFuzz seed 71: 5000 cases on arm64,llvm, 3m18s
- TestBackoffFuzz seed 72: 5000 cases on arm64,llvm, 3m20s
- TestBackoffFuzz seed 73: 5000 cases on arm64,llvm, 3m19s
- TestBackoffFuzz seed 74: 5000 cases on arm64,llvm, 3m30s
- TestBackoffFuzz seed 75: 5000 cases on arm64,llvm, 3m24s
- TestBackoffFuzz seed 76: 5000 cases on arm64,llvm, 3m56s
- TestBackoffFuzz seed 77: 5000 cases on arm64,llvm, 3m51s
- TestBackoffFuzz seed 78: 5000 cases on arm64,llvm, 3m35s
- TestBackoffFuzz seed 79: 5000 cases on arm64,llvm, 3m31s
- TestBackoffFuzz seed 80: 5000 cases on arm64,llvm, 3m25s
- TestBackoffFuzz seed 81: 5000 cases on arm64,llvm, 3m35s
- TestBackoffFuzz seed 82: 5000 cases on arm64,llvm, 3m46s
- TestBackoffFuzz seed 83: 5000 cases on arm64,llvm, 3m37s
- TestBackoffFuzz seed 84: 5000 cases on arm64,llvm, 3m30s
- TestBackoffFuzz seed 85: 5000 cases on arm64,llvm, 3m26s
- TestBackoffFuzz seed 86: 5000 cases on arm64,llvm, 3m29s
- TestBackoffFuzz seed 87: 5000 cases on arm64,llvm, 3m21s
- TestBackoffFuzz seed 88: 5000 cases on arm64,llvm, 3m19s
- TestBackoffFuzz seed 89: 5000 cases on arm64,llvm, 3m14s
- TestBackoffFuzz seed 90: 5000 cases on arm64,llvm, 3m10s
- TestBackoffFuzz seed 91: 5000 cases on arm64,llvm, 3m21s
- TestBackoffFuzz seed 92: 5000 cases on arm64,llvm, 3m20s
- TestBackoffFuzz seed 93: 5000 cases on arm64,llvm, 3m25s
- TestBackoffFuzz seed 94: 5000 cases on arm64,llvm, 3m38s
- TestBackoffFuzz seed 95: 5000 cases on arm64,llvm, 3m18s
- TestBackoffFuzz seed 96: 5000 cases on arm64,llvm, 3m12s
- TestBackoffFuzz seed 97: 5000 cases on arm64,llvm, 3m27s
- TestBackoffFuzz seed 98: 5000 cases on arm64,llvm, 3m22s
- TestBackoffFuzz seed 99: 5000 cases on arm64,llvm, 3m24s
- TestBackoffFuzz seed 100: 5000 cases on arm64,llvm, 3m13s
- TestBackoffFuzz seed 101: 5000 cases on arm64,llvm, 3m17s
- TestBackoffFuzz seed 102: 5000 cases on arm64,llvm, 3m28s
- TestBackoffFuzz seed 103: 5000 cases on arm64,llvm, 3m41s
- TestBackoffFuzz seed 104: 5000 cases on arm64,llvm, 3m44s
- TestBackoffFuzz seed 105: 5000 cases on arm64,llvm, 3m47s
- TestBackoffFuzz seed 106: 5000 cases on arm64,llvm, 3m38s
- TestBackoffFuzz seed 107: 5000 cases on arm64,llvm, 3m33s
- TestBackoffFuzz seed 108: 5000 cases on arm64,llvm, 3m49s
- TestBackoffFuzz seed 109: 5000 cases on arm64,llvm, 3m38s
- TestBackoffFuzz seed 110: 5000 cases on arm64,llvm, 3m21s
- TestBackoffFuzz seed 111: 5000 cases on arm64,llvm, 3m19s
- TestBackoffFuzz seed 112: 5000 cases on arm64,llvm, 3m11s
- TestBackoffFuzz seed 113: 5000 cases on arm64,llvm, 3m16s
- TestBackoffFuzz seed 114: 5000 cases on arm64,llvm, 3m28s
- TestBackoffFuzz seed 115: 5000 cases on arm64,llvm, 3m14s
- TestBackoffFuzz seed 116: 5000 cases on arm64,llvm, 3m13s
- TestBackoffFuzz seed 117: 5000 cases on arm64,llvm, 3m16s
- TestBackoffFuzz seed 118: 5000 cases on arm64,llvm, 3m16s
- TestBackoffFuzz seed 119: 5000 cases on arm64,llvm, 3m2s
- TestBackoffFuzz seed 120: 5000 cases on arm64,llvm, 3m9s
- TestBackoffFuzz seed 121: 5000 cases on arm64,llvm, 3m4s
- TestBackoffFuzz seed 122: 5000 cases on arm64,llvm, 2m57s
- TestBackoffFuzz seed 123: 5000 cases on arm64,llvm, 3m4s
- TestBackoffFuzz seed 124: 5000 cases on arm64,llvm, 2m59s
- TestBackoffFuzz seed 125: 5000 cases on arm64,llvm, 3m5s
- TestBackoffFuzz seed 126: 5000 cases on arm64,llvm, 3m3s
- TestBackoffFuzz seed 127: 5000 cases on arm64,llvm, 3m0s
- TestBackoffFuzz seed 128: 5000 cases on arm64,llvm, 2m57s
- TestBackoffFuzz seed 129: 5000 cases on arm64,llvm, 3m5s
- TestBackoffFuzz seed 130: 5000 cases on arm64,llvm, 3m4s
- TestBackoffFuzz seed 131: 5000 cases on arm64,llvm, 2m59s
- TestBackoffFuzz seed 132: 5000 cases on arm64,llvm, 3m20s
- TestBackoffFuzz seed 133: 5000 cases on arm64,llvm, 3m14s
- TestBackoffFuzz seed 134: 5000 cases on arm64,llvm, 3m28s
- TestBackoffFuzz seed 135: 5000 cases on arm64,llvm, 3m9s
- TestBackoffFuzz seed 136: 5000 cases on arm64,llvm, 3m17s
- TestBackoffFuzz seed 137: 5000 cases on arm64,llvm, 3m14s
- TestBackoffFuzz seed 138: 5000 cases on arm64,llvm, 3m5s
- TestBackoffFuzz seed 139: 5000 cases on arm64,llvm, 3m15s
- TestBackoffFuzz seed 140: 5000 cases on arm64,llvm, 3m14s
