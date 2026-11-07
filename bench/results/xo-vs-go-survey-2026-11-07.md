# Xo against Go across bench/perf, 2026-11-07

Where native Xo is still slower than Go, ranked, after the arm64
intrinsics of decision 0532 landed, so the next performance items come
from data. Asked by the lead on 2026-11-07.

## Method

- Rev adb9f0af5 (main plus the `twincheck -time` flag), xo 0.43.0
  built with `go run ./scripts/xobuild` from the 0.43.0 seed; Go 1.26.1.
- The first linux/arm64 offload host (Graviton), one job at a time;
  load average 5.7 at the start (the host's own idle background), so
  rows within about 10% of 1.0 are noise.
- `go run ./scripts/twincheck -time 5 bench/perf/*`: each case's Xo
  and Go twins are checked to print the same output, then each runs 5
  times, alternating, and the medians of wall time are compared.
  Ratio = Xo / Go; below 1 is Xo faster.
- What dominates each gap: `XO_PROF=cpu XO_PROF_HZ=5000` on the Xo
  program for that case on the same host, then `xo prof`.

## Summary

387 cases timed. Geometric mean of Xo / Go: 0.724. 287 cases are faster
than Go, 65 are more than 10% slower, 89 run under 5 ms on both sides
(startup dominated; excluded from the ranking).

## Top 10 gaps

Ratio is Xo / Go wall time (median of 5); the profile column is the Xo
program's XO_PROF on the same host ("cum" includes callees).

| # | Case | Ratio | Xo ms | Go ms | What dominates the Xo run |
|---|---|---|---|---|---|
| 1 | metrics/counter | 4.251 | 296.8 | 69.8 | `series_of` (std/native/metrics.xo:351) per `inc`: the label lookup runs inside an RwMutex `with_read` closure, and its unwind guard costs `ctx.unwinding` 13.6% plus `rc_defer`/`rc_defer_n` 21%; the call site's one element `[names[i % 50]]` List is built and dropped per call (`drop List[Str]`, `alloc.free` 5.5%) |
| 2 | cbor/umarshal | 3.493 | 15.2 | 4.3 | short run (Go 4.3 ms); `cbor.marshal` of a derived struct, 100 times: `enc_item` 79% cum, the deterministic map key sort (`merge_sort_in_place[Int]` 16%), output growth (`rl_grow_n` 27%, `reserve_slow`) |
| 3 | time/format | 3.021 | 106.4 | 35.2 | the `{field}` layout is scanned on every call: `field_scan` 31% cum, `field_code_b` 21%, `eq_range` 8%; `obj_rs_concat` 13%. With the layout compiled once (row 10) the gap is 1.65 |
| 4 | time/parse | 2.824 | 51.5 | 18.2 | `parse_stream` 39% flat (layout scanned per call, as in row 3: `field_code_b` 26% cum, `eq_range` 10%), `read_digits` 14% |
| 5 | metrics/histogram | 2.718 | 206.8 | 76.1 | the same `series_of` lookup as row 1 (69% cum), then `histogram_add` 9%, `mutex.reader_out` 6% |
| 6 | time/parse_layout | 2.006 | 36.0 | 18.0 | compiled layout: `parse_items` 32% flat and `read_digits` 32% flat (a digit at a time) |
| 7 | log/filtered | 1.793 | 13.4 | 7.5 | below the level, nothing written: the call's fields Map (`maph.putbody[Map[Str, xo.dyn]]`), its interpolated value and its `xo.dyn` boxes are built and dropped before the level check discards them |
| 8 | timego/parse | 1.776 | 16.8 | 9.5 | not profiled; the Go layout form over the same time parser as rows 4 and 6 |
| 9 | jsontoken/write | 1.732 | 106.7 | 61.6 | `JsonWriter.write_string` 70% cum (25% flat): the escape scan (`find_bytes`, `find_byte`, `tail16`) and `strings.replace`, then a byte at a time `rs_push_byte` 9% |
| 10 | time/format_layout | 1.654 | 58.1 | 35.1 | compiled layout: not profiled separately; row 3 without the layout scan (`format_stream`, `obj_rs_concat`, `tzrules.tx_at`) |

By cause, the ten rows are five items:

- metrics label lookup (rows 1, 5): an unwind guarded closure and a
  List per call where client_golang hashes the values under RLock.
- time layouts (rows 3, 4, 6, 8, 10): the layout scanned per call, and
  the compiled layout's digit and item loops.
- cbor deterministic marshal (row 2): key sort and output growth.
- log below the level (row 7): fields built before the level check.
- JSON string writing (row 9): the escape scan and replace.

Profiles of the next rows, for the queue after these:

| Case | Ratio | Xo ms | Go ms | What dominates the Xo run |
|---|---|---|---|---|
| tabwriter/pad | 1.543 | 47.9 | 31.1 | `TabWriter.rune_len` 20% flat, `write`/`write_buf`/`write_lines` 50% flat |
| walkdir/walk | 1.509 | 424.4 | 281.3 | not a like-for-like row: Xo's `walk_dir` returns a FileInfo per entry, so it stats each one (`statx` 36%), as Go's `filepath.Walk` does, while the twin runs `filepath.WalkDir` for every walk row; the matching Xo call, `walk_dir_entries` (walkdir/walk_entries), is 0.921 of Go. The twin runs filepath.Walk for walk and walk_info since this survey (bench/perf/walkdir) |
| trimchars/trim_end | 1.468 | 20.7 | 14.1 | `scan.ascii_set_partial` 27% flat: the cut set's table built on every call |
| netip/parse6 | 1.468 | 16.0 | 10.9 | `parse_ipv6` 69% flat |
| charset/w1252_decode | 1.413 | 37.2 | 26.3 | `charset.decode` 63% flat (a rune at a time), `scan.utf8_valid` 23% cum |
| strconv/itoa | 1.385 | 119.5 | 86.3 | `num.append_int` 31% flat, the Str's allocation and word pushes (`rs_push_words`, `rs_new_n`, `alloc_fast`, `obj_rc_free`) about 35% |
| csv/reader | 1.343 | 39.6 | 29.5 | `read_record` 77% cum, `drop Str` 11% |

## Other cases more than 10% slower (5 ms or more)

| Case | Ratio | Xo ms | Go ms |
|---|---|---|---|
| tabwriter/pad | 1.543 | 47.9 | 31.1 |
| walkdir/walk | 1.509 | 424.4 | 281.3 |
| tabwriter/right | 1.498 | 47.3 | 31.6 |
| trimchars/trim_end | 1.468 | 20.7 | 14.1 |
| netip/parse6 | 1.468 | 16.0 | 10.9 |
| netip/parse4 | 1.443 | 7.5 | 5.2 |
| charset/w1252_decode | 1.413 | 37.2 | 26.3 |
| netip/display4 | 1.411 | 9.5 | 6.7 |
| sqlrow/scan | 1.401 | 85.2 | 60.8 |
| strconv/itoa | 1.385 | 119.5 | 86.3 |
| trimchars/trim | 1.373 | 22.6 | 16.5 |
| trimchars/trim_start | 1.343 | 19.7 | 14.7 |
| csv/reader | 1.343 | 39.6 | 29.5 |
| path/parts | 1.328 | 8.7 | 6.5 |
| charset/utf16_decode | 1.325 | 44.3 | 33.5 |
| trimchars/uni | 1.320 | 41.4 | 31.3 |
| timego/format | 1.308 | 43.9 | 33.5 |
| strconv/atoi | 1.292 | 43.7 | 33.8 |
| aesgcm/open | 1.290 | 51.4 | 39.9 |
| sort/strs | 1.288 | 34.2 | 26.6 |
| log/json | 1.286 | 117.8 | 91.6 |
| url/query | 1.274 | 43.7 | 34.3 |
| aesgcm/key | 1.274 | 8.8 | 6.9 |
| trimchars/one | 1.245 | 11.4 | 9.1 |
| netip/mixed | 1.232 | 24.6 | 19.9 |
| csv/format | 1.181 | 38.5 | 32.6 |
| cbor/udecode | 1.181 | 14.0 | 11.9 |
| xml/stream | 1.180 | 246.5 | 208.8 |
| archive/zip_writer | 1.176 | 556.6 | 473.2 |
| ed25519/sign | 1.171 | 80.1 | 68.4 |
| aead/gcm_key | 1.164 | 44.9 | 38.6 |
| regexp/match | 1.163 | 57.2 | 49.1 |
| llvm/sieve | 1.162 | 190.2 | 163.7 |
| repeat/byte100 | 1.158 | 94.0 | 81.2 |
| big/text | 1.154 | 6.1 | 5.3 |
| aesgcm/seal | 1.144 | 47.7 | 41.7 |
| sha3/small | 1.142 | 67.0 | 58.6 |
| xml/tokens | 1.140 | 236.6 | 207.5 |
| conc/rwmutex | 1.134 | 5.7 | 5.0 |
| archive/zip | 1.118 | 523.2 | 468.0 |
| walkdir/walk_entries_info | 1.115 | 500.6 | 449.0 |
| trimchars/index | 1.115 | 5.5 | 5.0 |
| jpeg/rgba | 1.114 | 109.3 | 98.1 |
| regexp/compile | 1.112 | 82.2 | 73.9 |
| template/render | 1.108 | 265.4 | 239.5 |
| aesgcm/open1 | 1.107 | 38.8 | 35.1 |
| otp/sha1 | 1.105 | 103.5 | 93.7 |

## Not timed

- Go twin did not build (the host has no module cache for these
  modules and `GOPROXY=off`): bcrypt, idna, jwt, protobuf, rate, sqlpg,
  toml, websocket. A rerun with the modules vendored or a module cache
  copied in would cover them.
- Output mismatch (twincheck compares the printed text; first line
  differs, not investigated here): metrics/scrape, sizewaste.
- jitter: the Xo program was killed (`signal: killed`) before it
  finished; not investigated here.
