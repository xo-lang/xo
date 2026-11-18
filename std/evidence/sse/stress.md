kind: stress
module: sse
result: pass
commit: ef0cb20d5
source: 3931375663f596a31c33fc5906dd85e96ab3f50e923ed63ea38b2576cbe0bce9
date: 2026-11-18
host: linux/arm64 host 4
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 4" sse
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth from minutes 15 to 17 to the last 3 on 2 of 3 runs a backend (decision 0717), 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 2h6m0s

Stress evidence (decision 0616): the program std/evidence/sse/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64 run 1: sse stress: 254369786 events by 64 subscribers in 30 min over 12568233 streams, every event once and in order, rss 18 MB at the mark (median of minutes 15 to 17), 19 MB at the end (median of the last 3 minutes; 2.0%), peak 19 MB, pass, 30m0s
- arm64 run 2: sse stress: 325549919 events by 64 subscribers in 30 min over 16088121 streams, every event once and in order, rss 18 MB at the mark (median of minutes 15 to 17), 19 MB at the end (median of the last 3 minutes; 5.0%), peak 19 MB, pass, 30m0s
- llvm run 1: sse stress: 424015074 events by 64 subscribers in 30 min over 20949776 streams, every event once and in order, rss 22 MB at the mark (median of minutes 15 to 17), 22 MB at the end (median of the last 3 minutes; 0.0%), peak 22 MB, pass, 30m0s
- llvm run 2: sse stress: 344968411 events by 64 subscribers in 30 min over 17045697 streams, every event once and in order, rss 18 MB at the mark (median of minutes 15 to 17), 19 MB at the end (median of the last 3 minutes; 3.0%), peak 19 MB, pass, 30m0s
- arm64 --check=all: sse stress: 45432084 events by 64 subscribers in 5 min over 2244451 streams, every event once and in order, rss not sampled, pass; xo leakcheck: 0 live of 1538012331 allocations, 5m0s
