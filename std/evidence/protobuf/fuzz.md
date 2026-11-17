kind: fuzz
module: protobuf
result: pass
commit: 3ee30e40b
source: 60304805bf37b3c5deea63383cba965beb1f1a614dd538624505c54449ececc3
date: 2026-11-17
host: darwin/arm64
command: go run ./scripts/stdproven fuzz -cases 1000000 -hours 8 -batch 5000 -backends arm64,llvm -host "" protobuf
measure: TestProtobufFuzz 1001504 cases per backend, 2 backends (arm64,llvm), 13m40s, 0 mismatches
seeds: 2 to 113

Long fuzz evidence (decision 0616): every harness that names std/protobuf, against Go's package, with fresh seeds of 5000 cases each.

- TestProtobufFuzz seed 2: 8942 cases on arm64,llvm, 1m4s
- TestProtobufFuzz seed 3: 8942 cases on arm64,llvm, 9s
- TestProtobufFuzz seed 4: 8942 cases on arm64,llvm, 8s
- TestProtobufFuzz seed 5: 8942 cases on arm64,llvm, 8s
- TestProtobufFuzz seed 6: 8942 cases on arm64,llvm, 7s
- TestProtobufFuzz seed 7: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 8: 8942 cases on arm64,llvm, 7s
- TestProtobufFuzz seed 9: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 10: 8942 cases on arm64,llvm, 7s
- TestProtobufFuzz seed 11: 8942 cases on arm64,llvm, 7s
- TestProtobufFuzz seed 12: 8942 cases on arm64,llvm, 7s
- TestProtobufFuzz seed 13: 8942 cases on arm64,llvm, 7s
- TestProtobufFuzz seed 14: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 15: 8942 cases on arm64,llvm, 8s
- TestProtobufFuzz seed 16: 8942 cases on arm64,llvm, 8s
- TestProtobufFuzz seed 17: 8942 cases on arm64,llvm, 8s
- TestProtobufFuzz seed 18: 8942 cases on arm64,llvm, 8s
- TestProtobufFuzz seed 19: 8942 cases on arm64,llvm, 7s
- TestProtobufFuzz seed 20: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 21: 8942 cases on arm64,llvm, 7s
- TestProtobufFuzz seed 22: 8942 cases on arm64,llvm, 7s
- TestProtobufFuzz seed 23: 8942 cases on arm64,llvm, 7s
- TestProtobufFuzz seed 24: 8942 cases on arm64,llvm, 10s
- TestProtobufFuzz seed 25: 8942 cases on arm64,llvm, 11s
- TestProtobufFuzz seed 26: 8942 cases on arm64,llvm, 10s
- TestProtobufFuzz seed 27: 8942 cases on arm64,llvm, 7s
- TestProtobufFuzz seed 28: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 29: 8942 cases on arm64,llvm, 7s
- TestProtobufFuzz seed 30: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 31: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 32: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 33: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 34: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 35: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 36: 8942 cases on arm64,llvm, 7s
- TestProtobufFuzz seed 37: 8942 cases on arm64,llvm, 10s
- TestProtobufFuzz seed 38: 8942 cases on arm64,llvm, 9s
- TestProtobufFuzz seed 39: 8942 cases on arm64,llvm, 9s
- TestProtobufFuzz seed 40: 8942 cases on arm64,llvm, 10s
- TestProtobufFuzz seed 41: 8942 cases on arm64,llvm, 8s
- TestProtobufFuzz seed 42: 8942 cases on arm64,llvm, 7s
- TestProtobufFuzz seed 43: 8942 cases on arm64,llvm, 7s
- TestProtobufFuzz seed 44: 8942 cases on arm64,llvm, 7s
- TestProtobufFuzz seed 45: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 46: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 47: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 48: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 49: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 50: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 51: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 52: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 53: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 54: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 55: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 56: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 57: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 58: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 59: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 60: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 61: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 62: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 63: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 64: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 65: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 66: 8942 cases on arm64,llvm, 7s
- TestProtobufFuzz seed 67: 8942 cases on arm64,llvm, 7s
- TestProtobufFuzz seed 68: 8942 cases on arm64,llvm, 9s
- TestProtobufFuzz seed 69: 8942 cases on arm64,llvm, 7s
- TestProtobufFuzz seed 70: 8942 cases on arm64,llvm, 8s
- TestProtobufFuzz seed 71: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 72: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 73: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 74: 8942 cases on arm64,llvm, 7s
- TestProtobufFuzz seed 75: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 76: 8942 cases on arm64,llvm, 7s
- TestProtobufFuzz seed 77: 8942 cases on arm64,llvm, 7s
- TestProtobufFuzz seed 78: 8942 cases on arm64,llvm, 7s
- TestProtobufFuzz seed 79: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 80: 8942 cases on arm64,llvm, 7s
- TestProtobufFuzz seed 81: 8942 cases on arm64,llvm, 7s
- TestProtobufFuzz seed 82: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 83: 8942 cases on arm64,llvm, 8s
- TestProtobufFuzz seed 84: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 85: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 86: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 87: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 88: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 89: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 90: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 91: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 92: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 93: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 94: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 95: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 96: 8942 cases on arm64,llvm, 7s
- TestProtobufFuzz seed 97: 8942 cases on arm64,llvm, 7s
- TestProtobufFuzz seed 98: 8942 cases on arm64,llvm, 7s
- TestProtobufFuzz seed 99: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 100: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 101: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 102: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 103: 8942 cases on arm64,llvm, 7s
- TestProtobufFuzz seed 104: 8942 cases on arm64,llvm, 7s
- TestProtobufFuzz seed 105: 8942 cases on arm64,llvm, 7s
- TestProtobufFuzz seed 106: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 107: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 108: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 109: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 110: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 111: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 112: 8942 cases on arm64,llvm, 6s
- TestProtobufFuzz seed 113: 8942 cases on arm64,llvm, 6s
