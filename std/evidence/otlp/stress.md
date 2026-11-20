kind: stress
module: otlp
result: pass
commit: 633c73343
source: 9466e2dcea06619dcb8da5bf7cd320d2aac412e95f681a9db445eec98e69d338
date: 2026-11-20
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 3" otlp
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth from minutes 15 to 17 to the last 3 on 2 of 3 runs a backend (decision 0717), 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 2h5m49s

Stress evidence (decision 0616): the program std/evidence/otlp/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64 run 1: otlp stress: 143504880 exports by 64 tasks in 30 min (137265536 traces and 14350462 metrics requests taken), 0 wrong, rss 59 MB at the mark (median of minutes 15 to 17), 62 MB at the end (median of the last 3 minutes; 5.0%), peak 63 MB, pass, 30m0s
- arm64 run 2: otlp stress: 217926059 exports by 64 tasks in 30 min (208451020 traces and 21792577 metrics requests taken), 0 wrong, rss 72 MB at the mark (median of minutes 15 to 17), 74 MB at the end (median of the last 3 minutes; 3.0%), peak 74 MB, pass, 30m0s
- llvm run 1: otlp stress: 227422799 exports by 64 tasks in 30 min (217534854 traces and 22742255 metrics requests taken), 0 wrong, rss 72 MB at the mark (median of minutes 15 to 17), 73 MB at the end (median of the last 3 minutes; 2.0%), peak 74 MB, pass, 30m0s
- llvm run 2: otlp stress: 192011829 exports by 64 tasks in 30 min (183663490 traces and 19201154 metrics requests taken), 0 wrong, rss 70 MB at the mark (median of minutes 15 to 17), 74 MB at the end (median of the last 3 minutes; 6.0%), peak 74 MB, pass, 30m0s
- arm64 --check=all: otlp stress: 16920201 exports by 64 tasks in 5 min (16184542 traces and 1691991 metrics requests taken), 0 wrong, rss not sampled, pass; xo leakcheck: 0 live of 10921033341 allocations, 5m0s
