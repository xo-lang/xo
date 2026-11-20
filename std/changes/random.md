# std/random changes

## 2026-11-19: perf: the x86-64 ChaCha8 block in Go's vertical layout

- Kind: perf
- Commit: this push
- Found by: std/random Proven perf on linux/amd64 (exp 1.05 of Go with its count branch-free, the rest the generator): 1000 Hz profiles of 1.2 * 10^8 bare words put the ChaCha8 block at 80% of Xo's time and 55% of Go's, at about the same instruction count (Xo's kernel ran two blocks at a time in rows, latency bound, then transposed)
- Go reference: internal/chacha8rand chacha8_amd64.s, go1.26.1
- Test: internal/llvm TestRuntimeRandChaCha8/amd64 on the linux/amd64 host (Go's TestOutput vector, 372 words, unchanged); bench/perf/random draw (new: 5 * 10^7 bare words) 0.663 (0.662 to 0.683) of the time (`xo bench --ab`, linux/amd64 host, 6 rounds, verdict better); 1.2 * 10^8 words 0.495 to 0.325 s, Go 0.479 s
- Shipped in: not yet released

rt.chacha8_rand4's x86-64 lowering (compiler/ssa x86chacha.xo chacha8rand_x86_lines) is now a port of Go's kernel: word k of the four blocks in xmm k, so each half round is four independent quarter rounds and the result is already in Go's layout. The rotations by 16 and 8 stay pshufb, their tables loaded from the output buffer during the rounds. arm64 (NEON) and the llvm backend are unchanged.

## 2026-11-19: perf: normal without a branch on the draw's sign

- Kind: perf
- Commit: this push
- Found by: task #87 (normal 1.7x of Go on linux/amd64 with equal instruction counts): `normal`'s `if w >= 2^31` and `if j < 0` test a random bit, so each branch was mispredicted half the time; Go's compiler makes both conditional moves
- Go reference: math/rand/v2 normal.go (NormFloat64), go1.26.1
- Test: golden random_seeded and internal/stdtest/testdata/corpus/random (draws equal to Go's, unchanged); bench/perf/random normal 0.782 (0.774 to 0.798) of the time (`xo bench --ab`, host 1); linux/amd64 10^6 draws 21.3 to 16.9 ms, Go 12.5 ms
- Shipped in: not yet released

`normal` computes `int32(u)` as `w - ((w & 2^31) << 1)` and its magnitude as `(j ^ m) - m` with `m = j >> 63`; the values are unchanged. Turning such branches into selects in the compiler instead was slower elsewhere (bench/perf/text float_fmt 1.025, zone_offset 1.039), since it cannot tell a random bit from a predictable test.

## 2026-11-16: feature: seeded and fixed, pure Rand sources

- Kind: feature
- Commit: this push
- Found by: task #141 (decision 0652 step 2b): a pure Rand for sans-I/O code and tests
- Go reference: math/rand/v2 NewChaCha8 and ChaCha8.Uint64, internal/chacha8rand (go1.26.11)
- Test: golden random_seeded (arm64, LLVM, wasm, Go parity): Go's words 0 to 3, 31, 32, 123, 124 and 299 and a checksum of 300 for one seed; both sources passed as a `Rand[()]`
- Shipped in: not yet released
- Timing: none yet (a List based block, not the runtime's; a timing row against Go's ChaCha8 is to follow)

`random.seeded(seed)` is a `Rand[()]` over Go's ChaCha8 stream, and `random.fixed(data)` one that answers the given bytes, then faults. Code given either stays pure (decision 0652).

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
