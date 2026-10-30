# bench/perf/metricsum: the quantile stream's flush without copies, 2026-10-29

After bench/results/metricsum-2026-10-29.md (1.73x Go), XO_PROF=cpu put half
the samples in platform_memmove: qflush read the sample list into a local
while the stream still held it, so the first insert copied the whole list
every flush (gotchas/element-copy-store-back-quadratic.md), and the
500-value buffer was reset empty and regrew each flush. qflush now takes
both lists out of the stream, presizes the buffer, and merges the sorted
buffer into the samples in one pass (perks inserts into the slice one value
at a time; the samples and their order are the same, golden
metrics_summary unchanged on arm64 and LLVM).

`xo bench --ab`, measure slot on the development Mac (arm64); A built with
origin/main's std/native/metrics.xo, B with this change, the same xo:

| A | B | rounds | wall_ms A | wall_ms B | wall B/A (95%) | instructions B/A | max_rss_kb A | max_rss_kb B |
|---|---|---|---|---|---|---|---|---|
| Xo arm64, main | Xo arm64, main (control) | 10 | 1264 | 1260 | 0.997 (0.979 to 1.005) | 1.0 | 3440 | 3440 |
| Xo arm64, main | Xo arm64, this change | 12 | 1253 | 1148 | 0.916 (0.911 to 0.92) | 0.909 | 3440 | 3296 |
| Go client_golang | Xo arm64, this change | 10 | 722.2 | 1148 | 1.589 (1.584 to 1.595) | 1.64 | 12376 | 3296 |
| Go client_golang | Xo LLVM, this change | 10 | 719.2 | 1303 | 1.811 (1.802 to 1.816) | 1.845 | 12392 | 3408 |

2M observations and a scrape natively in 1148 ms against 1253 before (wall 0.916, instructions 0.909, max RSS 3.3 MB against 3.4), 1.59 times Go's 722 ms (was 1.73); LLVM 1.81 (was 1.9).
