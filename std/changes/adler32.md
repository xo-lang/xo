# std/adler32 changes

## 2026-11-03: port: std/adler32, Go's hash/adler32

- Kind: port
- Commit: this commit
- Found by: the lead's roadmap (priority 2: the hash packages), std-coverage.md planned row
- Go reference: hash/adler32 (adler32.go: two sums mod 65521, nmax blocks) of go1.26.1
- Test: internal/stdtest/testdata/corpus/adler32 (Go's values: its golden strings and pseudo random buffers of lengths that cross the word step and the block, split at several points) on arm64, LLVM and the Go backend; bench/perf/hash twincheck
- Timing: see bench/results/std-twin-rows9-2026-11-03.md

`adler32.checksum` and `adler32.update` (spec/std.md 52). With 64 bit sums the modulus is taken once a megabyte, not every 5,552 bytes.
