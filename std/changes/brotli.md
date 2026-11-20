# std/brotli changes

## 2026-11-19: perf: quality 10 and 11 price each copy code's run of lengths once

- Kind: perf
- Commit: this push
- Found by: task #271 (enc11 instruction profile of update_nodes on linux/arm64 host 1: its hottest loop computed copy_code, with checked arithmetic, for every candidate length, though the command's cost changes only when the copy code does)
- Go reference: andybalholm/brotli backward_references_hq.go updateNodes (computes getCopyLengthCode for every length; Xo now walks ENC_COPY_BASE's runs instead, the same costs and choices)
- Measured: callgrind on linux/arm64 (68932cfca against 9e67ae77c): bench/perf/brotli enc11 7.77% fewer instructions, enc6 unchanged (quality 6 does not use it); `xo bench --ab` on linux/arm64 host 1 (6 rounds): enc11 0.901 (95% 0.875 to 0.931, 253.8 to 228.8 ms, better)
- Test: brotli module tests (8 passed); bench output unchanged (enc11 13312, enc6 167596 both)
- Affected: none (no behavior change)
- Shipped in: not yet released

## 2026-11-19: perf: quality 10 and 11 keep each node's distance cache in the queue

- Kind: perf
- Commit: this push
- Found by: task #271 (enc11 profile on linux/arm64 host 1 against the Go twin: alloc.resize, obj.grow and rl_grow_n about 3.5% of Xo's time, none in Go's; evaluate_node built a 4 element List for every position)
- Go reference: andybalholm/brotli backward_references_hq.go evaluateNode, computeDistanceCache and startPosQueuePush (a [4]int on the stack)
- Measured: callgrind on linux/arm64 (9c2150dc0 against deb934b16): bench/perf/brotli enc11 2.04% fewer instructions; `xo bench --ab` on linux/arm64 host 1 (6 rounds): enc11 0.966 (95% 0.948 to 0.985, 265.2 to 256.3 ms, better)
- Test: brotli module tests (8 passed); bench output unchanged (enc11 13312, enc6 167596 both)
- Affected: none (no behavior change)
- Shipped in: not yet released

## 2026-11-19: perf: the encoder's match probes compute each offset once

- Kind: perf
- Commit: this push
- Found by: task #271 (enc11 line profile on the Mac: update_nodes' distance cache probe, brotli.xo line 5112, 12.5% flat; its disassembly computed `cur_m + best_len` as a checked add three times per probe, as the compiler does not reuse a checked add)
- Go reference: andybalholm/brotli backward_references_hq.go updateNodes and h5.go FindLongestMatch (same probes)
- Measured: callgrind on linux/arm64 (423898b0b against this change): bench/perf/brotli enc11 2.39% fewer instructions, enc6 1.27% fewer; `xo bench --ab` on linux/arm64 host 1 (4 rounds each): enc11 0.978 (95% 0.953 to 0.985, 289.0 to 282.7 ms, better), enc6 (5 repeats) 0.991 (within noise); max RSS unchanged
- Test: brotli module tests (encoder round trips; bench output unchanged: enc11 13312 both)
- Affected: none (no behavior change)
- Shipped in: not yet released

## 2026-11-19: perf: the encoder's bit writer stores 8 bytes at once

- Kind: perf
- Commit: this push
- Found by: task #271 (enc1 profile on the Mac: write_bits 28% flat; its eight indexed byte stores each kept a bounds check, which an up front range check did not remove)
- Go reference: andybalholm/brotli write_bits.go (binary.LittleEndian.PutUint64, one store)
- Measured: `xo bench --ab` on linux/arm64 host 1 (22400a562 against this change, 6 rounds each): bench/perf/brotli enc1 0.767 (95% 0.759 to 0.781, 26.4 to 20.2 ms, "too short"), enc6 0.993 (noise), enc11 1.008 (within noise); max RSS enc1 0.948
- Test: brotli module tests (encoder round trips; the bench output unchanged in twincheck)
- Affected: none (no behavior change)
- Shipped in: not yet released

## 2026-11-19: perf: the encoder reads an 8-byte word with one load

- Kind: perf
- Commit: this push
- Found by: task #271 (perf profile of quality 1 on linux/arm64 host 2: write_bits 23% and match_len 19%, neither in Go's profile; ld64 was eight checked byte loads, as the arm64 backend does not combine loads)
- Go reference: andybalholm/brotli's binary.LittleEndian.Uint64 reads (one load)
- Measured: `xo bench --ab` on linux/arm64 host 1 (origin/main e6c1d8b8e against this change): bench/perf/brotli enc1 0.783 of the time (12 rounds, 95% 0.776 to 0.798, 38.8 to 30.4 ms), enc6 0.965 (6 rounds), enc11 1.003 (4 rounds, unchanged); max RSS unchanged; runs under 40 ms read "too short"
- Test: brotli module tests (ld64 against the byte loop at every offset)
- Affected: none (no behavior change)
- Shipped in: not yet released

## 2026-11-18: perf: the encoder's hash table is one zeroed allocation

- Kind: perf
- Commit: this push
- Found by: task #271 (bench/perf/brotli encode profile: growth copies of the table built by pushes were 16% of quality 1)
- Go reference: andybalholm/brotli encode.go getHashTable (one cleared table)
- Measured: Mac 2000 quality 1 encodes 1.27 to 0.90 s (page faults 232k to 128k); with the prelude's fresh mapping skip, 400 quality 6 encodes 1.04 to 0.82 s (quality 1 both: 1.28 to 0.95 s)
- Test: brotli module tests (output bytes unchanged in the bench)
- Affected: none (no behavior change)
- Shipped in: not yet released

## 2026-11-18: perf: decoding inlines the bit reader's fast path

- Kind: perf
- Commit: this push
- Found by: task #271 (bench/perf/brotli text decode about 1.2 of Go)
- Go reference: andybalholm/brotli bit_reader.go (the inlined fast paths of getBits and dropBits)
- Measured: host 1 `xo bench --ab` text decode wall 0.909 (0.884 to 0.909); Mac 5000 text decodes 1.72 to 1.51 s, mixed 0.795 to 0.688 s
- Test: brotli module tests; test/codegen/faultexit (no call to a small padded reader left in its caller)
- Affected: none (no behavior change)
- Shipped in: not yet released

peek_bits and drop_bits keep their refill and need_bytes paths in
fill_bits and drop_bits_more, out of line, and the arm64 inliner now
takes the rest at each call site (decision 0716).

## 2026-11-18: perf: quality 10 and 11 encode prices commands once per copy code (task #271)

- Kind: perf
- Commit: this push
- Found by: the Proven perf run on the linux/amd64 host (enc11 1.645 of Go)
- Go reference: andybalholm/brotli's zopfli (updateNodes, the cost model built per block)
- Test: std/brotli_test.xo's round trips at every quality (output unchanged; bench/perf/brotli's enc1, enc6, enc11 and text outputs match before and after)
- Shipped in: not yet released

