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

After std/changes/path.md 2026-10-26 (is_match by offset, base cut once; Xo Lang 2), the same --ab, A the Go
twin, B the Xo program:

| workload | Go wall | Xo wall | wall | instructions |
|---|---|---|---|---|
| path match | 8.45 ms | 8.43 ms | 0.998 | 1.12 |
| path parts | 9.68 ms | 14.81 ms | 1.530 | 1.694 |
| path clean | 9.97 ms | 11.45 ms | 1.148 | 1.467 |
| path join | 20.88 ms | 24.33 ms | 1.165 | 1.349 |

match is at Go's time. parts is still over the bar. (Corrected the same evening: a Str slice is a view since
decision 0330, not a copy.) An XO_PROF=cpu profile of parts, 100x the passes, 109 samples: path.clean 54%
cumulative (dir calls it, as Go's Dir does), of which path.Lazy.append 22% (its copy buffer, a List[UInt8]
pushed a byte at a time) and the loop 28%; prelude.llrt_str_slice_chk 16.5% (each view's range and UTF-8
boundary check); drop Str 7%. The next step is clean's buffer, written in runs rather than bytes. Done 2026-10-27
(std/changes/path.md: elements as offsets, a clean path returned as it is): clean 1.016 of Go's wall (0.850 of
the parent), join 1.11 (0.981), match 0.975, parts 1.544 unchanged (its cost is outside clean now).

## uuid, base32, mime (same day, Xo Lang 2)

