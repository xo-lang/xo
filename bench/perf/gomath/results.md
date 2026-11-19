# bench/perf/gomath results

`go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" prelude`
(bench/perf/gomath counts for prelude since edb5a7dd0), 2026-11-19, commit
80d5dfdff (after 1b586b512's exact fmod, 4b26f52b3's trunc and 72abe2abd's
frexp and ldexp bit paths), xo's default backend.
Medians of wall time in ms, the two programs alternating. erf, erfc, exp
and pow are judged against Go's portable code; package math's time is the
last column (decision 0721).

| workload | Go ms | Xo ms | Xo / Go | Go package math ms |
|---|---|---|---|---|
| erf | 47.8 | 46.4 | 0.969 | 36.0 |
| erfc | 55.3 | 54.2 | 0.980 | 40.5 |
| erfinv | 17.4 | 17.7 | 1.019 | |
| erfcinv | 18.2 | 17.3 | 0.950 | |
| gamma | 83.3 | 79.1 | 0.950 | |
| lgamma | 26.9 | 26.7 | 0.992 | |
| next_after | 6.6 | 6.4 | 0.978 | |
| remainder | 151.0 | 103.6 | 0.686 | |
| acosh | 24.5 | 21.0 | 0.857 | |
| exp | 21.0 | 16.1 | 0.766 | 14.1 |
| pow | 79.2 | 66.5 | 0.840 | 69.4 |

None over 1.05: prelude's perf passes (std/evidence/prelude/perf.md, same
run). pow went 1.057 to 0.840 with 72abe2abd (frexp and ldexp were 1.245
of Go's in a split run). Tried and dropped: wrapping_add and wrapping_sub
for pow's loop counters (slower, gotchas/wrapping-add-slower-than-checked.md).