update_nodes no longer rebuilds its constant Lists on each call, computes a command's cost once for each copy code instead of once per length, and rb_match compares 4 bytes a step. `xo bench --ab` of bench/perf/brotli enc11 on host 1: wall 0.937 (0.93 to 0.947), 3 of 3 rounds better. The rest of the gap to Go is arm64 register allocation in update_nodes (its loop values spill; notes on the regalloc work under decision #142).

## 2026-11-16: perf: the encoder's hash tables hold uint32s, as Go's do (task #156)

- Kind: perf (memory)
- Commit: this push
- Found by: xw-1's brotli stress run (host 2: rss 3183 to 3613 MB, the 10% rule failed)
- Go reference: andybalholm/brotli's h5, h6, h10 and hashToBinaryTree (`[]uint32`)
- Test: std/brotli_test.xo "a quality 9 encode holds its hash table as Go's uint32s, 32 MB, not 64" (fails before: mem_peak rose about 64 MB)
- Shipped in: not yet released

The bucket tables of qualities 2 to 9 and the binary tree of qualities 10 and 11 were `List[Int]`,
8 bytes a position where Go keeps 4: a quality 9 encode of a few KB filled a 64 MB table (`(1 << 15) << 8`
entries, touched all over by the hash), Go's 32 MB. They are `List[UInt32]` now, each position stored
as Go's `uint32(...)` stores it; the output is unchanged (the corpus and the stress program compare
bytes with andybalholm's). On amd64 llvm the 64 task stress program's resident set at its 5 minute
mark fell from 3150 MB to about 1350 MB (max 4.29 GB to 2.30 GB): memory use halved. No speed
claim (that needs a host 1 `xo bench --ab`, decision 0340). Why 3 GB at all: 64 tasks each in a quality 7 to 11 encode at once, 16 to 64 MB
of table each; the growth the stress run saw tracks how many are live at a sample, not a leak.



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
