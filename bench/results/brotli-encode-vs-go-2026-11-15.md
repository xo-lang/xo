# std/brotli encode against andybalholm/brotli (2026-11-15)

Xo's std/brotli encoder (decision 0666; branch l3-brenc at 5b5bdfe31 plus the
bench workloads, built `--release`), against Go with
github.com/andybalholm/brotli v1.2.2's Writer (NewWriterLevel, Write,
Close). The twin is bench/perf/brotli (main.xo, twin.go.txt): the 100 KB
html input at quality 1 (enc1, 100 encodes), 6 (enc6, 20) and 11 (enc11,
2). Both programs print the same checksums (enc1 1003445, enc6 167596,
enc11 13312): the outputs are byte identical, as std's tests require.

Host 1 (linux/arm64, exclusive lease), `xo bench --ab --rounds=10 go xo`,
so B / A is Xo over Go:

| workload | wall Go (ms) | wall Xo (ms) | Xo / Go wall (95%) | Xo / Go max RSS |
|---|---|---|---|---|
| enc1 | 29.28 | 41.1 | 1.403 (1.04 to 1.499) | 0.274 |
| enc6 | 26.17 | 361.5 | 13.81 (13.28 to 14.86) | 0.452 |
| enc11 | 181.7 | 353.3 | 1.944 (1.934 to 1.965) | 2.715 |

The Mac twin survey of the same build (`go run ./scripts/twincheck -time
3 bench/perf/brotli`, not an --ab run) gave Xo / Go 1.55 enc1, 3.60 enc6
and 1.79 enc11. Quality 6 is the open gap: 13.8 times Go on host 1 and
3.6 on the Mac, so like the decoder's gap (brotli-decode-vs-go-2026-11-15.md)
most of it is specific to linux/arm64. Not profiled yet. A hypothesis to
test first: each encode allocates and zeroes its hash tables (h5/h6's
buckets, megabytes at quality 6), and on linux/arm64 fresh pages fault
in on every call where Go's allocator reuses its heap. Quality 11 also
peaks at 2.7 times Go's memory (80 MB against 29 MB).

Next: XO_PROF=cpu and callgrind of enc6 on host 1 for both programs, then
the levers ranked to the lead with the decoder's (task #67).
