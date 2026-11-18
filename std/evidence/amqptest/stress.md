kind: stress
module: amqptest
result: pass
commit: bed1e8b29
source: 58ab9bb135b3c6494ccc87cd00ffcc6cee332ced8e5eef9817eff6109b64aab4
date: 2026-11-18
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 3" amqptest
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h5m42s

Stress evidence (decision 0616): the program std/evidence/amqptest/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: amqptest stress: 23708788 messages by 64 producers, 23536298 accepted once and 172490 dead lettered by 64 consumers in 30 min, none lost, rss 57 MB at the mark (median of minutes 5 to 7), 55 MB at the end (median of the last 3 minutes; -4.0%), peak 61 MB, pass, 30m1s
- llvm: amqptest stress: 17941961 messages by 64 producers, 17811120 accepted once and 130841 dead lettered by 64 consumers in 30 min, none lost, rss 53 MB at the mark (median of minutes 5 to 7), 51 MB at the end (median of the last 3 minutes; -4.0%), peak 62 MB, pass, 30m1s
- arm64 --check=all: amqptest stress: 1472094 messages by 64 producers, 1461241 accepted once and 10853 dead lettered by 64 consumers in 5 min, none lost, rss not sampled, pass; xo leakcheck: 0 live of 467569367 allocations, 5m1s
