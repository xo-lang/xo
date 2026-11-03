# Timing rows against Go, part 9 (2026-11-03)

Linux/arm64 offload host,  style runs (scratchpad twin-ab2: 15 rounds, Go and Xo alternated, caches off) of the bench/perf twins added with this batch: hash (crc64, crc64e, adler32 and the existing crc32, crc32c, fnv rows), csv (parse, format and the new streaming ), bcrypt, charset and bufio.  is the setup alone. Go is the standard library's package, except bcrypt (golang.org/x/crypto/bcrypt v0.58.0) and charset (golang.org/x/text encoding/unicode and charmap v0.43.0). Rows are as measured, not selected: ADR 0461 counts a row Proven at 1.05 of Go's wall or less.

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
| charset | none | 8.903 | 10.11 | 1.135 | 1.099-1.154 | 0.544 |
| charset | utf16_encode | 66.07 | 119.0 | 1.802 | 1.697-1.874 | 0.463 |
| charset | utf16_decode | 63.47 | 123.5 | 1.945 | 1.914-1.968 | 0.497 |
| charset | w1252_decode | 96.09 | 206.1 | 2.145 | 2.075-2.204 | 0.289 |
| charset | w1252_encode | 318.9 | 203.2 | 0.637 | 0.633-0.641 | 0.429 |
| bufio | none | 64.79 | 43.21 | 0.667 | 0.465-0.744 | 0.769 |
| bufio | lines | 101.1 | 56.11 | 0.555 | 0.403-0.859 | 0.436 |
| bufio | until | 104.1 | 75.84 | 0.729 | 0.633-0.882 | 0.474 |
| bufio | exact | 81.34 | 54.65 | 0.672 | 0.604-0.821 | 0.768 |

Reading:

- hash: crc64 (1.04) and adler32 (1.05) are within noise of Go; crc64e 1.10 has an interval of 1.01 to 1.23 and uses the same slicing-by-8 tables and loop as crc64 (Go's crc64 also uses slicing-by-8 for both tables), so the gap between the two rows is noise; it is re-measured with the next batch. crc32c is 0.42 (Go's crc32c has no arm64 assembly path here), fnv128a 0.79.
- csv: parse 0.66, format 0.69 and the streaming  over a File 0.69 of Go's wall at 0.46 to 0.72 of its peak memory.
- bufio: lines 0.64, until 0.69 and exact 0.70 of Go's bufio.Reader over 1460 byte chunks (the  row, the building of the 8 MiB test data, is 0.57).
- bcrypt: hash 1.44 and compare 1.55 of x/crypto's. Not Proven. The 16 Blowfish rounds do four List reads each and each is bounds checked; Go reads fixed arrays by byte index, with no checks. Next experiment: an inner loop on the form the sha256 and aes inner loops use.
- charset: the first run, on a twin whose test text was 50% non-ASCII (unrealistic); decode is 1.8 to 2.1 of Go's, encode to Windows-1252 0.64. The twin data was then made text-like (90% ASCII) and decode given an ASCII run fast path; the re-run replaces these rows in the next part.
