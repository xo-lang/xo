# 0467: std/jpeg, a port of Go's image/jpeg decoder

Status: Proposed (Xo Lang 3, 2026-11-03; roadmap priority 1 after std/png, from the Xo Lead)

## Context

std/png (decision 0466) covers lossless images. JPEG is the other format a web service meets
(uploads, thumbnails, EXIF stripping). Go's image/jpeg decoder is about 1,600 lines
(reader.go, huffman.go, scan.go) plus the integer IDCT in dct.go; baseline and progressive,
all chroma subsampling ratios, restart intervals, Adobe CMYK/YCCK and RGB.

## Decision (proposed)

A new module `std/jpeg`, written once in Xo (std/native/jpeg.xo, go_compiled like std/png),
a port of Go's decoder with its integer IDCT and color handling, so the decoded planes match
Go byte for byte; decode and decode_config only (no encoder in this decision).

The API is written when the port is done; its shape follows std/png: one `Image` struct that
carries Go's planar layouts (Gray, YCbCr with its subsample ratio and strides, CMYK, RGBA),
a `Config`, a `JpegErr` enum with Go's `FormatError` and `UnsupportedError` texts, and
`to_rgba` for programs that only want pixels.

## Consequences

To be filled in with the implementation (corpus from Go's image/testdata, a fuzz differential
and a timing row, as for std/png).
