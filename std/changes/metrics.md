# std/metrics changes

## 2026-10-29: perf: a summary's quantile stream compresses in one pass

- Kind: perf
- Commit: this commit
- Found by: the metricsum profile after the flush fix (compress's removal
  per fold)
- Go reference: beorn7/perks quantile stream.compress (the same samples, the
  kept ones written once instead of a removal per fold)
- Test: golden metrics_summary and metrics_text (arm64, LLVM); xo bench
  --ab in bench/results/metricsum-compress-2026-10-29.md

2M observations and a scrape natively in 1032 ms against 1147 before (wall 0.9, instructions 0.904, max RSS 3.1 MB against 3.3), 1.43 times Go's 722 ms (was 1.59); LLVM 1.70 (was 1.81).

## 2026-10-29: perf: a summary's quantile stream flushes without copying its lists

- Kind: perf
- Commit: this commit
- Found by: XO_PROF=cpu of bench/perf/metricsum (half the samples in
  platform_memmove; bench/results/metricsum-2026-10-29.md, 1.73x Go)
- Go reference: beorn7/perks quantile stream.merge and compress (the same
  samples; the merge in one pass instead of an insert per value)
- Test: golden metrics_summary and metrics_text (arm64, LLVM); xo bench
  --ab in bench/results/metricsum-qflush-2026-10-29.md

2M observations and a scrape natively in 1148 ms against 1253 before (wall 0.916, instructions 0.909, max RSS 3.3 MB against 3.4), 1.59 times Go's 722 ms (was 1.73); LLVM 1.81 (was 1.9).

## 2026-10-29: feature: summaries, scrape-time readers, removing series

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-29 std survey (std-coverage.md rank 10): an
  exporter that reads its state at scrape time
- Go reference: prometheus/client_golang Summary (Objectives, MaxAge,
  AgeBuckets), GaugeFunc, CounterFunc, Collector, DeleteLabelValues,
  Reset; beorn7/perks quantile stream; decision 0411
- Test: internal/llvm/testdata/golden/metrics_summary (native and Go)

`r.summary(...)` reports quantile objectives, `_sum` and `_count`, aged over
`max_age` by a scrape's time (`text(now)`, `handler_at(clock)`).
`gauge_func`, `counter_func` and `collector` read values at each scrape;
`remove` and `reset` delete series.

## 2026-10-26: fix: `histogram` without buckets builds on the Go backend

- Kind: fix
- Commit: this commit
- Found by: DS-2 (the metrics corpus: `r.histogram("x", "y")` with `--backend=go`)
- Go reference: none (the Go backend's lowering of std default values)
- Test: golden std_default_call on arm64, LLVM and the Go backend (TestGoldenGoParity)

`buckets: List[Float] = latency_buckets()` was "unsupported std default
value" with `--backend=go`, so a program that called `histogram` without
buckets did not build there; arm64 and LLVM built it. The Go backend now
lowers any default value of a std declaration (a call, a non-empty list,
map or set, an interpolated string), evaluated at each call as on the
native backends; before, a non-empty list default would have become an
empty one.

## 2026-10-21: feature: Registry.middleware, request counts and latencies for a router

- Kind: feature
- Commit: b9b44a29f
- Found by: decision 0325 (a server's per route counts and latencies with no handler code)
- Go reference: none (the Prometheus client's promhttp middleware is the shape)
- Test: internal/llvm/testdata/golden/metrics_text (the middleware counts by method, route and status, times, and leaves in flight at 0)

`router.wrap(reg.middleware(clock, label))` counts every request in
`http_requests_total{method,route,status}`, times it in
`http_request_seconds{route}` and tracks `http_in_flight`.

## 2026-10-21: feature: std/metrics, counters, gauges, histograms and the Prometheus text exposition

- Kind: feature
- Commit: 81c91200c
- Found by: decision 0325 (the owner's language poll)
- Go reference: none in the standard library; the format is Prometheus text exposition 0.0.4 (the hand written corpus, decision 0312's addendum)
- Test: internal/llvm/testdata/golden/metrics_text (labels, escaping, sorted series, cumulative buckets, 2,000 concurrent increments, the 10,000 series limit, the handler; arm64, LLVM, Go parity)

`metrics.registry()` makes a registry; counters, gauges and histograms
take label values and are safe from any task; `text()` and `handler()`
write the exposition. One source for every backend (std/go_compiled.txt).
