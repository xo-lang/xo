# std/compress changes

## 2026-10-28: perf: hdec_init builds its tables in locals

- Kind: perf
- Commit: this commit
- Found by: archive unzip's profile at main f516907c7 (hdec_init 13%, 44 of its 76 ms the fill loop's `h.chunks[off] = chunk`: a store through the struct's field each time)
- Go reference: compress/flate inflate.go (huffmanDecoder.init)
- Test: internal/stdtest/testdata/corpus/compress and archive (go, arm64, LLVM); TestSelfhostWasm crc32_vectors and archive_formats
- Timing: quiet --ab, 15 rounds, the parent's std as A: archive unzip instructions 0.951, wall 0.996 (0.974 to 1.013, within noise), cycles 0.988 (within noise). With the copy-back commit, against Go (twin.go.txt builds as A): unzip wall 1.036 (1.027 to 1.048), instructions 1.209; gunzip wall 1.01 (1.006 to 1.013), cycles 0.987

The chunk and link tables are local lists until the decoder is made at
the end, so the fill loop stores into a list it owns.

## 2026-10-28: perf: inflate copies a back-reference in bulk

- Kind: perf
- Commit: this commit
- Found by: archive unzip at about 1.26x Go; its CPU profile at main f516907c7 (huffman_block's byte at a time copy loop, `f.out[from + k]` then a push, 186 of 684 ms: 27%)
- Go reference: compress/flate dict_decoder.go (writeCopy: copy() in chunks, the overlapping case repeating its pattern)
- Test: std/native/llrt/raw_test.xo (u8s_copy_back against the byte loop for every distance and length up to 40, shared, unique and full lists); internal/stdtest/testdata/corpus/compress and archive (go, arm64, LLVM); TestSelfhostWasm crc32_vectors and archive_formats
- Timing: quiet --ab, 15 rounds, the parent's std as A: archive unzip wall 0.834 (0.825 to 0.852), instructions 0.814, cycles 0.835; compress gunzip wall 0.941 (0.927 to 0.949), instructions 0.889; compress inflate wall 0.938 (0.925 to 0.944), instructions 0.883. Against Go (bench/perf/archive and compress twin.go.txt builds as A): unzip wall 1.052 (1.037 to 1.062, was about 1.26), gunzip 1.002 (0.986 to 1.007, within noise)

huffman_block appends a back-reference with raw.u8s_copy_back: one
mem_copy when the distance covers the length, else chunks that double
from the pattern's start, in place when the output list is unique and
has room (a shared or full one moves to a list of twice the capacity
first). wasm, whose List[UInt8] holds a word an element, keeps the loop.

## 2026-10-28: perf: the CRC-32 table is a constant

- Kind: perf
- Commit: this commit
- Found by: bench/perf/compress and archive profiles (crc_table built for every inflate stream, every gzip write call, every zip reader)
- Go reference: hash/crc32 (its slicing-by-8 table is built once, ieeeOnce)
- Test: std/native/llrt/flate_test.xo (the constant equals the generating loop; hash/crc32's vectors); internal/stdtest/testdata/corpus/compress and archive (go, arm64, LLVM); TestSelfhostWasm crc32_vectors and archive_formats

crc_table() returns CRC_TABLE, the same 2,048 entries, where each call
built them (2,048 stores and a 2 KiB zeroing). Quiet --ab, 15 rounds,
the parent's std as A: archive unzip wall 0.997 (0.985 to 1.002, within noise), instructions 0.995, cycles 0.989; compress gunzip wall 0.983 (0.962 to 1.003, within noise), instructions 0.987, cycles 0.984. A small fixed cost per stream, not the inflate loop: the unzip and gunzip rows against Go stay where they were.

## 2026-10-28: perf: the deflater's hash tables hold UInt32, as Go's do

- Kind: perf
- Commit: this commit
- Found by: bench/perf/archive zip at 1.222x Go (measured on main 55af0adc5); its CPU profile (_bzero 12%: a new 131,072 and 32,768 entry table of 8 byte Ints zeroed for every deflate stream, one per zip entry)
- Go reference: compress/flate deflate.go (hashHead [hashSize]uint32, hashPrev [windowSize]uint32) (go1.26.1)
- Test: internal/stdtest/testdata/corpus/compress and archive (go, arm64, LLVM; the compressed bytes equal Go's); internal/arm64 archive_formats, http_gzip, crc32_vectors; TestSelfhostWasm archive_formats and crc32_vectors; twincheck's checksums agree on zip, gzip, deflate, unzip

hash_head and hash_prev are List[UInt32] (raw.zero_u32s: packed four
bytes an element natively, a word an element on wasm), the same table
sizes, so the matches and the output are unchanged; half the bytes to zero
for each stream and to read in the match search. Quiet --ab, 15 rounds,
the parent's std as A: archive zip wall 0.959 (0.950 to 0.964); Xo / Go about 1.17 (estimate: 1.222 x 0.959), from 1.222, instructions 0.969; gzip
and deflate (one stream each) instructions within 0.2%.
## 2026-10-28: fix: gunzip and unzip on wasm

- Kind: fix
- Commit: this commit
- Found by: internal/selfhost TestSelfhostWasm archive_formats failing on main (zip checksum error); bisected to 124e378a9 (2026-10-27, inflate's output as a packed List[UInt8])
- Go reference: none (a layout difference between Xo's backends)
- Test: internal/llvm/testdata/golden/crc32_vectors (new: hash/crc32's values through gzip's trailer, every length 0 to 40 round-tripped, a zip of a stored and a deflated file; checked against zlib) on wasm, arm64, LLVM and the Go backend; archive_formats on wasm

flate.list_bytes copied a List[UInt8]'s bytes in one copy from the
native packed layout, which wasm lists do not have (a word an element),
so every inflate on wasm returned garbage (gotchas/packed-list-raw-copy-wasm.md).
On wasm it now copies a byte at a time; the native targets keep the one
copy.

## 2026-10-28: perf: CRC-32 a word a step with arm64's crc32x

- Kind: perf
- Commit: this commit
- Found by: bench/perf/archive unzip at 1.41x Go and compress gunzip at 1.23x (measured on main 55af0adc5); unzip's profile: flate.crc32 13%
- Go reference: hash/crc32 crc32_arm64.go (ieeeUpdate with the CRC32X instructions) (go1.26.1)
- Test: std/native/llrt/flate_test.xo (zlib's IEEE values, whole and split at every offset, crc32_list) and cpu_test.xo (crc32x against its body) on arm64 and llvm; internal/llvm and internal/arm64 archive_formats and http_gzip with Go parity; linux/arm64 in Docker (the AT_HWCAP path); fixed point; test/codegen/intrinsics flate.crc32

std/compress, std/archive and std/http's gzip share flate.crc32: where
the CPU has arm64's CRC32 instructions (every Apple arm64 CPU; Linux's
AT_HWCAP bit), it takes eight bytes a step with one crc32x (cpu.crc32x,
decision 0231's addendum of 2026-10-28), and keeps slicing-by-8
elsewhere (amd64, whose crc32 instruction is the Castagnoli polynomial,
and wasm). Results unchanged. Quiet --ab, 15 rounds, the parent's xo and
std as A: compress gunzip 0.949 (0.941 to 0.953), instructions 0.956;
archive unzip 0.893 (0.888 to 0.902), instructions 0.915. Xo / Go about
1.17 for gunzip (estimate: 1.228 x 0.949) and 1.26 for unzip (1.406 x
0.893).

## 2026-10-27: perf: CRC-32 reads a word a step and Huffman tables reverse codes without a loop

- Kind: perf
- Commit: this commit
- Found by: bench/perf/archive unzip at 1.46x Go; its CPU profile (flate.crc32 13.5%, eight checked byte reads a step; hdec_init 12.4%, a 16 step loop to reverse every code)
- Go reference: hash/crc32 crc32_generic.go slicingUpdate; math/bits Reverse16 as compress/flate inflate.go huffmanDecoder.init uses it (go1.26.1)
- Test: internal/stdtest/testdata/corpus/compress and archive (go, arm64, LLVM); TestStdModules native archive; internal/llvm and internal/arm64 archive_formats, http_gzip, http_gzip_responses; twincheck bench/perf/archive and bench/perf/compress; reverse16 equal to the loop for all 65,536 inputs

The CRC-32 of std/compress, std/archive and the gzip of std/http (one
function in std/native/llrt/flate.xo) takes its eight bytes a step as
one load, and a Huffman decoder reverses each code's bits by four
swaps. Results are the same. Quiet --ab, 15 rounds, the parent's std as
A: bench/perf/archive unzip wall 0.965 (0.958 to 0.984), instructions
0.923; bench/perf/compress gunzip wall 0.987 (0.982 to 0.997), instructions
0.986.

## 2026-10-27: perf: flate's write_tokens keeps the bit buffer in locals

- Kind: perf
- Commit: this commit
- Found by: bench/perf/compress gzip1 CPU profile at 1000 Hz after the code tables (BitWriter.write_bits a call per code, offset_code a call per match)
- Go reference: compress/flate huffman_bit_writer.go writeTokens, writeCode and writeBits (go1.26.1)
- Test: internal/llvm/testdata/golden/archive_formats, lz4_roundtrip, http_gzip, http_gzip_responses (and internal/arm64's); internal/stdtest corpora; twincheck bench/perf/compress and archive; the outputs of gzip, gzip1, deflate and zip byte for byte the parent's

write_tokens holds the pending bits and their count in locals over the
token loop and writes six bytes (put48) once 48 are pending, as Go's
writeTokens does with writeCode and writeBits inlined; the offset code is
computed inline. Quiet --ab, 15 rounds, the parent's std as A: gzip1
wall 0.987 (0.968 to 0.992), instructions 0.911.

## 2026-10-27: perf: flate's length and offset codes by Go's tables

- Kind: perf
- Commit: this commit
- Found by: bench/perf/compress gzip1 (BestSpeed) at 1.57x Go after the first flate fixes; its CPU profile (write_tokens and index_tokens about half)
- Go reference: compress/flate token.go lengthCodes, offsetCodes and offsetCode (go1.26.1)
- Test: internal/llvm/testdata/golden/archive_formats, lz4_roundtrip, http_gzip, http_gzip_responses (and internal/arm64's); internal/stdtest corpora; twincheck bench/perf/compress and archive; the outputs of gzip, gzip1, deflate and zip byte for byte the parent's

length_code and offset_code searched the base tables a step at a time for
every match token, twice a block (index_tokens counts, write_tokens
writes); they are now Go's 256 entry tables, an offset above 255 looked
up as off >> 7 (+ 14) or off >> 14 (+ 28). Quiet --ab, 15 rounds, the
parent's std as A: gzip1 0.553, gzip 0.917, archive zip 0.883. Against
Go: gzip1 0.858 (from 1.57).

## 2026-10-27: perf: packed byte output and Go's fast paths in flate

- Kind: perf
- Commit: this commit
- Found by: bench/perf/compress, the std Go twin (1.43x to 1.94x Go)
- Go reference: compress/flate deflate.go matchLen, deflatefast.go load32 and matchLen, huffman_bit_writer.go writeBits (go1.26.1)
- Test: internal/llvm/testdata/golden/archive_formats, lz4_roundtrip, http_gzip, http_gzip_responses (and internal/arm64's); internal/stdtest compress and archive corpora; bench/perf/compress and archive twincheck

The inflater's output and the bit writer's are a List[UInt8] (one byte an
element; a List[Int] took a word a byte), handed out as Bytes in one copy;
match lengths compare 8 bytes a step and BestSpeed's hash reads 4 bytes in
one load, as Go's do; the bit writer keeps up to 63 bits and writes 6 bytes
once 48 are pending (a streaming take still gives out every whole byte).
Quiet --ab, 10 rounds, the parent's std as A: gzip 0.876, gzip BestSpeed
0.897, gunzip 0.859, deflate 0.872, inflate 0.851. Against Go: gzip 1.25,
BestSpeed 1.57, gunzip 1.29, deflate 1.24, inflate 1.22 (archive zip 1.36,
unzip 1.46).

## 2026-10-27: perf: CRC-32 slicing-by-8, and gunzip checks a member without copying it

- Kind: perf
- Commit: this commit
- Found by: bench/perf/compress, the std Go twin (gunzip 1.95x Go; a third of its time in the checksum and the member copy before it)
- Go reference: hash/crc32 slicing8Update (go1.26.1; Go's arm64 build uses the CRC32 instructions)
- Test: internal/llvm/testdata/golden/archive_formats, lz4_roundtrip, http_gzip, http_gzip_responses; bench/perf/compress and archive twincheck

The IEEE table is eight tables (2048 words), and crc32 takes eight bytes a
step; gunzip computes a member's CRC over the output list in place
(crc32_list) instead of copying the member into a list and then into
Bytes. Quiet --ab, 15 rounds, the parent's std as A: gunzip wall 0.773
(instructions 0.869), about 1.51x Go; gzip and inflate within noise.
Also used by std/archive (zip and gzip).

## 2026-10-21: feature: streaming Compressor and Decompressor

- Kind: feature
- Commit: 11919936
- Found by: decision 0305
- Go reference: compress/flate Writer (Write, Flush, Close) and Reader, compress/gzip, compress/zlib (go1.26.1)
- Test: internal/stdtest/testdata/corpus/compress (160 chunked Compressors with flushes against Go's bytes; 431 streams fed to a Decompressor in pieces against the whole decoders)

`compress.compressor(format, level)` and `compress.decompressor(format,
max_size)` work push style, bytes in and bytes out. A Compressor's
outputs join to Go's bytes for the same writes and flushes. A
Decompressor's outputs join to what `gunzip`, `inflate`, or `unzlib`
gives. Both are move only, as the builders are (core 5.3).

## 2026-10-21: feature: std/compress, Go's gzip, flate, and zlib with max_size

- Kind: feature
- Commit: 6e3bdf2c
- Found by: the std coverage table (decision 0276; owner approval 2026-10-21, decision 0305)
- Go reference: compress/flate (deflate.go, deflatefast.go, huffman_bit_writer.go, inflate.go), compress/gzip, compress/zlib, hash/adler32 (go1.26.1)
- Test: internal/stdtest/testdata/corpus/compress (scripts/stdcorpus compress: 720 writes over every level, 431 decoder cases with Go's results)

`gzip`, `deflate`, and `zlib` write the bytes Go's writers write at every
level (HuffmanOnly, NoCompression, BestSpeed's own encoder, 2 to 9).
`gunzip`, `inflate`, and `unzlib` give Go's output or Go's error, and stop
at `max_size` (default 1 GiB) against decompression bombs. Natively, the
port of compress/flate that std/archive held at levels 5 and 6 moved to
std/native/llrt/flate.xo and gained every level.
