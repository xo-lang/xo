kind: stress
module: otlp
result: pass
commit: ef0cb20d5
source: e3a8fc78fecbfc2aa761830679145ef36e8a3ace559656bed4a8379a23125753
date: 2026-11-18
host: linux/arm64
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64" otlp
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth from minutes 15 to 17 to the last 3 on 2 of 3 runs a backend (decision 0717), 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 2h37m33s

Stress evidence (decision 0616): the program std/evidence/otlp/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64 run 1: otlp stress: 162173685 exports by 64 tasks in 30 min (155122654 traces and 16217343 metrics requests taken), 0 wrong, rss 91 MB at the mark (median of minutes 15 to 17), 95 MB at the end (median of the last 3 minutes; 4.0%), peak 95 MB, pass, 30m0s
- arm64 run 2: otlp stress: 95708678 exports by 64 tasks in 30 min (91547433 traces and 9570835 metrics requests taken), 0 wrong, rss 83 MB at the mark (median of minutes 15 to 17), 86 MB at the end (median of the last 3 minutes; 4.0%), peak 90 MB, pass, 30m0s
- llvm run 1: otlp stress: 241376426 exports by 64 tasks in 30 min (230881799 traces and 24137613 metrics requests taken), 0 wrong, rss 91 MB at the mark (median of minutes 15 to 17), 95 MB at the end (median of the last 3 minutes; 5.0%), peak 97 MB, pass, 30m0s
- llvm run 2: otlp stress: 341339482 exports by 64 tasks in 30 min (326498631 traces and 34133921 metrics requests taken), 0 wrong, rss 92 MB at the mark (median of minutes 15 to 17), 103 MB at the end (median of the last 3 minutes; 11.0%), peak 103 MB, FAIL (memory grew), 30m0s
- llvm run 3: otlp stress: 286720645 exports by 64 tasks in 30 min (274254529 traces and 28672038 metrics requests taken), 0 wrong, rss 105 MB at the mark (median of minutes 15 to 17), 101 MB at the end (median of the last 3 minutes; -4.0%), peak 110 MB, pass, 30m0s
- arm64 --check=all: otlp stress: 10100221 exports by 64 tasks in 5 min (9661083 traces and 1009998 metrics requests taken), 0 wrong, rss not sampled, pass; xo leakcheck: 0 live of 6524759163 allocations, 5m0s
