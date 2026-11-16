# std/brotli decode against andybalholm/brotli (2026-11-15)

Xo's std/brotli decoder (decision 0666) with ring writes through
`BytesBuilder.write_list` (branch l3-brotli3 at 1e536c2ab, built
`--release`), against Go with github.com/andybalholm/brotli v1.2.2's
Reader through io.ReadAll. The twin is bench/perf/brotli (main.xo,
twin.go.txt); both programs print the same checksums for every
workload (none 0, text 16017547, html 20025048, mixed 6024824).

Host 1 (linux/arm64, exclusive lease), `xo bench --ab --rounds=15 go xo`,
so B / A is Xo over Go:

| workload | wall Go (ms) | wall Xo (ms) | Xo / Go wall (95%) | Xo / Go max RSS |
|---|---|---|---|---|
| text | 56.74 | 109.4 | 1.928 (1.915 to 1.954) | 0.119 |
| html | 25.32 | 96.23 | 3.801 (3.675 to 3.874) | 0.119 |
| mixed | 27.23 | 44.19 | 1.623 (1.588 to 1.650) | 0.119 |

Xo decodes 1.6 to 3.8 times slower than Go and uses an eighth of its
memory (1.5 MB against 12.6 MB). html is the furthest behind. Not yet
profiled; the likely cause is the back reference copy, which moves one
byte at a time through the ring mask (std/brotli.xo, the copy loop in
the command state) where Go's decoder copies whole slices when the
source and destination do not overlap or wrap. The earlier 0.39 to 0.66 figures
(brotli-perf notes, the same day) compare the write_list decoder with
the decoder before it, not with Go.

The Mac twin survey of the same build (not an --ab run) gave Xo / Go
1.17 text, 1.12 html and 0.99 mixed, so most of the host 1 gap is
specific to linux/arm64 or to host 1, and not explained yet: the next
step is a profile breakdown of html on host 1 (XO_PROF=cpu, callgrind)
of both programs.

Then: a bulk ring copy for long back references (`write_list` from the
ring into itself in runs of min(distance, length)), measured with the
same twin.
