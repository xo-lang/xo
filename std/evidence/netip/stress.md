kind: stress
module: netip
result: pass
commit: 224654253
source: 6aeea21f62d1abfcd2937ac1d0793408dede9586a9305b2d1c3514244e07034b
date: 2026-11-17
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 2" netip
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 30m38s

Stress evidence (decision 0616): the program std/evidence/netip/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: netip stress: 2246449854 addresses by 64 tasks in 30 min, 0 identities broken, rss 8 MB at the mark, 9 MB at the end (15.0%), peak 9 MB, pass, 30m0s
- llvm: netip stress: 2119510890 addresses by 64 tasks in 30 min, 0 identities broken, rss 8 MB at the mark, 8 MB at the end (7.0%), peak 8 MB, pass, 30m0s
- arm64 --check=all: netip stress: 91027391 addresses by 64 tasks in 5 min, 0 identities broken, rss 89 MB at the mark, 94 MB at the end (5.0%), peak 94 MB, pass; xo leakcheck: 0 live of 6584444836 allocations, 5m0s
