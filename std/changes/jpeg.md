# std/jpeg changes

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
