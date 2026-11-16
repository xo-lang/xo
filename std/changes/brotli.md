# std/brotli changes

## 2026-11-15: perf: ring writes through BytesBuilder.write_list

- Kind: perf
- Commit: this push
- Found by: task #67, a CPU profile of the text workload (write_ring_buffer 23%, a byte at a time into the BytesBuilder)
- Go reference: none (Go's Reader writes the ring with one slice copy)
- Test: std/brotli_test.xo (unchanged, byte identical output); the bulk primitive's instruction-count guard
- Shipped in: not yet released
- Timing: host 1 `xo bench --ab`, against the decoder before: wall text 0.637, html 0.391, mixed 0.656; RSS 1.0. Against Go (bench/results/brotli-decode-vs-go-2026-11-15.md): Xo / Go wall text 1.93, html 3.80, mixed 1.62, max RSS 0.12; open gap against the 1.05 bar. The Mac twin survey of the same build gave 1.17, 1.12 and 0.99, so the host 1 gap is mostly linux/arm64 specific and unexplained yet.

The decoder writes each filled ring buffer to its output with one
`write_list` call instead of a push per byte.

## 2026-11-15: port: the decoder (decode, BrotliErr)

- Kind: port
- Commit: this push
- Found by: task #67 (owner, through xo-4c, 2026-11-15): std/ui's client module is 54.9 KB under Brotli against 68.6 KB under gzip; decision 0666
- Go reference: github.com/andybalholm/brotli v1.2.2 decode.go, huffman.go, transform.go, state.go, bit_reader.go, reader.go (NewReader, Read)
- Test: std/brotli_test.xo (64 andybalholm round trips at qualities 0 to 11 and windows 10 to 24, 1,622 changed, cut and hand made streams with Go's output or error); internal/stdtest/testdata/corpus/brotli (scripts/stdcorpus brotli: 160 seeded streams, some flushed part way, and 900 damaged ones; go, arm64, llvm)
- Shipped in: not yet released
- Timing: bench/perf/brotli against andybalholm's Reader (twincheck agrees); host 1 row in the perf entry above

`decode` reads a Brotli stream, as `io.ReadAll(brotli.NewReader(r))`
does, bounded by `max_size`. Errors are Go's texts at Go's points: the
decoder keeps the ring buffer sizes and the 32 KiB reads of Go's Reader,
which decide whether a command that runs past its meta-block is
`BLOCK_LENGTH_1` or `BLOCK_LENGTH_2`.
