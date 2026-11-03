# 0467: std/jpeg, a port of Go's image/jpeg decoder

Status: Accepted (owner, 2026-11-03)

## Context

std/png (decision 0466) covers lossless images. JPEG is the other format a web service meets
(uploads, thumbnails, EXIF stripping). Go's image/jpeg decoder is about 1,600 lines
(reader.go, huffman.go, scan.go) plus the integer IDCT in dct.go; baseline and progressive,
all chroma subsampling ratios, restart intervals, Adobe CMYK/YCCK and RGB.

## Decision (proposed)

A new module `std/jpeg`, written once in Xo (std/native/jpeg.xo, go_compiled like std/png),
a port of Go's decoder with its integer IDCT and color handling, so the decoded planes match
Go byte for byte; decode and decode_config only (no encoder in this decision).

The API (spec/std.md 49) follows std/png: one `Image` struct that carries Go's planar
layouts (Gray, YCbCr with its subsample ratio, CMYK, RGBA) with tight rows, a `Config`, a
`JpegErr` enum with Go's `FormatError` and `UnsupportedError` texts plus `Truncated`, and
`to_rgba` for programs that only want pixels. A frame over `max_pixels` is refused from the
header.

## Consequences

- The decoder reads a whole buffer (Go reads a stream): Go's bit reader and its one byte
  unread are kept as an index into the buffer. Go's int32 arithmetic is emulated where it can
  wrap (before a right shift in the IDCT and at stores); the bit accumulator is masked to 32
  bits.
- Corpus from Go (186 cases) and a fuzz differential (350 files in the committed corpus,
  fresh seeds in TestJpegFuzz, 12,000 cases agree on arm64 and LLVM), as for std/png.
- Timing (host 1, bench/perf/jpeg, four of Go's test images, 100 passes): decode 1.01 and
  decode with `to_rgba` 1.04 of Go's wall (decision 0461 asks for about 1.05: both meet it, so
  the module is Proven). It was 1.38 and 1.41 at the first port. What moved it: progressive
  scans decode in place on the coefficient list, quantization tables in natural order, and
  branch free clamps and sign extension, shifts for to_rgba's chroma index (the values are unpredictable, so branches cost more
  than the masks). Tried and measured slower on arm64: written out Huffman table hits, a DC only
  shortcut, an IDCT row shortcut.
- No encoder: `jpeg.Encode` is a separate item if a program needs it.

## Alternatives

- Binding a C decoder: rejected; the bytes must match Go's decoder, including its IDCT and error texts, and std stays pure Xo.
