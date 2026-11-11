# std/png changes

## 2026-11-11: refactor: std/png is one file (decisions 0588, 0597)

- Kind: refactor
- Commit: this push
- Found by: decision 0597 (std/native/png.xo imports nothing native)
- Go reference: none (no behavior change)
- Test: TestStdCorpora png, pngfuzz, image on go, arm64, llvm
- Shipped in: not yet released

std/native/png.xo moved into std/png.xo. The module is generated into std/rt
(std/rt_hosted.txt), since std/image uses it; the Go twin of PngErr's
display (std/rt/png.go) is gone, the port's is used.

## 2026-11-03: port: std/png, Go's image/png decode, decode_config and encode

- Kind: port
- Commit: this commit
- Found by: roadmap priority 1 (Xo Lead), decision 0466
- Go reference: image/png (go1.26.1)
- Test: testdata/corpus/png (Go's test images, changed copies of them, Go encoded images; byte exact encode at every level), testdata/corpus/pngfuzz (1,000 seeded files from a structure aware builder, checked against Go's Decode and DecodeConfig), `TestPngFuzz` (fresh seeds), bench/perf/png (twincheck)

New module. `png.decode` gives the image type Go's decoder gives, with Go's pix layout and error
texts; `png.encode` writes the bytes Go's encoder writes. 36,000 fresh fuzz cases (six seeds) agree with Go on
arm64 and LLVM.
