# std twin strconv: itoa host row after decision 0465 step 2 (2026-11-03)

Host 1, `xo bench --early`, Xo at 7101c5ed3, n=6, workload itoa only: Go 86.24 ms, Xo 116.3 ms, ratio 1.348 (1.331-1.378), peak memory 0.143 of Go's. Before (std-twin-rows2-2026-11-03.md:78): 1.522. The loop is `"${n}".len()` (bench/perf/strconv/main.xo:38-41): string interpolation of an Int, whose result Str is allocated per value; that lowering is in the prelude and codegen (Lang 1), not in std. Not Proven (ADR 0461).
