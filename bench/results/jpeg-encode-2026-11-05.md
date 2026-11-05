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
| Huffman code and value bits written by one emit | 1.108 (1.043-1.184) | 1.176 (1.110-1.247) |
| per MCU column index table instead of a min per pixel | 1.065 (1.039-1.164) | 1.163 (1.111-1.260) |
| RGB to YCbCr written inline (no tuple) | 1.104 (1.037-1.160) | 1.184 (1.104-1.275) |
| coefficients below half a divisor skip the multiply (abs(a) < d - d/2 rounds to 0, exact) | 0.829 (0.799-0.862) | 0.897 (0.877-0.919) |

The inline step is within noise of the row above (the compiler already inlines `rgb_ycc`) and was dropped.
With the zero test Xo is 10% to 17% faster than Go's Encode on both rows (`--ab` verdict better,
intervals clear of 1.0). CPU profile (XO_PROF, 1000 Hz) of the
first port: `enc_block` 55% (the quantizing loop over 63 coefficients), `encode` 19% (the
per pixel gather), `fdct` 11%, `enc_emit` 8%. Most coefficients of a quantized block are zero, which is why a compare beats a division.
