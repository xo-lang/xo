# std/metrics changes

## 2026-10-21: feature: Registry.middleware, request counts and latencies for a router

- Kind: feature
- Commit: this commit
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
