# Timing rows against Go, part 10 (2026-11-03)

Linux/arm64 offload host, `xo bench --ab` style runs (15 rounds, Go and Xo alternated, caches off) of bench/perf/heap: `std/heap` (decision 0472) against Go's container/heap over `[]int` (`heap.Push`, `heap.Pop`, `heap.Fix`), 1,000,000 operations per workload. `none` is the setup alone.

| bench | workload | Go wall (ms) | Xo wall (ms) | Xo / Go | interval | peak memory |
|---|---|---|---|---|---|---|
| heap | none | 6.024 | 3.913 | 0.650 | 0.408-0.836 | 0.41 |
| heap | sort | 212.2 | 173.2 | 0.816 | 0.593-1.368 | 0.446 |
| heap | steady | 448.6 | 309.3 | 0.689 | 0.537-0.836 | 0.226 |
| heap | fix | 79.2 | 80.43 | 1.015 | 0.846-1.577 | 0.695 |

Push-all-then-pop-all (`sort`) is 0.82 with an interval that spans 1.0 (within noise), a steady-state queue (`steady`) 0.69, and `fix` of a random element 1.02 (within noise). Go's container/heap calls through an interface for every comparison and swap; a Heap holds its comparison as a closure and the native backends inline it.
