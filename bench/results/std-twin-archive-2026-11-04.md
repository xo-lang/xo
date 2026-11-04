# std/archive: zip_reader rows (2026-11-04)

Host 1 (linux/arm64, `--early`, n=6), bench/perf/archive; ratio is Xo wall over Go wall.

| workload | Go ms | Xo ms | ratio | note |
|---|---|---|---|---|
| zip_reader | 83.06 | 95.23 | 1.147 | at 060252aac (before) |
| unzip | 82.1 | 81.68 | 0.995 | same entries, whole archive |

The Mac profile of zip_reader was flate (hdec_init 21 percent, huffman_block 18, huff_sym 18): each ZipReader.read built the fixed Huffman decoder (288 pushes and an hdec_init), where unzip built it once. After "flate: the fixed Huffman decoder is a constant chunk table": zip_reader 91.73 to 78.64 ms in the same A/B (0.857, 0.848-0.879), about 0.95 of Go's 83.06.

## zip_writer and zip_open (host 1, n=6, bench/perf/archive at 569c6ec75 plus the new workloads)

| workload | Go ms | Xo ms | ratio | interval |
|---|---|---|---|---|
| zip_writer | 464.5 | 588.1 | 1.266 | 1.24 to 1.274 |
| zip_open | 97.24 | 90.39 | 0.930 | 0.921 to 0.953 |

zip_writer streams the 53 entries of the archive workload to a file with ZipWriter (create, write, close), 200 passes; zip_open opens the file and reads every entry through ZipReader, 200 passes. zip_writer is the in-memory zip row's work (deflate at level 5 against Go's default) plus the file; its gap is the compressor, not the writer's framing.
