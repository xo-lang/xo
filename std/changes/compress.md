# std/compress changes

## 2026-10-21: feature: std/compress, Go's gzip, flate, and zlib with max_size

- Kind: feature
- Commit: (filled in after the push)
- Found by: the std coverage table (decision 0276; owner approval 2026-10-21, decision 0305)
- Go reference: compress/flate (deflate.go, deflatefast.go, huffman_bit_writer.go, inflate.go), compress/gzip, compress/zlib, hash/adler32 (go1.26.1)
- Test: internal/stdtest/testdata/corpus/compress (scripts/stdcorpus compress: 720 writes over every level, 431 decoder cases with Go's results)

`gzip`, `deflate`, and `zlib` write the bytes Go's writers write at every
level (HuffmanOnly, NoCompression, BestSpeed's own encoder, 2 to 9).
`gunzip`, `inflate`, and `unzlib` give Go's output or Go's error, and stop
at `max_size` (default 1 GiB) against decompression bombs. Natively, the
port of compress/flate that std/archive held at levels 5 and 6 moved to
std/native/llrt/flate.xo and gained every level.
