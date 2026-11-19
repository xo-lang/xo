kind: stress
module: testing
result: pass
commit: 03f2eaad1
source: 2f1df126f3e39aa5ed014752c9f3877759a031665d82d6477781e4b602add083
date: 2026-11-19
host: linux/arm64
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64" testing
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth from minutes 15 to 17 to the last 3 on 2 of 3 runs a backend (decision 0717), 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 2h4m57s

Stress evidence (decision 0616): the program std/evidence/testing/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64 run 1: testing stress: 400 rounds of every fake, then 519240 rounds of MemFs and FakeEnv (33231360 task rounds) by 64 tasks in 2011 s, 0 mismatches, memory level from half way, pass, 33m40s
- arm64 run 2: testing stress: 400 rounds of every fake, then 486900 rounds of MemFs and FakeEnv (31161600 task rounds) by 64 tasks in 1871 s, 0 mismatches, memory level from half way, pass, 31m21s
- llvm run 1: testing stress: 400 rounds of every fake, then 522780 rounds of MemFs and FakeEnv (33457920 task rounds) by 64 tasks in 1633 s, 0 mismatches, memory level from half way, pass, 27m21s
- llvm run 2: testing stress: 400 rounds of every fake, then 523830 rounds of MemFs and FakeEnv (33525120 task rounds) by 64 tasks in 1658 s, 0 mismatches, memory level from half way, pass, 27m46s
- arm64 --check=all: testing stress: 400 rounds of every fake, then 86310 rounds of MemFs and FakeEnv (5523840 task rounds) by 64 tasks in 271 s, 0 mismatches, memory level from half way, pass; xo leakcheck: 0 live of 127 allocations, 4m40s
