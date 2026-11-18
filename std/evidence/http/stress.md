kind: stress
module: http
result: pass
commit: (private commit)
source: 84028521d101656883a90a16f5e00d780428c97ff3a8a00ab3269ab00d5e76ce
date: 2026-11-18
host: linux/arm64 host 4
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 4" http
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth from minutes 15 to 17 to the last 3 on 2 of 3 runs a backend (decision 0717), 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h6m44s

Stress evidence (decision 0616): the program std/evidence/http/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64 run 1: http stress: 56996508 requests by 64 clients and 20618 hostile connections by 8 in 30 min, 0 mismatches, rss 48 MB at the mark (median of minutes 15 to 17), 49 MB at the end (median of the last 3 minutes; 2.0%), peak 49 MB, pass, 30m2s
- arm64 run 2: http stress: 58344804 requests by 64 clients and 20972 hostile connections by 8 in 30 min, 0 mismatches, rss 52 MB at the mark (median of minutes 15 to 17), 52 MB at the end (median of the last 3 minutes; 1.0%), peak 52 MB, pass, 30m4s
- llvm run 1: http stress: 53014713 requests by 64 clients and 21014 hostile connections by 8 in 30 min, 0 mismatches, rss 50 MB at the mark (median of minutes 15 to 17), 51 MB at the end (median of the last 3 minutes; 2.0%), peak 52 MB, pass, 30m2s
- llvm run 2: http stress: 37922433 requests by 64 clients and 20756 hostile connections by 8 in 30 min, 0 mismatches, rss 53 MB at the mark (median of minutes 15 to 17), 54 MB at the end (median of the last 3 minutes; 2.0%), peak 54 MB, pass, 30m3s
- arm64 --check=all: http stress: 3619185 requests by 64 clients and 3137 hostile connections by 8 in 5 min, 0 mismatches, rss not sampled, pass; xo leakcheck: 0 live of 577940790 allocations, 5m3s
