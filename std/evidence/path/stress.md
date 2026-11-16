kind: stress
module: path
result: pass
commit: 52efc3d15
source: d98d36531b93880e3374d5e8b779068e1df0effc96136de85eb917178eba8218
date: 2026-11-16
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 2" path
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h5m25s

Stress evidence (decision 0616): the program std/evidence/path/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: path stress: 405233100096 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark, 5 MB at the end (1.0%), peak 5 MB, pass, 30m0s
- llvm: path stress: 309019874240 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark, 5 MB at the end (0.0%), peak 5 MB, pass, 30m0s
- arm64 --check=all: path stress: 63590237568 operations by 64 tasks in 5 min, 0 mismatches, rss 112 MB at the mark, 112 MB at the end (0.0%), peak 112 MB, pass; xo leakcheck: 0 live of 38659974498 allocations, 5m0s
