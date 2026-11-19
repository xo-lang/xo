# bench/perf/gomath results

`go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" prelude`
(bench/perf/gomath counts for prelude since edb5a7dd0), 2026-11-19, commit
4b26f52b3 (after 1b586b512's exact fmod and 4b26f52b3's trunc), xo's
default backend.
Medians of wall time in ms, the two programs alternating. erf, erfc, exp
and pow are judged against Go's portable code; package math's time is the
last column (decision 0721).

| workload | Go ms | Xo ms | Xo / Go | Go package math ms |
|---|---|---|---|---|
| erf | 48.4 | 46.4 | 0.958 | 36.1 |
| erfc | 55.6 | 54.4 | 0.979 | 40.9 |
| erfinv | 17.5 | 17.5 | 1.001 | |
| erfcinv | 18.2 | 17.4 | 0.955 | |
| gamma | 83.1 | 84.0 | 1.010 | |
| lgamma | 26.8 | 26.8 | 0.998 | |
| next_after | 6.8 | 6.4 | 0.942 | |
| remainder | 151.7 | 102.9 | 0.678 | |
| acosh | 25.1 | 21.0 | 0.836 | |
| exp | 20.8 | 15.8 | 0.762 | 14.4 |
| pow | 79.4 | 83.9 | 1.057 | 69.7 |

Over 1.05: pow only (task #406, against the portable copy), which keeps
prelude's perf at fail. Tried and dropped: wrapping_add and wrapping_sub
for pow's loop counters (slower, gotchas/wrapping-add-slower-than-checked.md).
