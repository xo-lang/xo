# Timing rows against Go, part 12 (2026-11-03)

Linux/arm64 offload host, `xo bench --ab` style runs (15 rounds, Go and Xo alternated, caches off) of bench/perf/cookiejar: `cookiejar` (decision 0475) against Go's net/http/cookiejar. Each round stores three cookies for one of 40 URLs and asks for the cookies of a deeper URL, 100000 rounds.

| bench | workload | Go wall (ms) | Xo wall (ms) | Xo / Go | interval | peak memory |
|---|---|---|---|---|---|---|
| cookiejar | none | 0.862 | 0.515 | 0.598 | 0.547-0.641 | 0.344 |
| cookiejar | set | 73.57 | 122.6 | 1.666 | 1.646-1.679 | 0.215 |
| cookiejar | get | 114.5 | 176.3 | 1.54 | 1.523-1.556 | 0.212 |
| cookiejar | both | 203.8 | 342.5 | 1.681 | 1.666-1.691 | 0.221 |

Not Proven: 1.5 to 1.7 of Go's wall, with a fifth of Go's peak memory. Likely cost (not yet profiled): every call builds the host key and entry ids as strings, lower cases the host, and the jar's maps are keyed by string; Go does the same work but with its tuned map and no per call list copies. Next: profile `set` and `get` and cut the allocations.
