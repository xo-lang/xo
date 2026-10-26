# std/metrics changes

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
