kind: stress
module: go
result: pass
commit: 99d5a8117
source: 20ba3b9d9243d8ac8566f8ec0af9a8291d6eacd35b73ddc158a88cc18b5e7b66
date: 2026-11-19
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends go -check-minutes 0 -host "linux/arm64 host 2" go
measure: 64 tasks for 30 min on each of go, 0 mismatches, 0 faults, memory under 10% growth from minutes 15 to 17 to the last 3 on 2 of 3 runs a backend (decision 0717), 1h0m8s

Stress evidence (decision 0616): the program std/evidence/go/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- go run 1: go stress: 11274459360 checks by 64 tasks in 30 min, 0 mismatches, rss 44 MB at the mark (median of minutes 15 to 17), 46 MB at the end (median of the last 3 minutes; 6.0%), peak 149 MB, pass, 30m0s
- go run 2: go stress: 11339494440 checks by 64 tasks in 30 min, 0 mismatches, rss 45 MB at the mark (median of minutes 15 to 17), 46 MB at the end (median of the last 3 minutes; 4.0%), peak 190 MB, pass, 30m0s
