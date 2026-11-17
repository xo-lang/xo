# std/metrics changes

## 2026-11-17: fix: a counter's whole adds wrap as an unsigned 64-bit count

- Kind: fix
- Commit: this push
- Found by: the metrics fuzz harness on the Mac (seed 7, registry 1130; task #261)
- Go reference: prometheus/client_golang counter.Add (valInt, a uint64)
- Test: std/metrics_test.xo "whole counter adds past 2^63 wrap as client_golang's uint64 valInt"; metricsfuzz seeds 3, 7 and 11 at 5000 registries
- Timing: none
- Shipped in: not yet released

The earlier fix today sent whole adds below 2^63 to the integer cell with a plain Int add, so 6e+17 then 9e+18 faulted as an integer overflow. The cell is now an unsigned 64-bit count that wraps, as Go's valInt, taking whole adds below 2^64, and reads back as an unsigned number.

## 2026-11-17: fix: a whole counter add up to 2^63 is exact

- Kind: fix (behavior change)
- Commit: this push
- Found by: the metrics fuzz harness on the Mac (seed 3, registry 2877; task #261)
- Go reference: prometheus/client_golang counter.Add (valInt for a whole value, valBits otherwise)
- Test: std/metrics_test.xo "a whole counter add up to 2^63 is exact"
- Timing: none
- Shipped in: not yet released

A whole add from 2^53 up went to the counter's float sum, so fractions added after a 9e+17 rounded differently from client_golang (9.000000000000015e+17 for Go's 9.000000000000017e+17). Whole adds below 2^63 now go to the integer cell, as Go's valInt; Go's integer path goes on to 2^64 (unsigned), which Xo's Int does not reach.

## 2026-11-17: fix: values print as client_golang's text format

- Kind: fix (behavior change)
- Commit: this push
- Found by: the new metrics fuzz harness (metricsfuzz, task #261), against prometheus/client_golang v1.23.2's text exposition
- Go reference: prometheus/common expfmt writeFloat (NaN, +Inf, -Inf, 0, else strconv 'g' -1)
- Test: std/metrics_test.xo "values print as client_golang's text format"; corpus metricsfuzz (60 registries, arm64, llvm and go); TestMetricsFuzz (XO_METRICSFUZZ_SEED) for fresh seeds
- Timing: none
- Shipped in: not yet released

Whole numbers printed in full (800000000 where client_golang writes 8e+08), and every finite value past 1e300 printed as +Inf. Values now print as Go's shortest 'g' (1e+06, 123456, 1.2e-05), any zero (-0 too) as 0, as client_golang's text format; OpenMetrics follows (its ".0" rule unchanged).

## 2026-11-16: feature: instrument_round_tripper_trace and InstrumentTrace (decision 0677)

- Kind: feature
- Commit: this commit
- Found by: task #55, the 0655 audit's open map.txt rows promhttp.InstrumentRoundTripperTrace and promhttp.InstrumentTrace
- Go reference: client_golang v1.23.2 prometheus/promhttp instrument_client.go (InstrumentTrace, InstrumentRoundTripperTrace)
- Test: golden metrics_client_trace (fresh and reused connections, an observer left None; arm64 and LLVM equal to the go backend)
- Shipped in: not yet released

`metrics.instrument_round_tripper_trace(it, clock)` sends a request traced
with std/http's `with_trace` and gives each event the `InstrumentTrace`
observes the seconds since the send started, as promhttp's observers do.
The observers run when the send returns, not during the exchange, and Go's
Wait100Continue has no event.

## 2026-11-16: perf: labelled inc, add, set and observe keep the caller's label list on the stack

- Kind: perf
- Commit: this push
- Found by: task #87 (std/evidence/metrics/perf.md, counter 1.134 of Go on host 1), XO_PROF=cpu on bench/perf/metrics counter: obj.rl_unstack_n under label_values, 18%
- Go reference: prometheus/client_golang v1.23.2 (WithLabelValues takes a variadic slice the caller's frame holds)
- Test: std/metrics_test.xo (unchanged; the same values on every path); bench/perf/metrics instructions on the Mac: counter 2.607G to 1.967G (0.754), histogram 1.853G to 1.428G (0.770), counterheld and scrape unchanged
- Shipped in: not yet released

`Counter.inc` and `add`, `Gauge.set` and `add`, `Histogram.observe` and `Summary.observe` with nothing curried or constrained take the label values as given: label_values returned its argument, which made the caller's `[...]` literal owned and moved it to the heap on every call.

## 2026-11-15: feature: protobuf exposition and native histograms

- Kind: feature
- Commit: this push
- Found by: the stdaudit gaps of prometheus/client_golang (decision 0655)
- Go reference: prometheus/client_golang v1.23.2 histogram.go (NewConstNativeHistogram, validateCount, makeBucketsFromMap, DefNativeHistogramZeroThreshold, NativeHistogramZeroThresholdZero); prometheus/client_model v0.6.3 metrics.proto; prometheus/common expfmt (FmtProtoDelim)
- Test: std/metrics_test.xo (native_histogram's checks, makeBucketsFromMap's spans and deltas, TestConstNativeHistogram's no-op span, the exposition read back, the handler's negotiation)
- Shipped in: not yet released

`Registry.pid_collector(namespace)` with `update(fs, pid)` exports another process's CPU time, resident and virtual memory and start time from /proc (client_golang's process collector with PidFn; Linux only, an error elsewhere), and `pid_from_file` reads a pid file (NewPidFileFn); test: the procfs parsing cases and this process read back on Linux.

`Registry.protobuf(now)` writes the delimited protobuf exposition, which the handlers serve when a scraper lists it first in Accept (Prometheus does with native histograms on). A histogram collector's `Sample.native` (from `native_histogram`) is written as a native histogram. Repeated numbers are packed where Go writes proto2's unpacked form; Prometheus reads both.

## 2026-11-15: feature: OpenMetrics exposition and exemplars

- Kind: feature
- Commit: this push
- Found by: the stdaudit gaps of prometheus/client_golang (decision 0655)
- Go reference: prometheus/common v0.73.0 expfmt/openmetrics_create.go (MetricFamilyToOpenMetrics, writeOpenMetricsFloat, writeExemplar); prometheus/client_golang v1.23.2 value.go (newExemplar, ExemplarMaxRunes), counter.go and histogram.go (AddWithExemplar, ObserveWithExemplar), promhttp (EnableOpenMetrics, WithExemplarFromContext)
- Test: std/metrics_test.xo (openmetrics_create_test.go's cases 9, 10, 11 and 13 ported; instrument exemplars, their checks, removal, the handler's negotiation and the middleware's exemplar_of)
- Shipped in: not yet released

`Registry.open_metrics(now)` writes the OpenMetrics 1.0 text, which `handler(open_metrics: true)` serves to scrapers that ask for it. Counters and histograms keep the newest exemplar per series (per bucket) from `add_with_exemplar` and `observe_with_exemplar`; collectors give `Sample.exemplars`; the middleware takes `exemplar_of` for a trace id. Exemplar labels are written sorted by name (Go writes them in map order). `process_collector(clock, namespace)` registers process CPU time, peak memory and start time, read by its `update(proc)` (a scrape cannot use `proc`; Go reads at the scrape and reports the current resident memory).

## 2026-11-15: feature: compressed scrapes, size metrics and client instrumentation

- Kind: feature
- Commit: this push
- Found by: the stdaudit gaps of prometheus/client_golang's promhttp (decision 0655)
- Go reference: prometheus/client_golang v1.23.2 promhttp/http.go (negotiateEncodingWriter, Compression), instrument_server.go (InstrumentHandlerRequestSize, InstrumentHandlerResponseSize, computeApproximateRequestSize), instrument_client.go (InstrumentRoundTripperCounter, Duration, InFlight)
- Test: std/metrics_test.xo (Accept-Encoding negotiation cases, request and response sizes, a failed client send counted as `error`)
- Shipped in: not yet released

`handler` and `handler_at` gzip the body when the request accepts gzip and it is offered (the default offers gzip and identity, as promhttp's). `middleware(clock, label, sizes: true)` adds request and response size histograms. `client_send(clock)` counts, times and tracks in flight an http.Client's sends. A scraper that sends `Accept-Encoding: gzip` now gets a compressed body; pass `offer: [Compression.Identity]` for the old behavior. `runtime_collector()` (NewGoCollector, for Xo's heap) and `build_info(path, version, checksum)` (NewBuildInfoCollector, the module passed by main) register gauges.

## 2026-11-15: feature: gather several registries

- Kind: feature
- Commit: this push
- Found by: the stdaudit gaps of prometheus/client_golang (decision 0655)
- Go reference: prometheus/client_golang v1.23.2 registry.go (Gatherers.Gather, MultiError) and internal/metric.go (MetricSorter)
- Test: std/metrics_test.xo (examples_test.go's ExampleGatherers, both parts, ported; help and type mismatches and MultiError's text)
- Shipped in: not yet released

`metrics.gather(registries, now)` returns one exposition of several registries and a `MultiError`: families of one name are merged and sorted as Go's Gather sorts them; a family with another help or type, or a series given twice, is left out with Go's error text.

## 2026-11-15: feature: histogram and summary collectors, sample timestamps and created times

- Kind: feature
- Commit: this push
- Found by: the stdaudit gaps of prometheus/client_golang (decision 0655)
- Go reference: prometheus/client_golang v1.23.2 histogram.go (NewConstHistogram, NewConstHistogramWithCreatedTimestamp), summary.go (NewConstSummary, NewConstSummaryWithCreatedTimestamp), value.go (NewConstMetricWithCreatedTimestamp), metric.go (NewMetricWithTimestamp)
- Test: std/metrics_test.xo (TestNewConstHistogramWithCreatedTimestamp's and TestNewConstSummaryWithCreatedTimestamp's samples in the text, timestamps); std/native/otlp_test.xo (created and timestamp as the OTLP point times, bucket counts, quantiles, a gauge's created ignored)
- Shipped in: not yet released

A collector of `Kind.Histogram` or `Kind.Summary` exposes each Sample's `buckets` or `quantiles` with its `sum` and `count`. `Sample.timestamp` follows each value in the text; `Sample.created` is OTLP's start time. Go refuses a created time on a gauge; Xo ignores it (a gauge has no start). `Kind` has two more variants: a `match` with no `_` arm adds them.

## 2026-11-15: feature: wrapped registries and untyped series

- Kind: feature
- Commit: this push
- Found by: the stdaudit gaps of prometheus/client_golang (decision 0655)
- Go reference: prometheus/client_golang v1.23.2 wrap.go (WrapRegistererWith, WrapRegistererWithPrefix, WrapCollectorWith) and value.go/untyped.go (UntypedValue, UntypedFunc)
- Test: std/metrics_test.xo (wrap with nested prefixes and labels, untyped_func and an untyped collector); std/native/otlp_test.xo (an untyped series is a gauge; constant labels are attributes)
- Shipped in: not yet released

`Registry.wrap(prefix, labels)` returns the registry with a name prefix and constant labels for every instrument made through it; the labels come first in each exposition line, sorted by name, and are OTLP attributes. `untyped_func` and `Kind.Untyped` expose `# TYPE name untyped` series, exported to OTLP as gauges. `Kind` has a third variant: a `match` on it with no `_` arm adds `Kind.Untyped`.

## 2026-11-15: feature: label constraints

- Kind: feature
- Commit: this push
- Found by: the stdaudit gaps of prometheus/client_golang (decision 0655)
- Go reference: prometheus/client_golang v1.23.2 labels.go (ConstrainedLabels, LabelConstraint) and vec.go (constrainLabelValues, constrainLabels)
- Test: std/metrics_test.xo (vec_test.go's TestDeleteWithConstraints, TestDeleteLabelValuesWithConstraints, TestDeletePartialMatchWithConstraints, TestMetricVecWithConstraints and TestCurryVecWithConstraints, ported)
- Shipped in: not yet released

`counter`, `gauge`, `histogram` and `summary` take `constraints: Map[Str, fn(Str) -> Str]`: each named label's values go through its function in every call (inc, set, observe, child, remove, curry_with, delete_partial_match), as client_golang's ConstrainedLabels.

## 2026-11-15: feature: curried instruments, partial deletes and bucket helpers

- Kind: feature
- Commit: this push
- Found by: the stdaudit gaps of prometheus/client_golang (decision 0655)
- Go reference: prometheus/client_golang v1.23.2 vec.go (CurryWith, DeletePartialMatch) and histogram.go (LinearBuckets, ExponentialBuckets, ExponentialBucketsRange)
- Test: std/metrics_test.xo (vec_test.go's testCurryVec and testDeletePartialMatch, histogram_test.go's TestBuckets, ported)
- Shipped in: not yet released

`curry_with(labels)` on Counter, Gauge, Histogram and Summary returns the instrument with those label values fixed, sharing its series; `must_curry_with` faults on an error. `delete_partial_match(labels)` removes every series holding all the pairs and returns how many. `linear_buckets`, `exponential_buckets` and `exponential_buckets_range` build histogram bounds.

## 2026-11-15: behavior: scrape time readers may use shared state

- Kind: behavior
- Commit: this push
- Found by: decision 0662 (a closure that captures shared state performs `sync`)
- Go reference: none
- Test: test/errorcheck/effect_sync_capture; the module's own tests check clean
- Shipped in: not yet released

`gauge_func`, `counter_func` and `collector` take `fn() -> Float uses sync` (`fn() -> List[Sample] uses sync`): a reader is called at scrape time from another task and usually reads an Atomic or a Mutex, which a pure function type now refuses (decision 0662). `handler()`, `handler_at(clock)` and `middleware(clock, label)` return types that allow `sync` too (`uses sync`, `uses (clock, sync)`): the closures read the registry's Mutex. Callers change nothing; a program that passed or stored a variable of the old type adds `sync` to it.

## 2026-11-15: perf: a labelled hit reads its series in place

- Kind: perf
- Commit: this commit
- Found by: Lang 3 and Lang 1, the std Proven perf line (bench/perf/metrics counter 1.640 and histogram 1.127 of client_golang on linux/arm64 host 1); a linux CPU profile put the atomic count updates of the Series a Map.get copies at about a quarter of Counter.inc
- Go reference: prometheus/client_golang vec.go (metricMap.getMetricWithHashAndLabelValues: a lookup that returns a pointer)
- Test: the metrics and otlp corpora and std/native/otlp_test.xo (go, arm64, llvm); xo bench --ab on linux/arm64 host 1, 3 rounds: counter 114.5 to 77.0 ms (0.673, about 1.05 of Go), histogram 84.2 to 65.6 ms (0.780, about 0.85 of Go)
- Affected: none (no behavior change)
- Shipped in: not yet released

An instrument's series were a Map[Str, Series], so each labelled inc, add or observe copied the Series out of the map: its three shared lists counted up and down, six atomic updates. The series now sit in a list with a map from key to index, and a hit reads the series' cells in place. Removing a series moves the last one into its index; a scrape and std/otlp read a snapshot as before.

## 2026-11-15: fix: an instrument with no series writes nothing, one without labels writes its zeros

- Kind: fix
- Commit: this commit
- Found by: Lang 1, bench/perf/metrics `scrape` against client_golang (52 bytes a scrape)
- Go reference: prometheus/client_golang registry.go Gather and internal.NormalizeMetricFamilies (a family with no metrics is dropped); NewCounter and NewHistogram export one metric from the start
- Test: golden metrics_empty (go, arm64, llvm); the six metrics goldens unchanged

`text()` wrote `# HELP` and `# TYPE` for a labelled instrument no child of
which was made (or whose children were all removed) and for a collector that
gave no samples; client_golang writes nothing for them. It also wrote only
the two comment lines for an instrument without labels before its first
update, where Go writes its zero values (`boot_total 0`, a histogram's zero
buckets, sum and count). Both now match Go.

## 2026-11-11: refactor: std/metrics is one file (decisions 0588, 0597)

- Kind: refactor
- Commit: this commit
- Found by: decision 0597's merges
- Go reference: none (no behavior change)
- Test: TestStdCorpora metrics and otlp on go, arm64, llvm; xo test --std-module=std/metrics (arm64, llvm); TestStdChecks
- Shipped in: not yet released

std/native/metrics.xo moved into std/metrics.xo. It is generated into std/rt (std/rt_hosted.txt), since std/otlp, which is, uses it.

## 2026-11-08: perf: a labelled hit updates under the read share; counters add whole numbers with one atomic add

- Kind: perf
- Commit: this push
- Found by: counting the atomic updates of a labelled Counter.inc hit (Xo Lang 1, path 1 of the lead's queue): five (the share's two, the cells list's share and release, the float's compare and swap) against client_golang's three
- Go reference: prometheus/client_golang's counter (valInt, an atomic add for a whole increment, beside valBits) and its metricVec lookup under RLock
- Test: goldens metrics_buckets, metrics_child, metrics_summary, metrics_text, otlp_export (arm64, LLVM, Go parity); std corpus metrics (go, arm64, llvm)
- Timing: --ab on linux/arm64 host 1 against main without it, 7 rounds: bench/perf/metrics counter 0.972 (0.935 to 0.996), histogram 0.960 (0.942 to 0.987, n=6), counterheld within noise (0.978)
- Shipped in: not yet released

A hit (the series exists) now adds inside with_read's closure, reading
the cells in place, so it no longer shares and releases the cells list
(two atomic updates a call); a histogram finds its bucket before the
share, so the closure captures no list of the Family (capturing the
bounds made the Family owned: 1.76 times slower in a first try). A
counter's whole increments below 2^53 (inc, and add of a whole number)
go to cell 1, unused by a counter before, with one atomic add instead of
the float's load and compare and swap loop; the text exposition and the
OTLP exporter read the counter as cell 0's float plus cell 1.

## 2026-11-07: perf: a labelled update shares only the series' cells

- Kind: perf
- Commit: this push
- Found by: XO_PROF=cpu of bench/perf/metrics counter on linux/arm64 (the drop of the looked up Series was 16% of samples)
- Go reference: prometheus/client_golang's metricVec lookup (the found metric is used in place)
- Test: goldens metrics_buckets, metrics_child, metrics_summary, metrics_text (arm64, LLVM); std corpus metrics (go, arm64, llvm)
- Timing: --ab on linux/arm64 against main without it: bench/perf/metrics counter 0.900 (0.892 to 0.923, n=9), histogram 0.866 (0.855 to 0.873, n=6), counterheld within noise
- Shipped in: not yet released

A labelled inc, set, add or observe took the whole Series out of the map
on every call, sharing and then releasing its three lists (atomic counts:
the series is shared between threads). A hit now takes only the cells
list (cells_of), one count instead of three.

## 2026-11-07: perf: a labelled update no longer counts the Family's fields

- Kind: perf
- Commit: this push
- Found by: bench/results/xo-vs-go-survey-2026-11-07.md (metrics/counter 4.25 and histogram 2.72 times client_golang)
- Go reference: prometheus/client_golang's metricVec lookup (no per call cost on the family)
- Test: goldens metrics_buckets, metrics_child, metrics_summary; std corpus metrics
- Timing: --ab on linux/arm64 against main without it: bench/perf/metrics counter 0.781, histogram 0.667, counterheld within noise
- Shipped in: not yet released

The miss path's closure captured the Family's bounds List, which made the
Family an owned parameter of every lookup: each inc, set, add and observe
retained and released every heap field of it (atomic counts, since the
Family is shared). It captures the bounds' count instead (gotchas/captured-field-owns-struct-param.md).

## 2026-11-04: feature: child handles (decision 0501)

- Kind: feature
- Commit: this push
- Found by: bench/perf/metrics (a labelled increment is 3.7 times client_golang's: the lookup per call is the cost)
- Go reference: prometheus/client_golang's WithLabelValues (a held Counter or Observer)
- Test: golden metrics_child (every backend and the Go backend), bench/perf/metrics counterheld and histogramheld
- Timing: bench/results/std-twin-metrics-2026-11-04.md
- Shipped in: not yet released

`Counter`, `Gauge`, `Histogram` and `Summary` have `child(values)`, a held
series: `CounterChild.inc/add`, `GaugeChild.set/add`, `HistogramChild.observe`
and `SummaryChild.observe` take no label list, key or lookup. `remove` and
`reset` detach a series; a child made before keeps counting into it.


## 2026-11-04: perf: a Series holds its cells in one list

- Kind: perf
- Commit: this push
- Found by: bench/perf/metrics (the profile of the labelled increment: six reference counted fields shared and dropped on every hit)
- Go reference: none (client_golang's children hold their atomics directly)
- Test: goldens metrics_text, metrics_summary, metrics_buckets (every backend and the Go backend), TestVetCostStd, stdgen
- Timing: bench/results/std-twin-metrics-2026-11-04.md (host 1): counter 4.07 to 3.67, histogram 3.26 to 2.68 of client_golang; Xo's own time 12% and 16% lower
- Shipped in: not yet released

metrics.Series is three fields (values, cells, q): cell 0 the value, 1 the
count, 2 the sum, 3 and on the buckets. The type is internal; nothing that
uses the instruments changes.


## 2026-11-04: perf: a one label series key without a copy, one bucket a histogram observation, a closure without the Family

- Kind: perf
- Commit: this push
- Found by: the new twin bench/perf/metrics against client_golang (counter 4.38, histogram 3.51 of Go; scrape 0.475)
- Go reference: client_golang's histogram.findBucket (one bucket counted, the scrape adds up), CounterVec.WithLabelValues
- Test: goldens metrics_text, metrics_summary and the new metrics_buckets (a value below the first bound, on a bound, between, on the last bound, above; every backend and the Go backend); TestVetCostStd
- Timing: bench/results/std-twin-metrics-2026-11-04.md (host 1): counter 4.378 to 4.070, histogram 3.508 to 3.256 of Go; Xo's own time 9% and 6% lower
- Shipped in: not yet released

series_of keys a one label series by the value itself, the histogram's
cells count one bucket each (the text exposition is unchanged: the scrape
adds them up), and the closure that makes a series captures the three
fields it reads, not the Family.


## 2026-10-30: perf: a summary observation writes no collection under its Mutex

- Kind: perf
- Commit: this commit
- Found by: Xo Lang 1's SSA reading of the qp repro (a Mutex.with body's
  first write to each collection copies it while the mutex still holds
  the value for a fault, spec core 5.1; observe pushed into every age
  bucket's buffer, so each observation copied up to 500 floats per bucket)
- Go reference: client_golang summary's hotBuf (observations buffered,
  then moved into the streams in a batch)
- Test: goldens metrics_summary and metrics_text (arm64, LLVM; Go parity
  for metrics_summary), the metrics corpus on arm64, LLVM and Go;
  xo bench --ab on linux/arm64 in bench/results/metricsum-hot-2026-10-30.md

2M observations and a scrape natively in 655 ms against 1,559 before on the linux/arm64 offload host (wall 0.42, max RSS unchanged at 6.9 MB), 0.766 times Go's 858 ms (was 1.825).

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
