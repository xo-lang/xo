# std/brotli changes

## 2026-11-15: port: the decoder (decode, BrotliErr)

- Kind: port
- Commit: this push
- Found by: task #67 (owner, through xo-4c, 2026-11-15): std/ui's client module is 54.9 KB under Brotli against 68.6 KB under gzip; decision 0666
- Go reference: github.com/andybalholm/brotli v1.2.2 decode.go, huffman.go, transform.go, state.go, bit_reader.go, reader.go (NewReader, Read)
- Test: std/brotli_test.xo (64 andybalholm round trips at qualities 0 to 11 and windows 10 to 24, 1,622 changed, cut and hand made streams with Go's output or error); internal/stdtest/testdata/corpus/brotli (scripts/stdcorpus brotli: 160 seeded streams, some flushed part way, and 900 damaged ones; go, arm64, llvm)
- Shipped in: not yet released
- Timing: bench/perf/brotli against andybalholm's Reader (twincheck agrees); the host 1 row is to come

`decode` reads a Brotli stream, as `io.ReadAll(brotli.NewReader(r))`
does, bounded by `max_size`. Errors are Go's texts at Go's points: the
decoder keeps the ring buffer sizes and the 32 KiB reads of Go's Reader,
which decide whether a command that runs past its meta-block is
`BLOCK_LENGTH_1` or `BLOCK_LENGTH_2`.
