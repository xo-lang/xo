# 0466: std/png, a port of Go's image/png (decode and encode)

Status: Accepted (owner, 2026-11-03)

## Context

std-coverage.md lists `image` and `image/*` as "not planned". The roadmap now puts PNG first,
then JPEG decode, then CBOR. Programs that serve or process images (thumbnails, charts, QR
codes) otherwise carry their own decoder. Go's image/png is about 1,800 lines
(reader.go 1,054, writer.go 660, paeth.go 71) over compress/zlib, which Xo has
(`compress.zlib`, `compress.unzlib`).

## Decision (proposed)

A new module `std/png`, written once in Xo (std/native/png.xo, listed in std/go_compiled.txt
like path, mail and rate, decision 0312; no std/rt Go file), a port of Go's image/png with
Go's tests and a Go-generated corpus (scripts/stdcorpus/png.go).

### API

Go's `image` package has an Image interface with seven concrete types; Xo has no such
interface, so one struct carries Go's pix layouts unchanged (a pixel buffer a program can
hand to another Go-compatible tool, and a corpus that compares bytes):

```
enum Model { Gray, Gray16, Paletted, RGBA, NRGBA, RGBA64, NRGBA64 }   // Go's image types

struct Image {
  width: Int
  height: Int
  model: Model
  pix: Bytes             // row after row, Go's layout for the model (stride = width * bytes per pixel)
  palette: List[Int]     // Paletted only: 0xRRGGBBAA (non premultiplied), at most 256
}

struct Config { width: Int, height: Int, model: Model }

enum Compression { Default, None, Speed, Best }          // Go's CompressionLevel

fn decode(data: Bytes, max_pixels: Int = 100000000) -> Image ! PngErr   // Go's png.Decode: the model Go picks for the color type
fn decode_config(data: Bytes) -> Config ! PngErr         // png.DecodeConfig
fn encode(img: Image, level: Compression = Compression.Default) -> Bytes ! PngErr   // png.Encoder.Encode
fn (img: Image) at(x: Int, y: Int) -> Int                // 0xRRGGBBAA non premultiplied, like color.NRGBAModel
fn image(width: Int, height: Int, model: Model) -> Image // zeroed pixels
fn (m: Model) bytes_per_pixel() -> Int
```

`PngErr` is an enum of Go's `FormatError` and `UnsupportedError` with their messages (the
text after `png: invalid format: ` or `png: unsupported feature: `), plus `Truncated` for the
`io.ErrUnexpectedEOF` case. A decoder limit `max_pixels` (default 100,000,000) refuses
larger headers before allocating, as Go's reader allocates from the header.

Not ported: interlaced encode (Go has none), `image.Image` interface values, `color` models
beyond pixel access, EncodeBuffer pools (the encoder allocates its own buffers).

### Fast paths (decision 0276's rule: port Go's, then give a timing row)

- The five filters and Go's `filterPaeth` with its early-out arithmetic (paeth.go), applied
  per row on the byte slice with the stride arithmetic of reader.go, no per pixel calls.
- Go's `cbPaletted`/`cbTCA8` row converters as loops over the row, not per pixel at().
- Inflate through std's compress (already at 0.91 to 0.96 of Go's wall).
- The encoder's per-row filter choice by sum of absolute differences, as Go's writer.go.

### Tests and evidence

- The Go tests that fit (reader_test.go's decode of Go's testdata/pngsuite images, the
  interlace and gamma ones; writer_test.go's round trips; paeth_test.go).
- A Go-generated corpus: PNGs encoded by Go in every color type and size, with the decoded
  pix recorded; Xo decodes them and compares pix, and Xo-encoded images are decoded by Go
  (the generator replays).
- A fuzz differential like Lang 2's (random and mutated PNGs through Go's Decode and Xo's
  decode: same pix or both an error of the same kind), run as a progen-style sweep on host 2.
- A timing row: bench/perf/png against Go's image/png (decode, encode, paeth heavy images).
- Lazy checking and the std index: spec/std.md section, std/changes/png.md, xo-at-a-glance.

## Consequences

- One more std module and a std-coverage row moves from "not planned" to done.
- Pix layout fidelity means Image is not a friendly pixel API; a later decision can add
  helpers (`set`, `crop`, `scale`) on top, or std/jpeg and std/gif can share Image.
- JPEG decode (next) and CBOR reuse Image and the corpus tooling.

## Alternatives

- One RGBA Image for all inputs (convert on decode): simpler, but loses Gray and Paletted
  fidelity and the corpus would compare converted pixels, hiding conversion bugs.
- A `use go` binding to Go's image/png: only the Go backend, and the native backends are the
  default.
- Do nothing: programs keep carrying their own decoders.

## Implementation notes (2026-11-03)

- A header over `max_pixels` is `Format("image too large")`; where Go allocates the image from the header
  and then fails on short data, Xo reads the stream for its errors without allocating.
- Go inflates as it reads rows, so a file with two faults in its pixel data stream (a bad row filter before a
  corrupt block, a bad IDAT checksum before the end of the data) names the first one it reaches. Xo inflates
  the whole buffer, and when that fails inflates again in growing pieces for the rows before the error.
  The corpus and the fuzz compare errors of the pixel data stream by kind only; every other error is
  compared exactly (text included), as are the decoded type, size, pix and palette.
- "too much pixel data" for trailing bytes after the zlib stream inside an IDAT is not reproduced
  (`compress.unzlib` reports no consumed length); a small std/compress addition would close it.
- The fuzz found three ordering differences (a bad IDAT checksum met before the end of short data, the
  checksum of the last IDAT checked before a cut file, the wrapped error text for a bad checksum after all
  rows), all fixed; 36,000 fresh cases on six seeds agree on arm64 and LLVM.
