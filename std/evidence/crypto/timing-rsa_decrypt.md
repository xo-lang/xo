kind: timing
module: crypto
result: pass
commit: e54216b12
source: 768e3d1221f5a98e3846035080fec81b47eb85aaf5e84a608727c2447ef394b4
date: 2026-11-12
host: linux/arm64 host 1
command: go run ./scripts/stdproven timing -n 1000000 -backends arm64,llvm -pairs oaep_first:oaep_lhash,oaep_lhash:oaep_sep,v15_head:v15_nozero,v15_nozero:v15_short -host "linux/arm64 host 1" crypto rsa_decrypt
measure: 8 pairs on arm64,llvm, 1000000 measurements each, max |t| 1.923 (pass below 4.5), 13m45s

Timing evidence (decision 0616 checklist item 3): the dudect program std/evidence/crypto/timing/rsa_decrypt, Welch's t over the times of two input classes, uncropped and cropped at several percentiles.

- arm64: oaep_first vs oaep_lhash: 1000000 measurements, max |t| 1.85282370515886, pass
- arm64: oaep_lhash vs oaep_sep: 1000000 measurements, max |t| 1.7015438487824592, pass
- arm64: v15_head vs v15_nozero: 1000000 measurements, max |t| 1.1679625155018247, pass
- arm64: v15_nozero vs v15_short: 1000000 measurements, max |t| 1.3521655682078595, pass
- llvm: oaep_first vs oaep_lhash: 1000000 measurements, max |t| 0.3237661678446145, pass
- llvm: oaep_lhash vs oaep_sep: 1000000 measurements, max |t| 1.4436408805184413, pass
- llvm: v15_head vs v15_nozero: 1000000 measurements, max |t| 0.7419391209123336, pass
- llvm: v15_nozero vs v15_short: 1000000 measurements, max |t| 1.922625722898949, pass
