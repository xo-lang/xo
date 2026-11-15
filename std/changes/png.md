# std/png changes

## 2026-11-15: feature: `PngErr` is an open enum (decision 0649)

- Kind: feature
- Commit: this commit
- Found by: decision 0649 (a new variant of a std error enum broke two SDKs, 0644's `ClientErr.Tls`)
- Go reference: none (Go's errors are values compared with errors.Is; Rust's `#[non_exhaustive]`)
- Test: compiler/check's open enum test; the open_enum golden; xo check over the programs that use the module

`PngErr` is declared `open`: a `match` on it outside std needs a `_` (or
binding) arm, warning XO0306 in 0.52 and an error after, so that std can
add a variant in a minor release without breaking a build. Matches that
already have `_` are unchanged.

## 2026-11-15: fix: one byte past the image follows Go's flush points

- Kind: fix
- Commit: this push
- Found by: the decision 0616 long fuzz of std/png (TestPngFuzz seed 190, case 2549, on linux/arm64 host 2), after the seed 29 fix
- Go reference: compress/flate inflate.go's readFlush calls (an empty stored block, the final block's end, a full window) and Read, which returns a flush's last byte with the error only for the error's own flush (go1.26.1)
- Before: one byte past the image always took the stream's error; a 30x6 paletted image whose IDAT inflates 187 of 186 bytes, then an empty stored block, then a cut, with a bad chunk checksum, gave "invalid checksum" where Go gives "too much pixel data"
- Fix: go_clean_flush walks the deflate stream as Go reads it and gives the output count at its last clean flush; one byte past the image is too much pixel data when that flush already holds it, and the stream's error otherwise
- Test: corpus png (scripts/stdcorpus png.go): the fuzz input as a fixed case after seed 29's; TestPngFuzz seeds 2 to 200 in the evidence rerun
- Affected: png.decode of a stream that inflates exactly one byte past the image and then fails, on native backends
- Shipped in: not yet released

## 2026-11-15: fix: one byte past the image and then a stream error gives the error, as Go does

- Kind: fix
- Commit: this push
- Found by: the decision 0616 long fuzz of std/png (TestPngFuzz seed 29, case 4393, on linux/arm64 host 2)
- Go reference: image/png reader.go decode's one byte check read after the last row, and compress/flate's Read, which hands over the output of the step that fails with the error once that output is read (go1.26.1)
- Before: decode answered too much pixel data whenever more than the image inflated before a stream error; a 30x14 NRGBA image whose IDAT inflates 1695 of 1694 bytes in a block the data cuts, with a bad chunk checksum, gave "too much pixel data" where Go gives "png: invalid format: invalid checksum"
- Fix: decode_image answers too much pixel data when two or more bytes past the image inflate before the error (or the one byte ends a 32 KB window, which Go hands over with no error); one byte takes the stream's error, as Go's check read does; the inflate limits are the image plus two bytes
- Test: corpus png (scripts/stdcorpus png.go): the fuzz input as a fixed case after seed 20's; TestPngFuzz seeds 2 to 29 in the evidence rerun
- Affected: png.decode of a stream that inflates exactly one byte past the image and then fails, on native backends
- Shipped in: not yet released

## 2026-11-15: fix: an IDAT stream with bytes past the image and then a stream error gives too much pixel data, as Go does

- Kind: fix
- Commit: this push
- Found by: the decision 0616 long fuzz of std/png (TestPngFuzz seed 20, case 4728, on linux/arm64 host 2)
- Go reference: image/png reader.go decode's check read after the last row (n != 0 is "too much pixel data" before the stream's error or the chunk's checksum) (go1.26.1)
- Before: decode inflated the stream again for the rows before a flate error in steps of at least 64 bytes and dropped the output of the step that failed, and with the image's bytes present it answered the stream's error; a 27x15 paletted image whose one 13 byte IDAT inflates 79 of 75 bytes and then cuts a stored block, with a bad chunk checksum, gave "invalid checksum" where Go gives "too much pixel data"
- Fix: partial_inflate feeds the failing step again a byte at a time from a fresh decompressor, keeping the output before the error; decode_image answers too much pixel data when more than the image inflates before the error
- Test: corpus png (scripts/stdcorpus png.go): the fuzz input as a fixed case after the seeded ones, on arm64, LLVM and the Go backend; TestPngFuzz seed 20 (5000 cases) on arm64 and LLVM
- Affected: png.decode of a stream that inflates past the image and then fails, on native backends
- Shipped in: not yet released

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
