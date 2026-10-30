# bench/perf/metricsum: the quantile stream's compress in one pass, 2026-10-29

After the flush without copies (metricsum-qflush-2026-10-29.md, 1.59x Go),
compress folded samples by removing each from the list (perks removes from its
slice the same way: a copy of the tail per fold). It now writes the kept samples
once, from the end of a list of the same length; the samples and their order
are the same (golden metrics_summary unchanged on arm64 and LLVM).

`xo bench --ab`, measure slot on the development Mac (arm64); A built with
origin/main's std/native/metrics.xo, B with this change, the same xo:

| A | B | rounds | wall_ms A | wall_ms B | wall B/A (95%) | instructions B/A | max_rss_kb A | max_rss_kb B |
|---|---|---|---|---|---|---|---|---|
| Xo arm64, main | Xo arm64, main (control) | 10 | 1148 | 1145 | 0.998 (0.993 to 1.002) | 1.0 | 3296 | 3296 |
| Xo arm64, main | Xo arm64, this change | 12 | 1147 | 1032 | 0.9 (0.896 to 0.906) | 0.904 | 3296 | 3072 |
| Go client_golang | Xo arm64, this change | 10 | 722.5 | 1031 | 1.426 (1.421 to 1.438) | 1.482 | 12336 | 3072 |
| Go client_golang | Xo LLVM, this change | 10 | 724.9 | 1229 | 1.696 (1.687 to 1.714) | 1.692 | 12408 | 3200 |

2M observations and a scrape natively in 1032 ms against 1147 before (wall 0.9, instructions 0.904, max RSS 3.1 MB against 3.3), 1.43 times Go's 722 ms (was 1.59); LLVM 1.70 (was 1.81).
