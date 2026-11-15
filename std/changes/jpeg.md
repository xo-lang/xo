# std/jpeg changes

## 2026-11-15: feature: `JpegErr` is an open enum (decision 0649)

- Kind: feature
- Commit: this commit
- Found by: decision 0649 (a new variant of a std error enum broke two SDKs, 0644's `ClientErr.Tls`)
- Go reference: none (Go's errors are values compared with errors.Is; Rust's `#[non_exhaustive]`)
- Test: compiler/check's open enum test; the open_enum golden; xo check over the programs that use the module

`JpegErr` is declared `open`: a `match` on it outside std needs a `_` (or
binding) arm, warning XO0306 in 0.52 and an error after, so that std can
add a variant in a minor release without breaking a build. Matches that
already have `_` are unchanged.

## 2026-11-14: fix: a frame of width or height 0 decodes to an image of 0 by 0

- Kind: fix
- Commit: this push
- Found by: the decision 0616 std/jpeg fuzz (TestJpegFuzz seed 9 case 772, a 150 by 0 frame, on linux/arm64 host 2)
- Go reference: image/jpeg makes its image with SubImage(image.Rect(0, 0, width, height)), and image's SubImage of an empty rectangle returns an image with the zero Rectangle
- Before: decode returned an Image of 150 by 0 (its planes empty), where Go's image is 0 by 0
- Fix: decode's image is 0 by 0 when the frame's width or height is 0, for every model; decode_config still reports the frame's own size, as Go's DecodeConfig does
- Test: std/jpeg_test.xo "a frame of height 0 decodes to an empty image ..." (the fuzz case's bytes); TestJpegFuzz seed 9
- Shipped in: not yet released

## 2026-11-14: perf: a baseline block whose scan wrote no AC term skips the inverse DCT

- Kind: perf
- Commit: this push
- Found by: the decision 0616 std/jpeg review's G1 (a flat 10000x10000 gray file, 1.17 MB, decoded in 1.76 times Go's time) and an XO_PROF profile of it (idct_rows and idct_cols about 42%, the dequantization 10%)
- Go reference: image/jpeg idct's row pass: "If all the AC components are zero, then the IDCT is trivial"
- Before: every baseline block was dequantized (64 multiplies) and run through both passes of the inverse DCT, though a block of a flat area has only its DC term
- Fix: process_sos notes whether a baseline block's scan wrote an AC term; a block with none goes to reconstruct_dc, which computes its one value as the two passes reduce to with seven of eight inputs zero (the DC term dequantized, << 17 and wrapped to 32 bits, then + 524288, wrapped, >> 20) and stores it, so the output is unchanged. A per row or per column zero test inside the passes was tried first and cost photos about 1 to 3%. The flat file decodes in 184 ms, was 393 ms (Go 223 ms; Mac). bench/perf/jpeg on linux/arm64 host 1, --ab against main, 11 rounds: decode within noise (0.994, n=11), rgba 0.986 (0.972 to 0.992, n=6).
- Test: xo test --std-module=jpeg native and llvm; TestStdCorpora jpeg, jpegenc, jpegfuzz, image on go, arm64, llvm (CRCs of each decoded plane and of the RGBA conversion against Go's)
- Shipped in: not yet released

## 2026-11-14: perf: decode stores a block row and an RGBA pixel in one store each, and a progressive image's coefficient store is made in one list again

- Kind: perf
- Commit: this push
- Found by: the stdproven perf run for std/jpeg (linux/arm64 host 1: decode 1.101, rgba 1.146 of Go's wall; the 2026-11-03 hand row's 1.01 and 1.04 were measured another way, as the same run before the security fix is within noise of this one) and XO_PROF profiles of decode and rgba
- Go reference: image/jpeg's reconstructBlock writes each clamped row into the plane slice; color.YCbCrToRGB per pixel into an RGBA slice
- Before: store_block built each 8 pixel row as two words and wrote them with two four byte stores; Image.to_rgba wrote four bytes a pixel one at a time; and the 2026-11-14 security fix's growing coefficient store built a 64 Ki zero chunk and concatenated it on every progressive decode, about 11% of decode, whose images hold far fewer coefficients than the chunk
- Fix: a row is packed into one word, pixel x in byte x (the top byte as a signed byte, so the shift fits Int), and written with one eight byte put_le; a pixel's R, G, B and 255 go out in one four byte write_le (a gray pixel as g * 0x010101); a frame holding at most 64 coefficients per byte of the file (EAGER_COEFS_PER_BYTE; Go's test images hold 2 to 12, the security fix's hostile file about 14,500) gets its whole store at its first write, in one list, and doubling stays for a header asking for more than its data could fill. bench/perf/jpeg on linux/arm64 host 1, --ab against 7c0857749: decode 0.905 (0.891 to 0.923, n=6; 102.5 to 92.8 ms), rgba 0.910 (0.899 to 0.920, n=6; 117.6 to 107.0 ms); the coefficient store step alone decode 0.969 and rgba 0.973
- Test: std/jpeg_test.xo (the hostile files still take the doubling path: Go's errors); TestStdCorpora jpeg, jpegenc, jpegfuzz and image on go, arm64, llvm; xo test --std-module=jpeg native and llvm
- Shipped in: not yet released

## 2026-11-14: security: a frame header asking for a large image allocates only what the scans fill

- Kind: security
- Commit: this push
- Found by: the decision 0616 std/jpeg review (Xo Lang 3): a probe of a frame header asking for 10000x10000 with little scan data behind it
- Go reference: image/jpeg makeImg and processSOS allocate the planes and a progressive image's coefficient blocks from the frame header with make, whose pages the system zeroes on first touch, so a file whose scans fail early costs Go almost nothing
- Before: std/jpeg wrote every plane's zero bytes (Bytes.zeroed, then copied into its builder) and pushed every coefficient of a progressive component one at a time when its first scan began, so the cost followed the header, not the data: Go's video-001.progressive.jpeg (20 KB) with its header changed to 10000x10000 took 1.08 s and 1.5 GB resident before failing (Go: 3 ms, 8.6 MB); a 496 byte baseline file 27 ms and 204 MB (Go: under 1 ms, 5.9 MB); Mac, native
- Fix: the planes are reserved (with_capacity) and written with zeros only up to the block being stored, doubling (ensure_plane), and padded to their size in finish; a progressive image's coefficients are kept in MCU order (every component's blocks of one MCU together, coef_at), so every scan writes in MCU row order, and the store grows by doubling only when a scan writes a coefficient past its end (coef_set, ensure_coef, a 64 Ki zero chunk concatenated), so an end-of-band run touches nothing; a coefficient past its end reads as zero (coef_get), and a block no scan wrote is stored as the inverse DCT of zero computed once (reconstruct_progressive), not dequantized and transformed. Both files above now fail in under 1 ms with 4 MB resident, with Go's errors; a valid 10000x10000 gray file (1.17 MB, Go's encoder) decodes in 460 ms and 105 MB, was 512 ms and 205 MB (Go: 226 ms, 108 MB)
- Test: std/jpeg_test.xo "a large frame whose scan data runs out fails as Go's decoder does" (baseline and progressive, Go's error texts, decode_config 10000x10000) and "max_pixels refuses a frame one pixel past it"; TestStdCorpora jpeg, jpegenc, jpegfuzz on go, arm64, llvm; TestJpegFuzz and TestJpegEncFuzz on arm64 and llvm
- Affected: every backend, 0.38.0 to 0.51.1; bounded by max_pixels (100,000,000 by default), so a cost per decode, not unbounded
- Shipped in: not yet released

## 2026-11-11: refactor: std/jpeg is one file (decisions 0588, 0597)

- Kind: refactor
- Commit: this push
- Found by: decision 0597 (std/native/jpeg.xo imports nothing native)
- Go reference: none (no behavior change)
- Test: TestStdCorpora jpeg, jpegenc, jpegfuzz, image on go, arm64, llvm
- Shipped in: not yet released

std/native/jpeg.xo moved into std/jpeg.xo. The module is generated into
std/rt (std/rt_hosted.txt), since std/image uses it; the Go twin of
JpegErr's display (std/rt/jpeg.go) is gone, the port's is used.

## 2026-11-05: fix: the decode corpora match JpegErr.Encode

- Kind: fix
- Commit: this commit
- Found by: tier 51 (TestStdCorpora jpeg and jpegfuzz: `match on jpeg.JpegErr is not exhaustive: missing Encode(_)`)
- Test: `go test ./internal/stdtest -run 'TestStdCorpora/.*/jpeg'`

The decode corpora (testdata/corpus/jpeg, jpegfuzz, and their generators scripts/stdcorpus/jpeg.go and internal/stdtest/jpegfuzz_test.go) match every variant of `JpegErr`, so they needed the arm for the variant `jpeg.encode` added. **Breaking for callers:** adding `Encode(msg)` to the public enum `JpegErr` (entry below) makes a user's exhaustive `match` on a `JpegErr` fail to compile with XO0301 until it handles `Encode(_)` (or ends in a catch-all arm); Go's `jpeg.FormatError`/`UnsupportedError` are separate types and cannot break this way. The 2026-11-05 feature entry below is the change that did it.

## 2026-11-05: perf: encode skips the quantizer for coefficients that round to zero

- Kind: perf
- Commit: this commit
- Found by: CPU profile of the encode row (`enc_block` 55%)
- Test: testdata/corpus/jpegenc, `TestJpegEncFuzz` (unchanged output), bench/perf/jpeg `encode` and `encrgba` (twincheck)

`jpeg.encode` compares each coefficient with half its divisor first (`abs(a) < d - d/2` rounds to 0, exact) and only divides the rest; the Huffman code and its value bits go out in one emit, and a column index table per MCU replaces a min per pixel. Output is unchanged. bench/perf/jpeg on linux/arm64: encode 1.29 to 0.83 of Go's wall, encrgba 1.37 to 0.90 (bench/results/jpeg-encode-2026-11-05.md).

## 2026-11-05: feature: jpeg.encode, Go's image/jpeg Encode byte for byte

- Kind: feature
- Commit: this commit
- Found by: roadmap (Xo Lead), decision 0510
- Go reference: image/jpeg writer.go and dct.go (go1.26.1)
- Test: testdata/corpus/jpegenc (245 generated cases of every kind at odd sizes and qualities, CRC-32 and length against Go's output, small ones in full), `TestJpegEncFuzz` (fresh seeds), bench/perf/jpeg `encode` and `encrgba` (twincheck)

New `jpeg.encode(img, quality = 75) -> Bytes ! JpegErr`: baseline JPEG, the standard Huffman
tables, quality scaled quantization tables and Go's integer forward DCT, so the output is the
bytes Go writes. Gray is one component; every other input is written 4:2:0 (a YCbCr of any
ratio is resampled as Go does). New `JpegErr.Encode(msg)` carries Go's `jpeg: image is too
large to encode`. Mismatched planes are a fault, as in `png.encode`.

## 2026-11-03: port: std/jpeg, Go's image/jpeg decode and decode_config

- Kind: port
- Commit: this commit
- Found by: roadmap priority 1 (Xo Lead), decision 0467
- Go reference: image/jpeg (go1.26.1)
- Test: testdata/corpus/jpeg (186 cases: Go's test images, changed copies, Go encoded images, error files), testdata/corpus/jpegfuzz (350 seeded files from a segment aware mutator, checked against Go's Decode and DecodeConfig), `TestJpegFuzz` (fresh seeds), bench/perf/jpeg (twincheck)

New module. `jpeg.decode` gives the image type, planes and error texts Go's decoder gives,
including its integer IDCT; `jpeg.decode_config` gives the size and image type. 12,000 fresh
fuzz cases (four seeds) agree with Go on arm64 and LLVM.

## 2026-11-03: perf: progressive scans decode and refine in place

- Kind: perf
- Commit: this commit
- Found by: bench/perf/jpeg per image timing (progressive 4:2:0 was 2.8 of Go's wall)
- Test: testdata/corpus/jpeg, testdata/corpus/jpegfuzz (unchanged output), bench/perf/jpeg

A progressive scan copied each block's 64 coefficients out of the coefficient list and back (37% of the profile); it now reads and writes the list at the block's offset. bench/perf/jpeg decode 1.38 to 1.25 of Go's wall.

## 2026-11-03: perf: quantization tables in natural order

- Kind: perf
- Commit: this commit
- Found by: host profile of the restart interval image (reconstruct_block 63%)
- Test: testdata/corpus/jpeg, testdata/corpus/jpegfuzz (unchanged output), bench/perf/jpeg

The tables are stored in row major order when read, so dequantizing a block walks both in order without the zigzag lookup. bench/perf/jpeg decode 1.25 to 1.22, with `to_rgba` 1.30 to 1.24.

## 2026-11-03: perf: branch free pixel clamp

- Kind: perf
- Commit: this commit
- Found by: host profile of the restart interval image (reconstruct_block)
- Test: testdata/corpus/jpeg, testdata/corpus/jpegfuzz (unchanged output), bench/perf/jpeg

The 0..255 clamp of each IDCT output is done with shifts and masks instead of two compares and a branch (the values are unpredictable). bench/perf/jpeg decode 1.22 to 1.14, with `to_rgba` 1.24 to 1.19.

## 2026-11-03: perf: branch free YCbCr to RGB clamp and receive_extend

- Kind: perf
- Commit: this commit
- Found by: host A/B after the IDCT clamp (the same unpredictable branches)
- Test: testdata/corpus/jpeg, testdata/corpus/jpegfuzz (unchanged output), bench/perf/jpeg

`to_rgba`'s channel clamps and the sign extension of `receive_extend` use shifts and masks instead of branches. bench/perf/jpeg decode 1.14 to 1.03, with `to_rgba` 1.19 to 1.07.

## 2026-11-03: perf: to_rgba chroma index by shifts

- Kind: perf
- Commit: this commit
- Found by: host profile of the rgba workload (image_to_rgba 15% flat)
- Test: testdata/corpus/jpeg, testdata/corpus/jpegfuzz (unchanged output), bench/perf/jpeg

`to_rgba` takes the chroma row and column of each pixel with shifts (the ratios are 1, 2 or 4) instead of two divisions. bench/perf/jpeg: decode 1.01, with `to_rgba` 1.07 to 1.04 of Go's wall; the module meets decision 0461's 1.05 on both workloads.
