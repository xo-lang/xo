# 0510: std/jpeg encode, a port of Go's image/jpeg writer

Status: Proposed

## Context

std/jpeg (decision 0467) decodes. A service that makes thumbnails or re-encodes uploads needs
the other direction. Go's `jpeg.Encode` (writer.go, about 640 lines, plus the forward DCT in
dct.go) writes baseline 4:2:0 (or one component for Gray) with the standard Huffman tables and
a quality scaled quantization table. Decision 0467 left the encoder as a separate item
("jpeg.Encode is a separate item if a program needs it"). std/png already has `encode`
(decision 0466), so a program that decodes either format and writes the other has no JPEG
writer today.

## Decision

Add `jpeg.encode` to std/jpeg, a port of Go's writer.go and `fdct`, with attribution and Go's
integer arithmetic, so the output is Go's bytes (byte for byte, not only a decodable file):

    fn encode(img: Image, quality: Int = 75) -> Bytes ! JpegErr

- Input is std/jpeg's `Image`: `Gray` (one component), `YCbCr` (any subsample ratio: like Go's
  yCbCrToYCbCr, the ratio only chooses where each sample is read, the output is 4:2:0),
  `RGBA` (Go's image.RGBA fast path: the bytes are premultiplied, alpha is ignored) and `CMYK`
  (Go's generic path through `color.CMYK.RGBA`). The edge blocks replicate the last row and
  column as Go does.
- `quality` is clamped to 1..100 as Go clamps `Options.Quality`; the default is
  `DefaultQuality` 75.
- Errors: a width or height of 65,536 or more gives Go's `jpeg: image is too large to encode`
  as a new `JpegErr.Encode(msg: Str)` variant; an `Image` whose plane lengths disagree with its
  size is a `fault` (as `png.encode`: Go's images cannot be inconsistent, so it is a
  caller bug, not an error value). A zero size image encodes as Go does (the differential decides).
- No streaming writer in this decision: the encoder returns a whole buffer like `png.encode`.
  Go's buffered writer and its sticky error have no counterpart (no I/O errors here).
- The Huffman tables, `unscaledQuant`, `bitCount` and the zig-zag order are Go's constants;
  the Huffman LUTs are built once at first use (Go builds them in init).

## Consequences

- One more entry in `spec/std.md` 49 and a `JpegErr` variant: a program that matches every
  variant of `JpegErr` gains a case (additive, same as 0467's own Truncated).
- Corpus: Go's `jpeg.Encode` output for the decoded images of the 0467 corpus at qualities
  1, 25, 50, 75, 90, 100, for Gray, YCbCr (all six ratios), RGBA and CMYK inputs, odd sizes
  (1x1, 7x9, 17x31) and sizes that are not multiples of 16; the expected bytes come from a Go
  program (`scripts/stdcorpus/jpegenc`) and are committed. Plus a fuzz differential: seeded
  random images and qualities, Go against Xo byte for byte, on arm64 and LLVM.
- Timing: a twin row in `bench/perf/jpeg` (`encode`, Go's `jpeg.Encode` of the same images,
  checksum of the output) with Go's fast paths ported (the RGBA and Gray block readers, the
  integer fdct, the Huffman LUT), measured with `xo bench --ab` / hostq2 on linux/arm64.
- Round trip test: `decode(encode(x))` of a smooth image stays within the quantization error
  Go's own round trip shows (checked against Go in the corpus, not by a tolerance of ours).
- Output compatibility is the contract: a later change that makes the encoder faster or
  smaller than Go's (optimized Huffman tables, progressive output) is a different function.

## Alternatives

- A tolerant encoder (any valid baseline JPEG): easier, but no oracle for correctness beyond
  "decodes", and programs that cache by output hash would differ from Go services.
- Progressive or optimized-Huffman output: not in Go's image/jpeg; a separate decision if
  wanted.
- Wrapping a C library (libjpeg-turbo): rejected, std is written in Xo (std/jpeg is pure, like std/png).
