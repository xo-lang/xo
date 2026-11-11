# std/random changes

## 2026-11-11: feature: std/random, Go's NormFloat64, ExpFloat64 and Perm over any Rand

- Kind: feature
- Commit: this push
- Found by: the std audit of 2026-11-11 (bench/results/std-audit-2026-11-11.md, gap 17), decision 0606
- Go reference: math/rand/v2 normal.go, exp.go, rand.go Shuffle and Perm (go1.26.1)
- Test: the random corpus on go, arm64, llvm (1,500 normal and 1,500 exponential values and 40 permutations equal to Go's bit for bit on Go's recorded PCG streams; the moments of 10^6 draws)
- Timing: none (a few multiplies and table reads a value; the Rand dominates)
- Shipped in: not yet released

A new merged, pure module: normal, exp and perm, the ziggurat with Go's tables and float32 comparisons, drawing through r.int and r.float. spec/std.md 81.
