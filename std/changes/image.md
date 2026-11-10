# std/image changes

## 2026-11-09: perf: Image.draw row fast paths for an RGBA destination (decision 0578 step 2)

- Kind: perf
- Commit: this push
- Found by: bench/perf/image (new, this push): draw of 1024 by 768 images, 5 passes, against Go's draw.Draw
- Go reference: image/draw drawCopyOver, drawCopySrc, drawNRGBAOver, drawNRGBASrc, drawGray
- Test: the 252 draw cases of testdata/corpus/image unchanged and passing; twincheck bench/perf/image: 5 workloads, 0 mismatches
- Timing: `go run ./scripts/twincheck -time 3 bench/perf/image` on the Mac (a survey cut, medians, Xo over Go, setup included; the setup alone is 0.88): before the fast paths over 2.72, src 2.56, gray 2.95; after over 1.50, src 1.50, gray 1.79, copy (RGBA Src, Go's row copy) 0.95
- Shipped in: not yet released

For an RGBA destination with an RGBA, NRGBA or Gray source, `Image.draw` now builds the new samples front to back from the source bytes (one append per pixel, one slice copy per
row for an RGBA Src copy) instead of reading each pixel through `at()`. Output unchanged. The remaining gap to Go is the per pixel divide-and-append; the other models still use the generic path.

## 2026-11-09: feature: std/image draw, Image.draw and Op (decision 0578 step 2, part 1)

- Kind: feature
- Commit: this push
- Found by: decision 0578 (owner accepted 2026-11-08)
- Go reference: image/draw Draw (clip, drawRGBA and its fast paths, the RGBA64 fallback, NRGBA and NRGBA64 copy)
- Test: testdata/corpus/image: 252 draws from Go (seven destination models, nine source models including YCbCr, Over and Src, two rectangles with source offsets and clipping), the destination pixels compared byte for byte, on arm64, LLVM and the Go backend
- Timing: the samples are copied once per call and each pixel is read through at(); typed row fast paths and the timing row against Go's draw.Draw come next
- Shipped in: not yet released

`Image.draw(r, src, sp_x, sp_y, op)` is Go's `draw.Draw` with a nil mask for every destination model but Paletted and YCbCr (to come: Paletted with `floyd_steinberg`, masks, a uniform source).

## 2026-11-09: feature: std/image YCbCr (planes, at, sub_image) and from_jpeg, Image.to_jpeg (decision 0578 step 1)

- Kind: feature
- Commit: this push
- Found by: decision 0578 (owner accepted 2026-11-08)
- Go reference: image (YCbCr At and SubImage for the six subsampling ratios), image/color (YCbCr.RGBA)
- Test: testdata/corpus/image: 3 random 5 by 4 images for each of the six ratios, probed whole and through three sub images (inside, clipped, at the edge) with At from Go, on arm64, LLVM and the Go backend; from_jpeg and to_jpeg keep samples and subsampling
- Timing: at() is per pixel; the YCbCr fast paths come with draw (step 2)
- Shipped in: not yet released

`Image` gains the planes `y`, `cb`, `cr`, their strides `y_stride`, `c_stride` and `subsample` (std/jpeg's enum) for the YCbCr model. `at` and `sub_image` handle YCbCr;
`crop` returns the sub image for YCbCr (planes are not packed yet). `image.from_jpeg(j)` and `Image.to_jpeg()` convert to and from `jpeg.Image`
(`jpeg.Image` is unchanged, option A of decision 0578).

## 2026-11-09: feature: std/image from_png and Image.to_png (decision 0578 step 1, option A)

- Kind: feature
- Commit: this push
- Found by: decision 0578 (owner accepted 2026-11-08)
- Go reference: none (a mapping between two Xo types; PNG's layouts are Go's)
- Test: testdata/corpus/image: a Paletted PNG image round trips with its pixels and palette; a Gray sub image is cropped by to_png; CMYK gives None
- Timing: the pixels move, no copy (a Bytes is a counted value); a sub image is cropped, which copies the rows
- Shipped in: not yet released

`image.from_png(p)` turns a decoded `png.Image` into an `image.Image` and `Image.to_png()` back (None for CMYK and YCbCr, which PNG has no layout for).
`png.Image` itself is unchanged, so nothing breaks (decision 0578, option A).

## 2026-11-09: feature: std/image Image.to_rgba and to_nrgba (decision 0578 step 1)

- Kind: feature
- Commit: this push
- Found by: decision 0578 (owner accepted 2026-11-08)
- Go reference: image/color (RGBAModel, NRGBAModel) applied pixel by pixel, as draw.Draw with Src does
- Test: testdata/corpus/image: 5 rounds of each of the 8 models, both conversions compared with Go's pix (NRGBA samples and palette entries are returned as they are, as Go's NRGBAModel does), on arm64, LLVM and the Go backend
- Timing: one at() per pixel; no Go-package timing row yet (the draw step of 0578 carries the fast paths)
- Shipped in: not yet released

## 2026-11-09: feature: std/image Image.sub_image and crop, Image.min_x/min_y (decision 0578 step 1)

- Kind: feature
- Commit: this push
- Found by: decision 0578 (owner accepted 2026-11-08)
- Go reference: image (SubImage of Gray, Gray16, Paletted, RGBA, NRGBA, RGBA64, NRGBA64, CMYK)
- Test: testdata/corpus/image: 5 rounds of each of the 8 models and three rectangles (inside, clipped, wholly outside): size, corner, stride, pix and At over the rectangle and a pixel around it, from Go, on arm64, LLVM and the Go backend; crop against the sub image's At
- Timing: sub_image shares the parent's pixels (a Bytes slice is a view); crop copies the rows
- Shipped in: not yet released

`Image` gains `min_x` and `min_y` (Go's `Bounds().Min`, 0 for a decoded image); `at` and `set` take Go's absolute coordinates. `sub_image(r)` clips r to
the image and keeps the parent's pixels and stride; `crop(r)` packs the part tightly. Additive: existing `Image` literals are unchanged.

## 2026-11-09: feature: std/image Image.set (decision 0578 step 1)

- Kind: feature
- Commit: this push
- Found by: decision 0578 (owner accepted 2026-11-08)
- Go reference: image (Set of Gray, Gray16, Paletted, RGBA, NRGBA, RGBA64, NRGBA64, CMYK), color.Palette.Index
- Test: testdata/corpus/image: 8 rounds of each of the 8 models, four colors set (one outside the image, opaque and transparent ones included), the pix compared with Go's, on arm64, LLVM and the Go backend
- Timing: a set copies the image (Bytes is immutable), so it is linear in the image size; bulk writers build their own buffers (the draw step of 0578)
- Shipped in: not yet released

`Image.set(x, y, c)` stores a color as Go's model conversion does (Paletted: Go's nearest entry by `Palette.Index`); a point outside the image is ignored.

## 2026-11-09: feature: std/image Image.at, web_safe and plan9 (decision 0578 step 1)

- Kind: feature
- Commit: this push
- Found by: decision 0578 (owner accepted 2026-11-08)
- Go reference: image (At of Gray, Gray16, Paletted, RGBA, NRGBA, RGBA64, NRGBA64, CMYK), image/color/palette (WebSafe, Plan9)
- Test: testdata/corpus/image: 6 images of each of the 8 models, probed inside and one pixel outside on every side, and both palettes in full, from Go, on arm64, LLVM and the Go backend
- Timing: scalar integer arithmetic on one pixel; no Go-package timing row yet (the draw step of 0578 is the first bulk user)
- Shipped in: not yet released

`Image.at(x, y)` returns the 16 bit premultiplied color Go's `At(x, y).RGBA()` gives, including Go's values outside the image. `web_safe()` and
`plan9()` return the palettes as `0xRRGGBBAA`.

## 2026-11-09: feature: std/image, Go's color conversions (decision 0578 step 1)

- Kind: feature
- Commit: this push
- Found by: decision 0578 (owner accepted 2026-11-08)
- Go reference: image/color (GrayModel, Gray16Model, NRGBAModel, RGBToYCbCr, YCbCrToRGB, RGBToCMYK, CMYKToRGB, NRGBA.RGBA())
- Test: testdata/corpus/image (about 1,800 Go results: premultiplied edge and random colors, 8 bit triples with the YCbCr clamps) on arm64, LLVM and the Go backend
- Timing: pure integer arithmetic on scalars with no allocation; no Go-package timing row yet (the first user is the draw step of 0578)
- Shipped in: not yet released

New module std/image: `Rect`, `Color`, `Model` (the nine pixel models of std/png and std/jpeg), `Image`, and the conversions listed in spec/std.md 75.
