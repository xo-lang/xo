# metricsum

std/metrics' Summary (decision 0411, a port of beorn7/perks' CKMS
targeted quantiles with client_golang's age buckets): 2,000,000
observations of a scrambled sequence (7 is a primitive root mod 1,000,003)
into one summary with objectives 0.5/0.05, 0.9/0.01 and 0.99/0.001, then
one scrape. `go/` is the twin on prometheus/client_golang v1.23.2's
Summary with the same objectives, registry and text exposition.

Run: `xo build --backend=arm64 -o /tmp/xo-msum .`, `(cd go && go build -o
/tmp/go-msum .)`, then `xo bench --ab /tmp/go-msum /tmp/xo-msum`.
Both print `metricsum 3 true` (three quantile lines).
