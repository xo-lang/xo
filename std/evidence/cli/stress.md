kind: stress
module: cli
result: pass
commit: 224654253
source: 06436b6976603b015b6652c833d7421c05016815a644e9e402eabcc9cad840d0
date: 2026-11-17
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 2" cli
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 30m39s

Stress evidence (decision 0616): the program std/evidence/cli/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: cli stress: 6593176512 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark, 6 MB at the end (23.0%), peak 6 MB, pass, 30m0s
- llvm: cli stress: 7404025152 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark, 7 MB at the end (22.0%), peak 7 MB, pass, 30m0s
- arm64 --check=all: cli stress: 243333248 operations by 64 tasks in 5 min, 0 mismatches, rss 305 MB at the mark, 305 MB at the end (0.0%), peak 305 MB, pass; xo leakcheck: 0 live of 7277185469 allocations, 5m0s
