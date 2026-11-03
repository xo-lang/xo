# Timing rows against Go, part 6 (2026-11-03)

Host 1, `xo bench --ab --rounds=15`, bench/perf/jpeg (four of Go's test images: baseline 4:4:4, progressive 4:2:0, gray, restart interval 2; 100 passes; `rgba` also converts each image to 8 bit RGBA; `none` is the setup) at the change set that ports Go's image/jpeg decode (decision 0467). Decision 0461 asks for at most about 1.05 on every workload: the decoder is 1.03 of Go (with the RGBA conversion 1.07, a little over the 1.05 mark)'s wall after progressive scans were made to decode in place and the quantization tables were kept in natural order and the pixel clamps (IDCT output, YCbCr to RGB) and the sign extension of `receive_extend` were made branch free (1.14 with the IDCT clamp alone; 1.38 before: the per block copy of 64 coefficients in and out was 37% of the progressive image's profile). Per image on the Mac: baseline 4:4:4 and gray at parity, progressive 4:2:0 1.8 (was 2.8), restart interval 1.4.

| bench | workload | Go wall (ms) | Xo wall (ms) | Xo / Go | interval | peak memory |
|---|---|---|---|---|---|---|
| jpeg | decode | 99.4 | 102.7 | 1.033 | 1.027 to 1.045 | 0.598 |
| jpeg | rgba | 113.2 | 121.0 | 1.069 | 1.059 to 1.077 | 0.599 |
| jpeg | none | 0.591 | 0.366 | 0.619 | 0.561 to 0.664 | 1.0 |
