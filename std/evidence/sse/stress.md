kind: stress
module: sse
result: pass
commit: 138b6ba21
source: 3931375663f596a31c33fc5906dd85e96ab3f50e923ed63ea38b2576cbe0bce9
date: 2026-11-18
host: linux/arm64 host 4
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 4" sse
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth from minutes 15 to 17 to the last 3 on 2 of 3 runs a backend (decision 0717), 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h2m22s

Stress evidence (decision 0616): the program std/evidence/sse/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64 run 1: sse stress: 397759753 events by 64 subscribers in 30 min over 19655943 streams, every event once and in order, rss 17 MB at the mark (median of minutes 15 to 17), 18 MB at the end (median of the last 3 minutes; 2.0%), peak 18 MB, pass, 30m0s
- arm64 run 2: sse stress: 417378928 events by 64 subscribers in 30 min over 20616163 streams, every event once and in order, rss 18 MB at the mark (median of minutes 15 to 17), 18 MB at the end (median of the last 3 minutes; 4.0%), peak 18 MB, pass, 30m0s
- llvm run 1: sse stress: 388493703 events by 64 subscribers in 30 min over 19194913 streams, every event once and in order, rss 20 MB at the mark (median of minutes 15 to 17), 20 MB at the end (median of the last 3 minutes; 2.0%), peak 20 MB, pass, 30m0s
- llvm run 2: sse stress: 437856982 events by 64 subscribers in 30 min over 21639632 streams, every event once and in order, rss 19 MB at the mark (median of minutes 15 to 17), 19 MB at the end (median of the last 3 minutes; 4.0%), peak 20 MB, pass, 30m0s
- arm64 --check=all: sse stress: 35022886 events by 64 subscribers in 5 min over 1730630 streams, every event once and in order, rss not sampled, pass; xo leakcheck: 0 live of 1185814749 allocations, 5m0s
