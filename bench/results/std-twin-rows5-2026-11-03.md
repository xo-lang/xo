# Timing rows against Go, part 5 (2026-11-03)

Host 1, `xo bench --ab --rounds=15`, bench/perf/png (four 256x256 images: opaque RGBA, NRGBA with alpha, Gray, 16 color Paletted; 8 passes; the setup builds and encodes the images once, so `none` is that setup) at the change set that ports Go's image/png (decision 0466) and makes compress's Adler-32 take the modulus once per 5552 bytes (`png decode` was 1.22 before it: `compress.adler32` was 29% of the profile). The same change set's bench/perf/compress rows are in std-twin-rows-2026-11-03.md (inflate 0.91, gunzip 0.92 after it).

| bench | workload | Go wall (ms) | Xo wall (ms) | Xo / Go | interval | peak memory |
|---|---|---|---|---|---|---|
| png | encode | 347.6 | 327.3 | 0.941 | 0.939 to 0.949 | 0.962 |
| png | decode | 62.03 | 64.46 | 1.039 | 1.03 to 1.048 | 1.227 |
| png | none | 40.05 | 38.53 | 0.962 | 0.948 to 0.971 | 1.0 |
