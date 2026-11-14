# std/png changes

## 2026-11-14: security: a header too large for an Int is refused as dimension overflow, not overflowed

- Kind: security
- Commit: this push
- Found by: the decision 0616 security review of std/png (std/evidence/png/review.md F1)
- Go reference: image/png reader.go parseIHDR's two "dimension overflow" checks and readImagePass's row size check (go1.26.1)
- Before: with max_pixels at or above about 2^60 (no limit), decode summed the packed rows in an Int that overflowed: a 2147483647 x 2147483647 16 bit RGBA header faulted (exit 70) where Go answers dimension overflow; Go's pixel count check alone is not enough, since (2^30+1) x (2^30-1) at 8 bytes a pixel still overflows the row sum. decode_config, which passes no limit, gave an image size for the headers Go refuses. The default max_pixels (100,000,000) refuses all of these first as image too large
- Fix: parse_ihdr refuses a pixel count whose 8 bytes a pixel overflow an Int, where Go does (before the bit depth and color type), with Go's error; decode also refuses a header whose rows and filter bytes come past 2^62 (an interlaced image's passes stay under twice that), where an Int could no longer hold the image
- Test: corpus png (scripts/stdcorpus png.go): "a header too large for an Int is refused, not overflowed", nine header only files with no limit, decode_config against Go's DecodeConfig exactly and decode against Go's Decode or dimension overflow, on arm64, LLVM and the Go backend
- Affected: png.decode with max_pixels at or above about 2^60, and png.decode_config, on every backend, 0.38.0 to 0.51.1
- Shipped in: not yet released

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

## 2026-11-11: fix: Image.at of an NRGBA64 image is Go's NRGBAModel conversion

- Kind: fix
- Commit: this commit
- Found by: the resolved std API coverage check (TestStdAPICovered): `Image.at` had no test; the new corpus cases found the difference
- Go reference: image/color NRGBAModel and NRGBA64.RGBA (go1.26.1)
- Test: testdata/corpus/png "at reads Go's pixel, as an NRGBA word" (each corner and a seeded point of 160 Go encoded images)
- Shipped in: not yet released

`at` took the high byte of each 16 bit channel. Go's NRGBAModel goes through `RGBA()`, which premultiplies by alpha (`c * a / 0xffff`), and divides by alpha again, so a channel of a pixel that is not opaque can be one lower. `at` now does the same.
