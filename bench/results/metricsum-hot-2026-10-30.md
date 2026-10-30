# bench/perf/metricsum: a summary observation writes no collection under its Mutex, 2026-10-30

After the one pass compress (metricsum-compress-2026-10-29.md, 1.43x Go on
the Mac; 1.825x on the linux/arm64 offload host, 1,468 against 809 ms),
Xo Lang 1 read the cause of the remaining cost in the SSA: a Mutex.with
body keeps the value it started from until it returns (a fault hands it
back, spec core 5.1, decision 0085), so the body's first write to each
collection copies it. observe pushed into every age bucket's buffer, so
each observation copied the bucket list and up to 500 floats per bucket.
The summary now keeps client_golang's hot buffer: 500 Atomic[Int] slots
(Float bits) and an Atomic count in QState, written under the Mutex
without touching a collection; a full buffer, or a scrape, drains the
values into each bucket through the same qinsert calls in observation
order, so the quantiles are the same (golden metrics_summary and
metrics_text unchanged on arm64, LLVM and the Go backend; the metrics
corpus on all three). Memory: 500 Atomics per summary series, against the
up to 500 floats per bucket the buffers held before.

`xo bench --ab` on the linux/arm64 offload host, n=10:

| A | B | wall_ms A | wall_ms B | wall B/A (95%) | max_rss_kb A | max_rss_kb B |
|---|---|---|---|---|---|---|
| Xo arm64, parent (origin/main) | Xo arm64, this change | 1559 | 655.2 | 0.42 (0.418 to 0.423) | 6908 | 6908 |
| Go client_golang v1.23.2 | Xo arm64, this change | 857.5 | 657.2 | 0.766 (0.749 to 0.826) | 14760 | 6908 |

Host load medians 7.6 to 10.3 (no round at or above the loaded line).
2M observations and a scrape natively in 655 ms against 1,559 before on
linux/arm64 (0.42), 0.766 times Go's 858 ms (was 1.825), max RSS 6.9 MB
against Go's 14.8 MB.

Mac (one run): 0.50 s against about 1.0 s.
