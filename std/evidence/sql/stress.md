kind: stress
module: sql
result: pass
commit: ff6fe1e36
source: e21162283a21425dd70161fd02deb6f465aca1e9b700d8a16d5e140b5a9b9a28
date: 2026-11-17
host: linux/arm64
command: go run ./scripts/stdproven stress -minutes 60 -tasks 64 -backends arm64 -check-minutes 1 -host "linux/arm64" sql
measure: 64 tasks for 60 min on each of arm64, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 1 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h1m18s

Stress evidence (decision 0616): the program std/evidence/sql/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: sql stress: 239352820 statements by 64 clients over 8 pools in 60 min, 0 mismatches, rss 59 MB at the mark, 54 MB at the end (-8.0%), peak 62 MB, pass, 1h0m0s
- arm64 --check=all: sql stress: 1128955 statements by 64 clients over 8 pools in 1 min, 0 mismatches, rss not sampled, pass; xo leakcheck: 0 live of 142140246 allocations, 1m0s
