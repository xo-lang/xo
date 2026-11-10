# 0578: std/image (a shared Image, color, draw, scalers), std/gif and compress's LZW

Status: Accepted (owner, 2026-11-08, on the lead's recommendation: questions 1 to 4 take the recommendations (A in the same push as std/image, then B one minor later with a Breaking note; port x/image/draw's CatmullRom and NearestNeighbor first; LZW in std/compress; WebP, TIFF, BMP and an `image.Image` interface stay out)). Proposed (Xo Lang 3, 2026-11-08; the lead approved writing it, owner decides, design only: no code until the owner answers)

Implementation (Last verified: 2026-11-09): steps 1 to 5 landed (std/image with draw, draw_mask, fill, floyd_steinberg and scale; std/compress lzw; std/gif); step 6 (png and jpeg aliased onto the shared Image) is open.

## Context

std has `std/png` (decision 0466) and `std/jpeg` (0467, 0510), each with its own `Image`, `Model` and `Config`, copied from Go's
`image/png` and `image/jpeg` with Go's pixel layouts. Nothing connects them:

1. **No shared image.** A program that decodes a JPEG and writes a PNG converts through `jpeg.Image.to_rgba()` bytes and a
   hand made `png.Image`. 0466 says so: "a later decision can add helpers (`set`, `crop`, `scale`) on top, or std/jpeg and std/gif
   can share Image".
