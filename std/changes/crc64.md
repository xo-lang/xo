# std/crc64 changes

## 2026-11-03: port: std/crc64, Go's hash/crc64

- Kind: port
- Commit: this commit
- Found by: the lead's roadmap (priority 2: the hash packages), std-coverage.md planned row
- Go reference: hash/crc64 (crc64.go: slicing-by-8, the ISO and ECMA tables) of go1.26.1
- Test: internal/stdtest/testdata/corpus/crc64 (Go's values: its golden strings and pseudo random buffers of lengths that cross the word step and the block, split at several points, the tables against the bit loop) on arm64, LLVM and the Go backend; bench/perf/hash twincheck
- Timing: see bench/results/std-twin-rows9-2026-11-03.md

`crc64.checksum` and `crc64.update` (spec/std.md 51). The Int holds Go's uint64 bits.
