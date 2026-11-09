# std/image changes

## 2026-11-09: feature: std/image, Go's color conversions (decision 0578 step 1)

- Kind: feature
- Commit: this push
- Found by: decision 0578 (owner accepted 2026-11-08)
- Go reference: image/color (GrayModel, Gray16Model, NRGBAModel, RGBToYCbCr, YCbCrToRGB, RGBToCMYK, CMYKToRGB, NRGBA.RGBA())
- Test: testdata/corpus/image (about 1,800 Go results: premultiplied edge and random colors, 8 bit triples with the YCbCr clamps) on arm64, LLVM and the Go backend
- Timing: pure integer arithmetic on scalars with no allocation; no Go-package timing row yet (the first user is the draw step of 0578)
- Shipped in: not yet released

New module std/image: `Rect`, `Color`, `Model` (the nine pixel models of std/png and std/jpeg), `Image`, and the conversions listed in spec/std.md 75.
