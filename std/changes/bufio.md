# std/bufio changes

## 2026-11-03: feature: std/bufio, a buffered reader over any Stream

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-30 backend survey (std-coverage.md rank 8: every line protocol hand writes a buffer over `read(max)`); decision 0471
- Go reference: bufio.Reader (ReadBytes, Peek, Read; bufio.go) and io.ReadFull
- Test: internal/stdtest/testdata/corpus/bufio (Go's bufio.Reader and io.ReadFull on 700 scripted data and chunkings, plus limit, EOF, error and long line tests) on arm64, LLVM and the Go backend; bench/perf/bufio twincheck
- Timing: see bench/results/std-twin-rows9-2026-11-03.md

`bufio.reader(stream)` with `read`, `read_until`, `read_line`, `read_exact`, `peek` and `buffered` (spec/std.md 55). A limit on a record (`TooLong`) keeps a peer from growing it without bound, which Go's ReadBytes does not offer.
