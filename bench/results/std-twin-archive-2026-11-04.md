# std/archive: zip_reader rows (2026-11-04)

Host 1 (linux/arm64, `--early`, n=6), bench/perf/archive; ratio is Xo wall over Go wall.

| workload | Go ms | Xo ms | ratio | note |
|---|---|---|---|---|
| zip_reader | 83.06 | 95.23 | 1.147 | at 060252aac (before) |
| unzip | 82.1 | 81.68 | 0.995 | same entries, whole archive |

The Mac profile of zip_reader was flate (hdec_init 21 percent, huffman_block 18, huff_sym 18): each ZipReader.read built the fixed Huffman decoder (288 pushes and an hdec_init), where unzip built it once. After "flate: the fixed Huffman decoder is a constant chunk table": zip_reader 91.73 to 78.64 ms in the same A/B (0.857, 0.848-0.879), about 0.95 of Go's 83.06.
