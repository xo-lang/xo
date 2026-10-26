# std modules against Go's packages: which have a timing row (2026-10-26)

The owner's rule (2026-10-21): every std port carries a timing row against
Go's package. This audit lists each std module, its bench program or results
file with a Go comparison (or none), and this day's new rows.

## Audit

Has a row against Go (results file or bench with a Go twin):

| std module | where |
|---|---|
| json, multipart | bench/results/json-vs-go-2026-10-21.md (bench/perf/jsontwin has the Go twin) |
| csv, regexp, time | regexp-time-csv-vs-go-2026-10-21.md; time-format/-parse-2026-10-22 |
| netip | netip-2026-10-22.md (bench/perf/netip, twin.go.txt) |
| tls, http (HTTP/2, TLS) | tls-h2-vs-go-2026-10-21.md |
| url | url-parse-2026-10-23.md |
| xml | xml-parse-2026-10-22.md |
| crypto (aesgcm, chacha, digest, rsa) | aesgcm-2026-10-22.md, chacha-poly1305-2026-10-22.md |
| prelude: bytes and text (search, split, case, affix) | bytessearch/bytes-split/case-fold results; bench/perf/text, strsplit |

No row against Go (before this day): archive, base32, base64, big (only
rsa-bigmod), compress, mail, metrics, mime (a baseline only), path, smtp,
sql, template, toml, uuid, websocket (Autobahn conformance, no timing),
xmlcodec, http's cookie and request-head parsers, and the prelude's number
text (Int.parse, Float.parse, "${n}", "${f}"). No bench for log, term,
testing, debug, cli, internal (not performance code).

## New rows (this day)

Three modules without a row, each with a Go twin (bench/perf/base64,
bench/perf/strconv, bench/perf/cookie; `go run ./scripts/twincheck` agrees on
all 8 workloads). `xo bench --ab`, quiet (load 3.8), 15 rounds, A the Go
twin, B the Xo program (native arm64, --release); ratios are B / A.

| workload | Go wall | Xo wall | wall | instructions | cycles |
|---|---|---|---|---|---|
| base64 encode (2000 buffers of 0 to 3,000 bytes, 20 passes), first measurement | 48.5 ms | 74.6 ms | 1.537 | 2.093 | 1.302 |
| base64 decode (same), first measurement | 35.7 ms | 74.4 ms | 2.085 | 2.713 | 1.950 |
| strconv atoi (1,000 ints, 300 passes) | 4.9 ms | 5.5 ms | 1.128 | 0.870 | 1.187 |
| strconv itoa | 7.4 ms | 8.3 ms | 1.113 | 0.609 | 1.137 |
| strconv atof | 9.5 ms | 8.0 ms | 0.848 | 0.780 | 0.816 |
| strconv ftoa | 17.1 ms | 10.6 ms | 0.620 | 0.580 | 0.587 |
| cookie, parse_cookie (100,000 passes) | 24.4 ms | 21.5 ms | 0.880 | 0.760 | 0.760 |
| cookie, parse_set_cookie | 28.5 ms | 23.5 ms | 0.827 | 0.748 | 0.784 |

Worst, first measurement: std/base64 (decode 2.09x wall, 2.7x instructions;
encode 1.54x wall, 2.1x instructions). That run was inflated by the Xo program's
setup (6 MB of text built by 3 million string appends, 0.55 of Go's wall alone
once replaced by a BytesBuilder), so bench/perf/base64 now has a `none`
workload (the setup alone) and the rows below are the corrected ones. The atoi and itoa wall figures (1.11 to 1.13) are 5 to 8 ms
runs where process start is a large share and Xo's instruction counts are
lower (0.87, 0.61): not a finding. The number text and the cookie parsers are
faster than Go's. Peak memory is lower for Xo in every row (0.15 to 0.66 of
Go's). Machine: the development Mac, Go 1.26.1.

## base64, corrected and fixed (same day)

With the setup fixed (`none` row: Xo 0.55 of Go's wall), main's std against Go: encode 1.084 (instructions
1.52), decode 1.380 (instructions 1.97, cycles 1.28). After the change in std/changes/base64.md (Std runs
Bytes.to_base64 and from_base64, which decodes eight and 32 characters at a time): encode 0.691
(instructions 0.80, cycles 0.59), decode 1.078 (instructions 1.31, cycles 1.00). Both inside the 1.2x
bar. Next twins: path, uuid, base32, mime, template, archive, compress.

## path (same day)

bench/perf/path (twin.go.txt against Go's path package; twincheck agrees on clean, join, parts, match).
`xo bench --ab`, quiet (load 3.3), 15 rounds, A the Go twin, B the Xo program; Xo / Go wall (instructions,
cycles); the runs are 8 to 24 ms, so process start is a share of each:

| workload | Go wall | Xo wall | wall | instructions | cycles |
|---|---|---|---|---|---|
| path clean (200,000 passes over 8 paths) | 9.6 ms | 11.6 ms | 1.207 | 1.468 | 1.232 |
| path join | 21.3 ms | 24.5 ms | 1.147 | 1.351 | 1.124 |
| path parts (split, dir, base, ext, is_abs) | 9.4 ms | 14.6 ms | 1.541 | 1.732 | 1.658 |
| path match (is_match against 6 patterns) | 8.2 ms | 14.0 ms | 1.697 | 1.602 | 1.862 |

Over the 1.2x bar: parts (1.54x) and match (1.70x), clean at the bar's edge (1.21x). Peak memory 0.23 to 0.57
of Go's. Fix next, one at a time: match, then parts.
