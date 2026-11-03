# Timing rows against Go, part 6 (2026-11-03)

Host 1, `xo bench --ab --rounds=15`, bench/perf/jpeg (four of Go's test images: baseline 4:4:4, progressive 4:2:0, gray, restart interval 2; 100 passes; `rgba` also converts each image to 8 bit RGBA; `none` is the setup) at the change set that ports Go's image/jpeg decode (decision 0467). Not Proven (decision 0461: at most about 1.05): the decoder is 1.14 of Go's wall after progressive scans were made to decode in place and the quantization tables were kept in natural order and the pixel clamp was made branch free (1.38 before: the per block copy of 64 coefficients in and out was 37% of the progressive image's profile). Per image on the Mac: baseline 4:4:4 and gray at parity, progressive 4:2:0 1.8 (was 2.8), restart interval 1.4.

| bench | workload | Go wall (ms) | Xo wall (ms) | Xo / Go | interval | peak memory |
|---|---|---|---|---|---|---|
| jpeg | decode | 99.98 | 114.1 | 1.142 | 1.118 to 1.163 | 0.591 |
| jpeg | rgba | 113.9 | 135.0 | 1.185 | 1.171 to 1.205 | 0.587 |
| jpeg | none | 0.719 | 0.425 | 0.592 | 0.558 to 0.628 | 1.0 |
