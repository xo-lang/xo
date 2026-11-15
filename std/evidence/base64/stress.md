kind: stress
module: base64
result: pass
commit: 41068182d
source: efd9b1731ce03a28532b686126bc1f938006e447a93cc3c6b91f8e9cdee37346
date: 2026-11-15
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 3" base64
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h5m9s

Stress evidence (decision 0616): the program std/evidence/base64/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: base64 stress: 159669743454 operations by 64 tasks in 30 min, 0 mismatches, rss 4 MB at the mark, 2 MB at the end (-40.0%), peak 4 MB, pass, 30m0s
- llvm: base64 stress: 84363063174 operations by 64 tasks in 30 min, 0 mismatches, rss 4 MB at the mark, 2 MB at the end (-43.0%), peak 4 MB, pass, 30m0s
- arm64 --check=all: base64 stress: 3330171468 operations by 64 tasks in 5 min, 0 mismatches, rss 235 MB at the mark, 235 MB at the end (0.0%), peak 235 MB, pass; xo leakcheck: 0 live of 9093930302 allocations, 5m0s
