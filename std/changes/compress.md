# std/compress changes

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
