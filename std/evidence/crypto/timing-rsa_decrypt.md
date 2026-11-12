kind: timing
module: crypto
result: pass
commit: 716c0f0e7
source: 9e08e7fafcd7a1b585731cc8739c18aec7bb2c2f1d1cd6038b318f7aa939da96
date: 2026-11-12
host: linux/arm64 host 1
command: go run ./scripts/stdproven timing -n 1000000 -backends arm64,llvm -pairs oaep_first:oaep_lhash,oaep_lhash:oaep_sep,v15_head:v15_nozero,v15_nozero:v15_short,oaep_sep:oaep_sep,v15_nozero:v15_nozero -host "linux/arm64 host 1" crypto rsa_decrypt
measure: 12 pairs on arm64,llvm, 1000000 measurements each, max |t| 3.357 (pass below 4.5), 22m10s

Timing evidence (decision 0616 checklist item 3): the dudect program std/evidence/crypto/timing/rsa_decrypt, Welch's t over the times of two input classes, uncropped and cropped at several percentiles.

- arm64: oaep_first vs oaep_lhash: 1000000 measurements, max |t| 3.3570010505864665, pass
- arm64: oaep_lhash vs oaep_sep: 1000000 measurements, max |t| 0.6053407979015247, pass
- arm64: v15_head vs v15_nozero: 1000000 measurements, max |t| 0.8879431702740935, pass
- arm64: v15_nozero vs v15_short: 1000000 measurements, max |t| 1.159050454497849, pass
- arm64: oaep_sep vs oaep_sep: 1000000 measurements, max |t| 1.4432098318327267, pass
- arm64: v15_nozero vs v15_nozero: 1000000 measurements, max |t| 0.4483789879729881, pass
- llvm: oaep_first vs oaep_lhash: 1000000 measurements, max |t| 2.029754026776217, pass
- llvm: oaep_lhash vs oaep_sep: 1000000 measurements, max |t| 1.5244614852904197, pass
- llvm: v15_head vs v15_nozero: 1000000 measurements, max |t| 1.6103265910318973, pass
- llvm: v15_nozero vs v15_short: 1000000 measurements, max |t| 1.6288308653914636, pass
- llvm: oaep_sep vs oaep_sep: 1000000 measurements, max |t| 1.621050255910926, pass
- llvm: v15_nozero vs v15_nozero: 1000000 measurements, max |t| 0.7478398924625084, pass
