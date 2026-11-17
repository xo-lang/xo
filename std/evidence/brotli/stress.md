kind: stress
module: brotli
result: pass
commit: a2dcc9bc7
source: 0e008588c519c65b551a580d3401c7090817d3f7088cfd71fb66b4b330a58c4b
date: 2026-11-17
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64 -check-minutes 5 -host "linux/arm64 host 3" brotli
measure: 64 tasks for 30 min on each of arm64, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 35m20s

Stress evidence (decision 0616): the program std/evidence/brotli/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: brotli stress: 5428221 operations by 64 tasks in 30 min, 0 mismatches, rss 1771 MB at the mark, 971 MB at the end (-45.0%), peak 2144 MB, pass, 30m3s
- arm64 --check=all: brotli stress: 633526 operations by 64 tasks in 5 min, 0 mismatches, rss 2242 MB at the mark, 2265 MB at the end (1.0%), peak 2403 MB, pass; xo leakcheck: 0 live of 1671970920 allocations, 5m4s
