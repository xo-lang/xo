# std/random changes

## 2026-11-16: perf: one draw a ziggurat try, as Go

- Kind: perf
- Commit: this push
- Found by: task #87 (std/evidence/random/perf.md, normal 1.83 and exp 2.08 of Go on host 1), XO_PROF=cpu on bench/perf/random exp: two generator draws and two interface calls a try where Go draws one Uint64
- Go reference: math/rand/v2 normal.go (NormFloat64), exp.go (ExpFloat64), go1.26.1
- Test: internal/stdtest/testdata/corpus/random (normal and exp draw for draw on Go's recorded streams; its replay Rand now maps the one int to Go's Uint64); bench/perf/random on the Mac: exp instructions 4.72G to 2.79G (0.59), normal 4.84G to 3.05G (0.63), 2*10^7 draws
- Shipped in: not yet released

`normal` and `exp` draw one `r.int(Int.min, Int.max)` a try and split it as Go splits its Uint64 (the native Rand answers that range without a division); a seeded Rand gives other values for the same seed than before (CHANGELOG).

## 2026-11-15: feature: zipf and Zipf.draw (Go's NewZipf and Zipf.Uint64)

- Kind: feature
- Commit: this push
- Found by: scripts/stdaudit gaps math/rand/v2.NewZipf, Zipf and Zipf.Uint64 (decision 0655)
- Go reference: math/rand/v2 zipf.go (go1.26.11; Copyright 2009 The Go Authors, BSD-style license)
- Test: internal/stdtest corpus random "zipf", twelve parameter sets (nil ones included) with 200 draws each, replayed on Go's recorded PCG stream (go, arm64, llvm)
- Shipped in: not yet released

`random.zipf(s, v, imax)` and `draw(r)`: Hörmann and Derflinger's rejection-inversion as Go writes it, one `r.float()` per try. The Zipf takes the Rand at each draw, where Go's holds one.

## 2026-11-11: feature: std/random, Go's NormFloat64, ExpFloat64 and Perm over any Rand

- Kind: feature
- Commit: this push
- Found by: the std audit of 2026-11-11 (bench/results/std-audit-2026-11-11.md, gap 17), decision 0606
- Go reference: math/rand/v2 normal.go, exp.go, rand.go Shuffle and Perm (go1.26.1)
- Test: the random corpus on go, arm64, llvm (1,500 normal and 1,500 exponential values and 40 permutations equal to Go's bit for bit on Go's recorded PCG streams; the moments of 10^6 draws)
- Timing: none (a few multiplies and table reads a value; the Rand dominates)
- Shipped in: not yet released

A new merged, pure module: normal, exp and perm, the ziggurat with Go's tables and float32 comparisons, drawing through r.int and r.float. spec/std.md 81.
