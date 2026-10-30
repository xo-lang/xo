# std/fnv changes

## 2026-10-29: perf: the 32 bit sums mask once

- Kind: perf
- Commit: this commit
- Found by: bench/perf/hash fnv32a at 1.289x Go's wall
- Go reference: hash/fnv fnv.go (sum32 and sum32a in uint32)
- Test: internal/stdtest/testdata/corpus/fnv on arm64, LLVM and Go
- Timing: bench/perf/hash fnv32a, quiet --ab 15 rounds: wall 0.792 (0.786 to 0.811) of the parent's, 1.037 (0.996 to 1.054) of Go's

`sum32`, `sum32a`, `update32` and `update32a` keep the 64 bit product and
take its low 32 bits once at the end (they depend only on the low bits),
not after every byte on the multiply chain.

## 2026-10-29: port: std/fnv, Go's hash/fnv

- Kind: port
- Commit: this commit
- Found by: the lead's queue (2026-10-29): a fast non cryptographic hash for sharding and hash keys
- Go reference: hash/fnv (fnv.go: New32, New32a, New64, New64a, New128, New128a, their Write, and the marshaled state for continuing a sum) of go1.26.1
- Test: internal/stdtest/testdata/corpus/fnv (fnv_test.go's golden inputs and more, values from Go's package; update split at every point, the 128 bit sums too) on arm64, LLVM and the Go backend; bench/perf/hash twincheck
- Timing: bench/perf/hash, quiet --ab 15 rounds against Go's hash/fnv (darwin/arm64), 2,000,000 keys of 8 to 40 bytes: fnv32a wall 1.289x Go (1.282 to 1.295, instructions 0.937); fnv64a 0.951x (0.946 to 0.972); fnv128a 0.757x (0.738 to 0.768)

`fnv.sum32`, `sum32a`, `sum64`, `sum64a` (Ints; a 64 bit sum's bits are
Go's uint64), `sum128` and `sum128a` (16 bytes, big endian), and an
`update` of each to continue a sum with more data.
