# Timing rows against Go, part 6 (2026-11-03)

Host 1, `xo bench --ab --rounds=15`, bench/perf/jpeg (four of Go's test images: baseline 4:4:4, progressive 4:2:0, gray, restart interval 2; 100 passes; `rgba` also converts each image to 8 bit RGBA; `none` is the setup) at the change set that ports Go's image/jpeg decode (decision 0467). Not Proven (decision 0461: at most about 1.05): the decoder is 1.22 of Go's wall after progressive scans were made to decode in place and the quantization tables were kept in natural order (1.38 before: the per block copy of 64 coefficients in and out was 37% of the progressive image's profile). Per image on the Mac: baseline 4:4:4 and gray at parity, progressive 4:2:0 1.8 (was 2.8), restart interval 1.4.

| bench | workload | Go wall (ms) | Xo wall (ms) | Xo / Go | interval | peak memory |
|---|---|---|---|---|---|---|
| jpeg | decode | 100.3 | 122.3 | 1.219 | 1.211 to 1.235 | 0.591 |
| jpeg | rgba | 114.0 | 141.6 | 1.242 | 1.23 to 1.268 | 0.591 |
| jpeg | none | 0.592 | 0.362 | 0.612 | 0.577 to 0.654 | 1.0 |