2. **No draw or resize.** Go has `image/draw` (`Draw`, `DrawMask`, Src and Over, a Floyd-Steinberg `Drawer`) and, outside
   the standard library, `golang.org/x/image/draw` (the scalers `NearestNeighbor`, `ApproxBiLinear`, `BiLinear`, `CatmullRom`),
   which is what Go programs actually use to make thumbnails. Xo has neither; each port that needs a thumbnail, an avatar, a QR
   code on a background or a chart output writes its own pixel loops (polarbearblog's qrterm draws into a bool grid).
3. **No GIF**, and no LZW: std/compress has gzip, deflate and zlib only. `image/gif` needs `compress/lzw` (Go's, with the GIF
   variant: LSB bit order, a literal width of 2 to 8, variable code size to 12 bits); TIFF and PDF readers would need it too.

maturity-gaps.md's larger standard library is tracked separately; this is the largest Go standard library family with no std
module and no decision (checked 2026-11-08 against std/, spec/std.md and decisions/: image/color, image/draw, image/gif,
image (the interfaces) and compress/lzw are absent; os/user, math/cmplx, testing/fstest and index/suffixarray are also absent
and lower use).

## Decision (proposed)

Add, in this order, each as its own push with a corpus from Go (decision 0276: port Go's code and tests where they fit,
attribution kept, Go's fast paths too, a timing row against Go beside each corpus):

1. **`std/image`**: `struct Image` (the superset of `png.Image` and `jpeg.Image`: `width`, `height`, `model`, `pix`, `palette`,
   and the YCbCr planes and `subsample`), `enum Model` (Gray, Gray16, Paletted, RGBA, NRGBA, RGBA64, NRGBA64, CMYK, YCbCr),
   `struct Config`, `struct Rect {x0, y0, x1, y1}`, `struct Color {r, g, b, a}` (16 bit premultiplied, Go's `color.Color.RGBA()`),
   Go's color model conversions (`Gray`, `NRGBA`, `YCbCr`, `CMYK` and back, the exact integer formulas of image/color), palettes
   (`Plan9`, `WebSafe` from image/color/palette), and the accessors `at(x, y) -> Color`, `set`, `sub_image(r)` (a view where `pix`
   allows it, as Go's `SubImage`), `crop`, `to_rgba`, `to_nrgba`. The pixel layout stays Go's (tight rows, no stride padding
   beyond what `sub_image` needs, so `stride` joins `Image`), so a decoder's `pix` is the same bytes.
2. **`std/image` `draw`** (Go's image/draw): `draw(dst, r, src, sp, op)` and `draw_mask`, with Go's fast paths (RGBA, NRGBA, Gray,
   YCbCr and Uniform sources, Src and Over), and `floyd_steinberg` for Paletted destinations. Timing row: `bench/perf/image`
   Over on RGBA, 1024 by 768, against Go's `draw.Draw`.
3. **Scalers** (x/image/draw, BSD-3, Go authors; not standard library, but the thumbnail primitive): `scale(dst, dr, src, sr,
   kernel, op)` and `transform` with `NearestNeighbor`, `ApproxBiLinear`, `BiLinear` and `CatmullRom` (the kernel based
   scaler with its precomputed weights, and the typed fast paths for RGBA, NRGBA, Gray and YCbCr). The first push is
   NearestNeighbor and CatmullRom (the two ends: fastest and best looking), the other two follow. Timing rows: CatmullRom 1024 by
   768 to 256 by 192 and NearestNeighbor, against Go's x/image/draw in the same harness as the other bench/perf rows.
4. **`std/compress` `lzw`**: Go's compress/lzw as `lzw_encode(data, order, lit_width)` and `lzw_decode(data, order, lit_width,
   max_size)` plus a streaming `Compressor`/`Decompressor` pair like the existing formats (decision 0276's Go port rule: Go's
   reader and writer, including the Go quirks of early clear codes and `max_size`).
5. **`std/gif`** (Go's image/gif): `decode`, `decode_all` (every frame with its delay, disposal, the loop count and the
   background), `decode_config`, `encode`, `encode_all`; frames are `image.Image` of model Paletted, an encode of a non palette image
   quantizes with Go's `Plan9` palette and Floyd-Steinberg (as Go's `gif.Encode`), LZW from step 4. Go's errors, `max_pixels`
   like png and jpeg.
6. **png and jpeg onto the shared Image** (see the next section).

### How png and jpeg move without breaking

0567 (compatibility policy) requires a rewrite and a changelog line for every break, and deprecation before removal. Three
options, in order of recommendation:

- **A (recommended first): conversions, no change.** `png.Image`, `jpeg.Image` stay as they are; std/image adds
  `from_png(png.Image) -> Image`, `to_png`, `from_jpeg`, `to_jpeg` (a `Bytes` is a counted value, so `pix` moves without a copy).
  Nothing breaks, programs adopt the shared type at their own pace.
- **B: aliases.** Xo has type aliases (core 2.8). `type Image = image.Image` in std/png and std/jpeg keeps `png.Image{...}`
  literals and `png.decode(...) -> png.Image` working if the new fields have defaults. The break is the `Model` enums:
  `png.Model` and `jpeg.Model` become the one `image.Model` with nine variants, so a user's exhaustive `match` over
  `png.Model` stops being exhaustive (a compile error, XO0305 shape), fixed by the `_ =>` arm; per 0567 that ships as a
  Breaking changelog entry with a rewrite (`xo fix`), after A has been out one minor release with the deprecation of the
  separate types.
- **C: separate forever** with conversions only. Rejected: the duplication is what this decision removes.

## Questions for the owner

1. **A then B, or A only?** Recommendation: A in the same push as std/image, B one minor later with a Breaking note.
2. **Port x/image/draw's scalers although it is outside the standard library?** Recommendation: yes (CatmullRom and
   NearestNeighbor first); resize is the reason most programs reach for the package, and decision 0276's "std follows Go's
   standard library" can read "and the one x/ package the standard library's image types need".
3. **LZW placement**: functions in std/compress (recommended: one place for compression formats) or its own `std/lzw`.
4. **Scope limits**: WebP, TIFF and BMP (x/image) are left out; `image.Image` as an interface is not ported (the one `Image` struct
   with a `Model` replaces it, as 0466 decided), so a user defined image type is not possible.

## Consequences

- One module family that every image port can build on; png and jpeg stop being islands.
- Cost: about 3,000 lines of std (color 600, draw 700, scalers 900, lzw 300, gif 600) with corpora from Go (random images
  through each Go function, the Go library's own tests where they fit, gif files from Go's encoder), a fuzz differential for gif
  as for png (`internal/stdtest/pngfuzz_test.go`), and `std/changes` entries per module; spec/std.md gains a section.
- Risk: draw and the scalers are large and branchy in Go (typed fast paths per source type); the corpus per source and
  destination model pair and the timing rows are the proof. `bench/perf/image` is new.

## Alternatives

- **One RGBA only** (convert on decode): loses Gray, Paletted and CMYK fidelity, hides conversion bugs from the corpora
  (0466's rejection stands).
- **Leave resize to programs**: each port repeats a slow, subtly different pixel loop.
- **`use go` bindings to image/draw**: only the Go backend; the native backends are the default.
- **A native only library with its own API** (not Go's): contradicts the std rule of following Go's standard library so ports
  of Go code stay mechanical.
