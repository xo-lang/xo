# std/png changes

## 2026-11-03: port: std/png, Go's image/png decode, decode_config and encode

- Kind: port
- Commit: this commit
- Found by: roadmap priority 1 (Xo Lead), decision 0466
- Go reference: image/png (go1.26.1)
- Test: testdata/corpus/png (Go's test images, changed copies of them, Go encoded images; byte exact encode at every level), testdata/corpus/pngfuzz (1,000 seeded files from a structure aware builder, checked against Go's Decode and DecodeConfig), `TestPngFuzz` (fresh seeds), bench/perf/png (twincheck)

New module. `png.decode` gives the image type Go's decoder gives, with Go's pix layout and error
texts; `png.encode` writes the bytes Go's encoder writes. 36,000 fresh fuzz cases (six seeds) agree with Go on
arm64 and LLVM.
