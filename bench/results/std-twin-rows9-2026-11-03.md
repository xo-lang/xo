# Timing rows against Go, part 9 (2026-11-03)

Linux/arm64 offload host, `xo bench --ab` style runs (scratchpad twin-ab2: 15 rounds, Go and Xo alternated, caches off) of the bench/perf twins added with this batch: hash (crc64, crc64e, adler32 and the existing crc32, crc32c, fnv rows), csv (parse, format and the new streaming `reader`), bcrypt, charset and bufio. `none` is the setup alone. Go is the standard library's package, except bcrypt (golang.org/x/crypto/bcrypt v0.58.0) and charset (golang.org/x/text encoding/unicode and charmap v0.43.0). Rows are as measured, not selected: ADR 0461 counts a row Proven at 1.05 of Go's wall or less.

| bench | workload | Go wall (ms) | Xo wall (ms) | Xo / Go | interval | peak memory |
|---|---|---|---|---|---|---|
| hash | none | 8.266 | 5.441 | 0.658 | 0.555-0.95 | 0.824 |
| hash | crc32 | 34.92 | 38.11 | 1.091 | 0.988-1.193 | 0.872 |
| hash | crc32c | 36.98 | 15.63 | 0.423 | 0.372-0.547 | 0.866 |
| hash | crc64 | 148.9 | 155.2 | 1.043 | 0.98-1.198 | 1.296 |
| hash | crc64e | 152.2 | 167.3 | 1.099 | 1.013-1.229 | 0.826 |
| hash | adler32 | 94.94 | 99.53 | 1.048 | 0.959-1.168 | 0.825 |
| hash | fnv32a | 63.33 | 64.44 | 1.018 | 0.775-1.18 | 0.826 |
| hash | fnv64a | 65.66 | 66.83 | 1.018 | 0.916-1.176 | 0.822 |
| hash | fnv128a | 502.9 | 395.0 | 0.785 | 0.689-0.945 | 0.381 |
| csv | none | 9.989 | 10.27 | 1.028 | 0.674-1.427 | 0.964 |
| csv | parse | 183.7 | 120.8 | 0.658 | 0.462-0.845 | 0.461 |
| csv | format | 150.5 | 103.0 | 0.685 | 0.624-0.818 | 0.382 |
| csv | reader | 177.7 | 122.3 | 0.688 | 0.571-0.984 | 0.715 |
| bcrypt | none | 0.706 | 0.446 | 0.631 | 0.584-0.733 | 1.178 |
| bcrypt | hash | 1219 | 1754 | 1.439 | 1.32-1.604 | 0.806 |
| bcrypt | compare | 1314 | 2034 | 1.548 | 1.406-1.681 | 0.806 |
| charset | none | 20.93 | 16.0 | 0.765 | 0.723-0.925 | 0.557 |
| charset | utf16_encode | 125.6 | 54.1 | 0.431 | 0.395-0.651 | 0.21 |
| charset | utf16_decode | 95.27 | 70.35 | 0.738 | 0.698-0.789 | 0.479 |
| charset | w1252_decode | 92.39 | 55.21 | 0.598 | 0.514-0.693 | 0.256 |
| charset | w1252_encode | 122.3 | 44.98 | 0.368 | 0.333-0.605 | 0.329 |
| bufio | none | 64.79 | 43.21 | 0.667 | 0.465-0.744 | 0.769 |
| bufio | lines | 101.1 | 56.11 | 0.555 | 0.403-0.859 | 0.436 |
| bufio | until | 104.1 | 75.84 | 0.729 | 0.633-0.882 | 0.474 |
| bufio | exact | 81.34 | 54.65 | 0.672 | 0.604-0.821 | 0.768 |

Reading:

- hash: crc64 (1.04) and adler32 (1.05) are within noise of Go; crc64e 1.10 has an interval of 1.01 to 1.23 and uses the same slicing-by-8 tables and loop as crc64 (Go's crc64 also uses slicing-by-8 for both tables), so the gap between the two rows is noise; it is re-measured with the next batch. crc32c is 0.42 (Go's crc32c has no arm64 assembly path here), fnv128a 0.79.
- csv: parse 0.66, format 0.69 and the streaming `reader` over a File 0.69 of Go's wall at 0.46 to 0.72 of its peak memory.
- bufio: lines 0.56, until 0.73 and exact 0.67 of Go's bufio.Reader over 1460 byte chunks (the `none` row, the building of the 8 MiB test data, is 0.67).
- bcrypt: hash 1.44 and compare 1.55 of x/crypto's. Not Proven. Tried 2026-11-03: unchecked S box and P reads (a relaxed slot load), F written out in the round loop, and wrapping adds; each measured 1.42 to 1.58 on the Mac, so the 4 bounds branches and 2 overflow-checked adds per F in the arm64 code are not the cost (a re-run on the host with the unchecked reads gave 2.0 on a loaded host and was dropped with the change). The 32 bit masks, the byte extraction as shift and mask against Go's byte indexing of a uint32 array, and 8 byte words (8 KiB of state against Go's 4 KiB) are what remains; next experiment: a Bytes-backed 32 bit state read with rt.bytes_word.
- charset: re-measured 2026-11-03 on the text-like twin data (9 in 10 characters ASCII, as text is) after the UTF-8 write was inlined: UTF-16 encode 0.43, UTF-16 decode 0.74, windows-1252 decode 0.60 and encode 0.37 of x/text's wall. The first run, on a twin whose text was 50% non-ASCII, had decode at 1.8 to 2.1 of Go's; the data, not only the code, differed, so those rows are replaced and not comparable with these.
