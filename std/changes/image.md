# std/image changes

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
