kind: stress
module: mail
result: pass
commit: 32c89673e
source: 61c1dcd3140a6bdae7cdde2808f63f4a1f814f02535ceb0729ece549f9eb09ca
date: 2026-11-17
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 2" mail
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 31m3s

Stress evidence (decision 0616): the program std/evidence/mail/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: mail stress: 816299862 messages by 64 tasks in 30 min, 0 identities broken, rss 6 MB at the mark, 7 MB at the end (3.0%), peak 7 MB, pass, 30m0s
- llvm: mail stress: 856105760 messages by 64 tasks in 30 min, 0 identities broken, rss 7 MB at the mark, 7 MB at the end (3.0%), peak 7 MB, pass, 30m0s
- arm64 --check=all: mail stress: 39959782 messages by 64 tasks in 5 min, 0 identities broken, rss 213 MB at the mark, 213 MB at the end (0.0%), peak 213 MB, pass; xo leakcheck: 0 live of 9820532229 allocations, 5m0s
