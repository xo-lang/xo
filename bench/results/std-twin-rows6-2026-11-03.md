# Timing rows against Go, part 6 (2026-11-03)

Host 1, `xo bench --ab --rounds=15`, bench/perf/jpeg (four of Go's test images: baseline 4:4:4, progressive 4:2:0, gray, restart interval 2; 100 passes; `rgba` also converts each image to 8 bit RGBA; `none` is the setup) at the change set that ports Go's image/jpeg decode (decision 0467). Decision 0461 asks for at most about 1.05 on every workload: the decoder is 1.01 of Go's wall (with the RGBA conversion 1.04: Proven) after progressive scans were made to decode in place and the quantization tables were kept in natural order and the pixel clamps (IDCT output, YCbCr to RGB) and the sign extension of `receive_extend` were made branch free, and `to_rgba` takes its chroma index by shifts (1.07 before the shifts; 1.14 with the IDCT clamp alone; 1.38 before: the per block copy of 64 coefficients in and out was 37% of the progressive image's profile). Per image on the Mac: baseline 4:4:4 and gray at parity, progressive 4:2:0 1.8 (was 2.8), restart interval 1.4.

| bench | workload | Go wall (ms) | Xo wall (ms) | Xo / Go | interval | peak memory |
|---|---|---|---|---|---|---|
| jpeg | decode | 101.2 | 101.8 | 1.007 | 0.994 to 1.024 | 0.481 |
| jpeg | rgba | 113.7 | 118.1 | 1.039 | 1.03 to 1.05 | 0.488 |
| jpeg | none | 0.714 | 0.42 | 0.588 | 0.572 to 0.637 | 1.22 |
