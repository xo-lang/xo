# std/jpeg changes

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
