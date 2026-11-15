# std/bufio changes

## 2026-11-15: feature: `BufErr` is an open enum (decision 0649)

- Kind: feature
- Commit: this commit
- Found by: decision 0649 (a new variant of a std error enum broke two SDKs, 0644's `ClientErr.Tls`)
- Go reference: none (Go's errors are values compared with errors.Is; Rust's `#[non_exhaustive]`)
- Test: compiler/check's open enum test; the open_enum golden; xo check over the programs that use the module

`BufErr` is declared `open`: a `match` on it outside std needs a `_` (or
binding) arm, warning XO0306 in 0.52 and an error after, so that std can
add a variant in a minor release without breaking a build. Matches that
already have `_` are unchanged.

## 2026-11-11: refactor: std/bufio is one file (decisions 0588, 0597)

- Kind: refactor
- Commit: this push
- Found by: decision 0597 (std/native/bufio.xo imports nothing native)
- Go reference: none (no behavior change)
- Test: TestStdCorpora bufio on go, arm64, llvm
- Shipped in: not yet released

std/native/bufio.xo moved into std/bufio.xo. The Go backend compiles it
from that source too: its hand-written Go twin (std/rt/bufio.go) is gone.

## 2026-11-03: feature: std/bufio, a buffered reader over any Stream

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-30 backend survey (std-coverage.md rank 8: every line protocol hand writes a buffer over `read(max)`); decision 0471
- Go reference: bufio.Reader (ReadBytes, Peek, Read; bufio.go) and io.ReadFull
- Test: internal/stdtest/testdata/corpus/bufio (Go's bufio.Reader and io.ReadFull on 700 scripted data and chunkings, plus limit, EOF, error and long line tests) on arm64, LLVM and the Go backend; bench/perf/bufio twincheck
- Timing: see bench/results/std-twin-rows9-2026-11-03.md

`bufio.reader(stream)` with `read`, `read_until`, `read_line`, `read_exact`, `peek` and `buffered` (spec/std.md 55). A limit on a record (`TooLong`) keeps a peer from growing it without bound, which Go's ReadBytes does not offer.
