kind: stress
module: grpcreflect
result: pass
commit: d2dbc9517
source: 0fea72ad674f126f62bd1fe073bff533e48370dc3b58110c8655e65e85e69a59
date: 2026-11-20
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 60 -tasks 64 -backends arm64 -check-minutes 0 -host "linux/arm64 host 3" grpcreflect
measure: 64 tasks for 60 min on each of arm64, 0 mismatches, 0 faults, memory under 10% growth from minutes 15 to 17 to the last 3 on 2 of 3 runs a backend (decision 0717), 3h0m40s

Stress evidence (decision 0616): the program std/evidence/grpcreflect/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64 run 1: grpcreflect stress: 194864392 rounds by 64 tasks in 60 min, every answer as expected, rss 93 MB at the mark (median of minutes 30 to 32), 95 MB at the end (median of the last 3 minutes; 2.0%), peak 96 MB, pass, 1h0m0s
- arm64 run 2: grpcreflect stress: 254842752 rounds by 64 tasks in 60 min, every answer as expected, rss 90 MB at the mark (median of minutes 30 to 32), 97 MB at the end (median of the last 3 minutes; 8.0%), peak 98 MB, FAIL (memory grew), 1h0m1s
- arm64 run 3: grpcreflect stress: 206544258 rounds by 64 tasks in 60 min, every answer as expected, rss 90 MB at the mark (median of minutes 30 to 32), 94 MB at the end (median of the last 3 minutes; 4.0%), peak 94 MB, pass, 1h0m30s
