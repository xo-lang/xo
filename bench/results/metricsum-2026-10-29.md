# bench/perf/metricsum: std/metrics Summary against client_golang, 2026-10-29

std/metrics' Summary (decision 0411, beorn7/perks' CKMS targeted
quantiles ported, client_golang's age buckets) against the Go twin on
prometheus/client_golang v1.23.2 (perks v1.0.1), Go 1.26.1: 2,000,000
observations of a scrambled sequence into one summary with objectives
0.5/0.05, 0.9/0.01, 0.99/0.001, then one scrape; both print
`metricsum 3 true`. `xo bench --ab`, 10 rounds each, measure slot on the
development Mac (arm64), xo of 3264eebbb's std, no round loaded.

| A | B | wall_ms A | wall_ms B | wall B/A (95%) | instructions B/A | max_rss_kb A | max_rss_kb B |
|---|---|---|---|---|---|---|---|
| Xo arm64 | Xo arm64 (control) | 1259 | 1260 | 1.001 (0.994 to 1.007) | 1.0 | 3440 | 3440 |
| Go client_golang | Xo arm64 | 728.0 | 1259 | 1.729 (1.725 to 1.737) | 1.805 | 12392 | 3440 |
| Go client_golang | Xo LLVM | 724.4 | 1376 | 1.9 (1.88 to 1.91) | 1.987 | 12376 | 3568 |

The port matches Go's answers but takes 1.73 times Go's time natively
(about 630 ns an observation against 364) at 0.28 of its memory. Where the
time goes (XO_PROF=cpu) is the next step: the candidates are the merge's
`List.insert` into the sample list (perks inserts into a slice the same
way, so a copy per insert is the same algorithm), the per-flush
`sort` of the 500-value buffer, and `qf`'s loop over the objectives per
sample in compress.
