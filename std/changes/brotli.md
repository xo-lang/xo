# std/brotli changes



## 2026-11-16: feature: brotli.responses, br response compression (HTTPCompressor)

- Kind: feature
- Commit: this push
- Found by: task #89 (features Proven): stdaudit's andybalholm/brotli rows HTTPCompressor and HTTPCompressorWithLevel were gaps
- Go reference: andybalholm/brotli v1.2.2 http.go (HTTPCompressor: br when Accept-Encoding allows it, gzip otherwise); the rules are gzhttp's, as http.gzip_responses (decision 0305)
- Test: golden http_brotli_responses (go, arm64, LLVM): br, gzip and identity by Accept-Encoding with gzip_responses outside, min_size, compressed types, weak ETags, a stream through a Compressor
- Timing: none (encode and the Compressor keep their paths)

`brotli.responses(min_size, quality)` is `http.encoded_responses("br",
...)` (decision 0674). Wrap it inside gzip_responses:
`r.wrap(http.gzip_responses()).wrap(brotli.responses())`.

## 2026-11-16: feature: the streaming decoder, brotli.Decompressor (Go's Reader)

- Kind: feature
- Commit: this push
- Found by: task #89 (features Proven): stdaudit's andybalholm/brotli rows NewReader, Reader and Reader.Read were gaps
- Go reference: andybalholm/brotli v1.2.2 reader.go (Reader.Read: 32 KiB pieces, ErrUnexpectedEOF, errExcessiveInput); the decoding is std/brotli's straight line port of decode.go, resumed per meta-block, not Go's state machine
- Test: testdata/corpus/brotli: every stream in pieces of 1, 997 and 33,000 bytes and the 900 damaged streams in pieces of 5 and 4,093 bytes give decode's output or error, max_size as decode (go, arm64, llvm); std/brotli_test.xo: 16 MB of output in 64 KiB writes under 4 MB of heap (it caught the output of finished meta-blocks being dropped on a retry)
- Timing: none (decode keeps its path)

`brotli.decompressor(max_size)` returns a Decompressor with `write` and
`finish`, as std/compress's. Each meta-block is decoded from a
checkpoint; one whose input has not all arrived goes back to it (the
ring bytes it wrote restored) and is retried once the input has doubled.

## 2026-11-16: feature: the streaming encoder, brotli.Compressor (Go's Writer)

- Kind: feature
- Commit: this push
- Found by: task #89 (features Proven): stdaudit's andybalholm/brotli rows (pinned in scripts/stdaudit/outside.txt) NewWriter, Writer.Write, Flush and Close were gaps
- Go reference: andybalholm/brotli v1.2.2 writer.go (Writer, writeChunk) and encode.go (encoderCompressStream, encoderCompressStreamFast, injectBytePaddingBlock, checkFlushComplete)
- Test: testdata/corpus/brotli's new "the Compressor gives back each stream written with Flush" tests replay the Write sizes and Flush calls of andybalholm's 49 flushed streams (qualities 0 to 11) byte for byte, on go, arm64 and llvm; the encode tests (now on the Encoder) unchanged
- Timing: none (encode keeps its path: one write and finish)

`brotli.compressor(quality, window)` returns a Compressor with `write`,
`flush` and `finish`, as std/compress's Compressor. `encode` is now one
write and a finish on a Compressor.

## 2026-11-15: port: the encoder (encode, qualities 0 to 11)

- Kind: port
- Commit: this push
- Found by: task #67 (decision 0666's order: the encoder after the decoder)
- Go reference: github.com/andybalholm/brotli v1.2.2 encode.go, writer.go, compress_fragment.go, compress_fragment_two_pass.go, the hashers (h2 to h6, h10, the forgetful chain), static_dict.go, backward_references.go, backward_references_hq.go (Zopfli), block_splitter.go, cluster.go, histogram.go, metablock.go, brotli_bit_stream.go, entropy_encode.go, literal_cost.go, bit_cost.go, utf8_util.go
- Test: std/brotli_test.xo (encode matches andybalholm's Writer on the 57 round trip inputs at qualities 0 to 11; round trips at every quality); internal/stdtest/testdata/corpus/brotli (encode gives back each of the 111 streams andybalholm wrote with no Flush, a test per quality band; go, arm64, llvm); a probe of 11 inputs by 12 qualities by 3 windows (396 streams) byte identical, outside the repository
- Shipped in: not yet released
- Timing: bench/results/brotli-encode-vs-go-2026-11-15.md, host 1 Xo / Go wall 1.40 q1, 13.81 q6, 1.94 q11; max RSS 0.27, 0.45, 2.72. Open gap against the 1.05 bar, quality 6 the furthest (3.60 on the Mac twin survey, so mostly linux/arm64 specific); not profiled yet.

`encode(data, quality, window)` compresses as andybalholm's Writer does
for one Write and Close, at every quality: the fast paths of 0 and 1,
the hashers of 2 to 9 with the static dictionary search, and the Zopfli
search, block splitting and context modeling of 10 and 11. Out of range
qualities and windows are faults. The encoder's state goes to every
helper as a `var` parameter: by value, the go backend copied the hash
tree at each position (gotchas/selfhost-by-value-receiver-marks-lists.md).

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