Twins: bench/perf/uuid (github.com/google/uuid v1.6.0; twin.txt's require line), bench/perf/base32
(encoding/base32), bench/perf/mime (mime and net/http's DetectContentType); twincheck agrees on every
workload, and each has a `none` workload (the setup and loop alone). `xo bench --ab`, quiet, 15 rounds, A the
Go twin, B the Xo program; Xo / Go wall before and after the changes in std/changes (uuid.md, base32.md,
mime.md, 2026-10-26):

| workload | before | after | note |
|---|---|---|---|
| uuid none | 0.887 | 0.904 | |
| uuid parse (five forms and one bad text, 300,000) | 2.794 | 1.529 | text written from the input's digits |
| uuid string (display) | 0.354 | 0.355 | Xo keeps the canonical text |
| uuid bytes (bytes() and from_bytes) | 9.655 | 7.141 | over: a Uuid holds text, from_bytes formats 36 characters where google/uuid copies 16 bytes |
| base32 none | 0.570 | 0.568 | |
| base32 encode (2000 buffers, 20 passes) | 2.082 | 1.130 | whole blocks as one word |
| base32 decode | 0.845 | | |
| mime none | 0.708 | 0.669 | |
| mime parse (6 values) | 1.064 | 1.051 | |
| mime format | 1.234 | 1.231 | over by 0.03 |
| mime ext (8 extensions) | 5.453 | 0.760 | a match generated from the table |
| mime detect (7 headers) | 8.256 | 1.216 | constant signatures, offsets instead of slices |

Then (2026-10-27, std/changes/uuid.md: std/internal's uuid_text and uuid_decode, table driven): uuid bytes
7.141 to 4.241, parse 1.529 to 1.230, string 0.347, none 0.878; what is left of bytes is its allocations (the
16 bytes, the Result, the text), where google/uuid copies into a value array.

Still over the 1.2x bar: uuid parse and bytes (the representation: a Uuid is its canonical text, and each
character is a write call; a native word writer, as base64's to_base64, is the next step), mime format (1.23)
and detect (1.22, at the edge), path parts (1.53, clean's byte at a time buffer and the slice checks above).


## compress, template, mail, archive, big, xmlcodec, toml, websocket (2026-10-27, Xo Lang 2)

Twins: bench/perf/compress (compress/gzip, compress/flate), template (text/template), mail (net/mail),
archive (archive/tar, archive/zip; the checksum counts entries and content bytes, not archive bytes),
big (math/big), xmlcodec (derive Xml against encoding/xml), toml (github.com/pelletier/go-toml/v2 v2.4.3)
and websocket (github.com/gorilla/websocket v1.5.3, an echo server and client on loopback in one program).
twincheck agrees on all 35 workloads. `xo bench --ab`, quiet, 15 rounds (big and websocket 10), A the Go
twin, B the Xo program (native arm64, --release, main at fbbf37322); Xo / Go:

| workload | Go wall | Xo wall | wall | instructions |
|---|---|---|---|---|
| compress none (40 buffers of 32 KB of text) | 6.73 ms | 4.93 ms | 0.733 | 0.686 |
| compress gzip (default level, 10 passes) | 490.5 ms | 719.9 ms | 1.468 | 1.933 |
| compress gzip1 (BestSpeed) | 87.2 ms | 169.1 ms | 1.939 | 2.514 |
| compress gunzip | 88.7 ms | 172.7 ms | 1.947 | 2.426 |
| compress deflate | 489.0 ms | 699.3 ms | 1.430 | 1.909 |
| compress inflate | 87.4 ms | 124.7 ms | 1.426 | 1.935 |
| template none | 3.42 ms | 2.95 ms | 0.862 | 0.680 |
| template parse (a page template, 20,000) | 105.1 ms | 105.7 ms | 1.006 | 1.078 |
| template render (30 items of JSON data, 20,000) | 210.6 ms | 614.4 ms | 2.917 | 1.847 |
| mail none | 3.32 ms | 3.05 ms | 0.919 | 0.957 |
| mail addr (five forms, one bad, 100,000) | 31.6 ms | 43.5 ms | 1.379 | 1.056 |
| mail list (four addresses) | 92.6 ms | 131.3 ms | 1.418 | 0.991 |
| mail display | 13.2 ms | 37.0 ms | 2.796 | 1.769 |
| mail msg (read_message, two header lookups) | 94.2 ms | 57.1 ms | 0.606 | 0.527 |
| mail date (four RFC 5322 dates) | 92.5 ms | 40.0 ms | 0.432 | 0.314 |
| archive none (50 files of 1 to 4 KB, 3 directories) | 3.26 ms | 2.77 ms | 0.848 | 0.722 |
| archive tar (200 passes) | 13.7 ms | 16.5 ms | 1.207 | 1.396 |
| archive untar | 23.1 ms | 9.4 ms | 0.406 | 0.342 |
| archive zip (deflated) | 473.0 ms | 729.1 ms | 1.541 | 2.387 |
| archive unzip | 84.7 ms | 137.9 ms | 1.629 | 2.159 |
| big none | 3.04 ms | 2.55 ms | 0.839 | 0.718 |
| big mul (2048 by 2048 bits, 20,000) | 10.7 ms | 32.1 ms | 2.997 | 3.938 |
| big quorem (4096 by 2048 bits, 10,000) | 11.6 ms | 23.5 ms | 2.025 | 2.975 |
| big text (2048 bits to decimal, 2,000) | 7.56 ms | 11.25 ms | 1.490 | 1.831 |
| big parse (decimal, 2,000) | 10.1 ms | 5.49 ms | 0.544 | 0.593 |
| big exp (1024 bit base, exponent, odd modulus; 200) | 60.0 ms | 764.3 ms | 12.75 | 6.037 |
| xmlcodec none | 3.05 ms | 2.66 ms | 0.872 | 0.763 |
| xmlcodec encode (an order of 20 lines, 20,000) | 173.8 ms | 310.7 ms | 1.788 | 1.413 |
| xmlcodec decode | 572.2 ms | 704.4 ms | 1.231 | 0.766 |
| toml none | 3.08 ms | 2.58 ms | 0.837 | 0.690 |
| toml decode (a config into a struct, 20,000) | 51.0 ms | 113.7 ms | 2.230 | 1.947 |
| websocket none | 4.73 ms | 3.48 ms | 0.735 | 0.510 |
| websocket small (64 byte text round trips, 20,000) | 314.7 ms | 332.8 ms | 1.057 | 1.100 |
| websocket big (16 KB binary round trips, 5,000) | 161.1 ms | 249.8 ms | 1.550 | 2.818 |
| websocket dial (handshake, one round trip, close; 500) | 59.8 ms | 52.0 ms | 0.868 | 0.706 |

Fixed the same day (std/changes compress.md, mail.md, big.md 2026-10-27), --ab against the parent's std:

| workload | against the parent | Xo / Go now |
|---|---|---|
| compress gunzip (CRC-32 slicing-by-8, no member copy) | 0.773 | about 1.51 |
| mail display (quote by bytes) | 0.361 | about 1.01 |
| big exp (bigmod's Montgomery exp for an odd modulus) | 0.072 | 0.920 (measured) |

Still over 1.2x, with an XO_PROF=cpu profile line each (loops scaled 10x; flat shares), waiting:

- template render 2.92: refcount traffic on the tree walk, no single hot spot: Ex.walk 9%, drop Ctx 7.6%,
  Ex.at_node 7.2%, walk_list 6.7%, dup Node 4.9%, drop V 4.8%, drop Node 4.6% (dup and drop together about
  25%).
- compress deflate and gzip 1.43 to 1.47 (and archive zip 1.54): Deflater.deflate 80% (the match search).
  gzip1 1.94: Blocks.write_tokens 33%, index_tokens 20%, Fast.encode 18%. inflate 1.43 (archive unzip 1.63;
  gunzip after the fix): huffman_block 38%, huff_sym 17%, memmove 12.5% (the output is a List[Int], one
  word a byte).
- big mul 3.00: nat_mul 86% (Go's addMulVVW is assembly). quorem 2.03: nat_divmod_basic 84%. text 1.49:
  convert_words 29% (few samples).
- toml decode 2.23: spread out: alloc 7.8%, TomlParser.key 7.8%, memmove 6.8%, json_string 5.8% (decode goes
  through JSON text), ws 5.8%, basic_string 4.9%.
- xmlcodec encode 1.79: string building: obj.rs_append 12%, move_at 6.9%, Printer.write_start 6.1%,
  encode_element 5.4%. decode 1.23: the byte reader: Dec.getc 12.8%, raw 10%, token 9.4%, mustgetc 7.3%.
- websocket big 1.55: _sendto 60%, kevent 18%, _ulock_wait 8%, read 7%: syscall and wait bound; whether a
  frame goes out in more than one sendto is the next check.
- mail list 1.42 and addr 1.38: rune decoding: bytes_decode_rune 7.6 to 13.9%, raw.decode 9.2%, is_atext 6.7 to
  13.9%, llrt_str_slice_chk 7.6%.
- archive tar 1.21 (a 14 ms run): set_ustar 27%.

Under the bar: template parse, mail msg and date, archive untar, big parse, websocket small and dial, and
the fixed rows. Still without a row: metrics (no Go counterpart in the module cache offline:
prometheus/client_golang), smtp and sql (need a server), and http's request-head parser (bench/perf/httpparse
exists; its row is Xo Lang 3's).
