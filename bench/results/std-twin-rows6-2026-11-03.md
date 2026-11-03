# Timing rows against Go, part 6 (2026-11-03)

Host 1, `xo bench --ab --rounds=15`, bench/perf/jpeg (four of Go's test images: baseline 4:4:4, progressive 4:2:0, gray, restart interval 2; 100 passes; `rgba` also converts each image to 8 bit RGBA; `none` is the setup) at the change set that ports Go's image/jpeg decode (decision 0467). Not Proven (decision 0461: at most about 1.05): the decoder is 1.38 of Go's wall; the profile is Huffman decode and reconstruct (dequantize, integer IDCT, color store), no one function over a third.

| bench | workload | Go wall (ms) | Xo wall (ms) | Xo / Go | interval | peak memory |
|---|---|---|---|---|---|---|
| jpeg | decode | 100.6 | 139.0 | 1.382 | 1.367 to 1.408 | 0.597 |
| jpeg | rgba | 113.8 | 160.4 | 1.409 | 1.39 to 1.436 | 0.734 |
| jpeg | none | 0.619 | 0.363 | 0.586 | 0.547 to 0.624 | 1.0 |
