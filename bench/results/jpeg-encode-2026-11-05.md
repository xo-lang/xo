# jpeg.encode against Go's image/jpeg Encode (2026-11-05)

Decision 0510. `bench/perf/jpeg` workloads `encode` (the four test images of the decode row,
decoded once, then encoded 100 times each at quality 75; the YCbCr ones are written 4:2:0
as Go does) and `encrgba` (the same images as RGBA). Host: linux/arm64 offload host 2,
`xo bench --ab` against the Go twin (twin.go.txt), 7 rounds. Both sides print the same checksum.

Output: byte for byte Go's (testdata/corpus/jpegenc, 245 cases, and `TestJpegEncFuzz`, 5,500
fresh cases, on arm64, LLVM and the Go backend).

| step | encode (Xo/Go wall) | encrgba (Xo/Go wall) |
| --- | --- | --- |
| first port (64 bit divisions in the quantizer) | 1.292 (1.223-1.338) | 1.369 (1.297-1.455) |
| quantizer divides by multiplying with ceil(2^31/d) (exact below 2^20; checked for all 255 divisors) | 1.199 (1.141-1.289) | 1.228 (1.175-1.290) |
| chroma plane offset by shifts, not divisions; model and planes hoisted out of the pixel loop | 1.158 (1.106-1.217) | 1.242 (1.157-1.318) |

Xo is still 16% to 24% slower than Go here: not a win. CPU profile (XO_PROF, 1000 Hz) of the
first port: `enc_block` 55% (the quantizing loop over 63 coefficients), `encode` 19% (the
per pixel gather), `fdct` 11%, `enc_emit` 8%. Next candidates: skip the all zero tail of a
block before the multiply loop, gather a block row at a time without per pixel `min`, and a
bit buffer written to a preallocated list instead of `BytesBuilder.write_byte`.
