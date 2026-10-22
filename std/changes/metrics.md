# std/metrics changes

## 2026-10-21: feature: std/metrics, counters, gauges, histograms and the Prometheus text exposition

- Kind: feature
- Commit: (filled in after the push)
- Found by: decision 0325 (the owner's language poll)
- Go reference: none in the standard library; the format is Prometheus text exposition 0.0.4 (the hand written corpus, decision 0312's addendum)
- Test: internal/llvm/testdata/golden/metrics_text (labels, escaping, sorted series, cumulative buckets, 2,000 concurrent increments, the 10,000 series limit, the handler; arm64, LLVM, Go parity)

`metrics.registry()` makes a registry; counters, gauges and histograms
take label values and are safe from any task; `text()` and `handler()`
write the exposition. One source for every backend (std/go_compiled.txt).
