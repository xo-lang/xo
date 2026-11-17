kind: stress
module: cbor
result: pass
commit: 224654253
source: fe8e1317bf4eacb3684af4568afb12ccc59fc34d3763b3ec93688922f14335f8
date: 2026-11-17
host: linux/arm64 host 4
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 4" cbor
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 30m59s

Stress evidence (decision 0616): the program std/evidence/cbor/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: cbor stress: 32781394872 operations by 64 tasks in 30 min, 0 mismatches, rss 6 MB at the mark, 6 MB at the end (9.0%), peak 6 MB, pass, 30m1s
- llvm: cbor stress: 31979697096 operations by 64 tasks in 30 min, 0 mismatches, rss 11 MB at the mark, 11 MB at the end (3.0%), peak 11 MB, pass, 30m0s
- arm64 --check=all: cbor stress: 1182191512 operations by 64 tasks in 5 min, 0 mismatches, rss 80 MB at the mark, 80 MB at the end (0.0%), peak 80 MB, pass; xo leakcheck: 0 live of 3427885721 allocations, 5m0s
