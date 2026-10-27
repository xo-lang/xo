# std/compress changes

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
