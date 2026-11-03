# Timing rows against Go, part 4 (2026-11-03)

Host 1, `xo bench --ab --rounds=15`, bench/perf/walkdir at the change set that returns clean fs paths unchanged
(fs_clean_slash fast path) and adds a `walk_info` workload (Go also calls `d.Info()` on each entry, which is the
work Xo's FileInfo has done already).

| bench | workload | Go wall (ms) | Xo wall (ms) | Xo / Go | interval | peak memory |
|---|---|---|---|---|---|---|
| walkdir | none | 37.95 | 36.82 | 0.97 | 0.927 to 0.996 | 1.0 |
| walkdir | walk | 298.5 | 459.1 | 1.538 | 1.517 to 1.56 | 0.558 |
| walkdir | walk_info | 476.7 | 454.1 | 0.952 | 0.937 to 0.962 | 0.561 |

walk was 1.92 before. The profile before the change (host 1, walk): statx 36%, linux.names 20% (getdents), fs_clean_slash 20%.
dns mx (4.66): the profile has 3 samples in 30 ms of CPU, so the time is waiting: a UDP round trip between the program's
resolver and its server task, i.e. scheduler wakeup latency, not std code; measured with `--ab` only and not changed.
