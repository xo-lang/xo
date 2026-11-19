# bench/perf/gomath results

`go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" go`
(gomath's twins; bench/perf/gomath is not yet in prelude's perf set),
2026-11-19, commit fc0295fb9 plus this twin, xo's default backend.
Medians of wall time in ms, the two programs alternating. erf, erfc, exp
and pow are judged against Go's portable code; package math's time is the
last column (decision 0721).

| workload | Go ms | Xo ms | Xo / Go | Go package math ms |
|---|---|---|---|---|
| erf | 47.7 | 46.3 | 0.970 | 36.2 |
| erfc | 55.9 | 54.3 | 0.971 | 40.6 |
| erfinv | 17.6 | 17.4 | 0.990 | |
| erfcinv | 18.0 | 17.5 | 0.969 | |
| gamma | 83.4 | 94.2 | 1.130 | |
| lgamma | 27.0 | 28.4 | 1.054 | |
| next_after | 6.8 | 6.7 | 0.987 | |
| remainder | 150.9 | 209.8 | 1.391 | |
| acosh | 24.9 | 20.9 | 0.839 | |
| exp | 21.6 | 15.9 | 0.735 | 14.7 |
| pow | 79.4 | 90.6 | 1.142 | 69.3 |

Over 1.05: gamma (task #406), pow (task #406, against the portable copy),
lgamma and remainder (Lang 1, task #407). Add `"gomath": "prelude"` to
scripts/stdproven's perfOwners when every row is at or under 1.05.
