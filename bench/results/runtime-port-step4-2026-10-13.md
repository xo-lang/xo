# Runtime port, step 4: std/json's parser, tape readers, and text (2026-10-13)

Decision 0193 step 4. Machine: the shared 12 core Apple M series Mac
(darwin arm64), about ten agents running; every run through the heavy
lock, one at a time (the same machine and method as steps 1 to 3, so the
counts compare with theirs). Figures are instructions retired
(`/usr/bin/time -l`, the minimum of 5 runs of each workload,
`bench/perf/json/measure.sh`). Native builds used the Go driver
(`XO_SELF_DRIVER=0`); the self hosted SSA builder produces the same code
(TestSelfhostSSA, below). Measured at 5cca21c5.

## What was ported

`std/native/llrt/jsontext.xo` (970 lines) replaces std/llrt/xo_json.c
(983 lines) on the default path:

- the parser: encoding/json's Decoder.Token state machine and scanner as
  std/rt's jsonParse drives them, into the same tape (kind, offset,
  length or end, three words a node), with the same error texts; string
  bodies are scanned eight bytes a word (`rt.bytes_word` and wrapping
  word arithmetic), and a string node records whether it has an escape
  or a byte from 0x80 so that reading it needs no second scan;
- the tape readers the generated decoders call (`json_str` with
  encoding/json's unquote, `json_find`, `json_members` with Go's first
  position and last value of a duplicate key, by comparison up to eight
  members and a hash table above, `json_int_ok` and `json_int`,
  `json_float_ok` and `json_float` through num.xo's `parse_float_at`,
  which reads the number in place);
- the encoders' text: `rs_append_json_str` (std/rt's appendJSONString),
  `rs_append_json_float` (num.xo's Dragonbox digits in encoding/json's
  'f' and 'e' layout, `append_json_float`), and std/json's Bytes
  (base64 in quotes through num.xo's `append_base64`; the decoder's
  base64 check and decode through `from_base64`).

The SSA builders route these calls through the native prelude's
`llrt_json_*`, `llrt_rs_append_json_*`, `llrt_rs_append_b64_quoted`,
`llrt_b64_ok`, `llrt_b64_decode` (internal/ssa `jsonRT`, selfhost/ssa
`json_rt`); std/native/json.xo's decode calls `jsontext.parse`. The C
path is `XO_LLRT_C=jsontext` (jsontext_c.xo; the builder then emits the
C calls as before); the C moved under `#ifdef XO_JSONTEXT_C` (all of
xo_json.c and xo_rs_append_b64_quoted), with its IOFlags entry and
xo1's `io_flags`.

## What was measured

`bench/perf/json` (new; 5,000 values, 1,000 small documents): `small`
(decode 1,000 objects of six fields into a struct, 40 rounds), `ints`
(a List[Int] of 5,000 Ints of 1 to 19 digits), `floats` (a List[Float]
of 5,000 numbers in five forms), `strings` (a List[Str] of 5,000:
plain, escapes, raw UTF-8, \u escapes and surrogate pairs), `maps`
(100 objects of 40 keys into Map[Str, Int]), `parse` (the parser alone:
a 300 KB mixed document decoded as Bool, which fails after the parse),
`invalid` (1,000 texts with an error at the end, the middle, the start),
`enc_small`, `enc_strings`, `enc_floats` (encode_str of structs,
strings, Floats), `bytes` (encode and decode a List[Bytes] of 5,000).
"C" is the same compiler with `XO_LLRT_C=jsontext`. The documents are
built by interpolation, not json.encode, so the setup (`none`) costs the
same on both paths (ratio 1.004 and 1.003); the bar is read on "less
setup" (each path's `none` subtracted), as in step 3.

Checksums are identical on both backends, both paths, and equal the Go
backend's (small 946556621, ints -992501159, floats 800544357, strings
759638791, maps 863563615, parse 405984031, invalid 672279139,
enc_small 579487119, enc_strings 481629333, enc_floats 821466738, bytes
222217691).

## Final (the committed code)

| Workload | arm64 Xo | arm64 C | ratio | less setup | LLVM Xo | LLVM C | ratio | less setup |
|---|---|---|---|---|---|---|---|---|
| `none` | 42,242,884 | 42,070,619 | 1.004 |  | 38,189,993 | 38,060,564 | 1.003 |  |
| `small` | 347,233,437 | 532,015,851 | 0.653 | 0.622 | 309,900,321 | 516,324,781 | 0.600 | 0.568 |
| `ints` | 224,694,362 | 234,953,264 | 0.956 | 0.946 | 209,935,863 | 228,929,576 | 0.917 | 0.900 |
| `floats` | 369,783,296 | 538,505,700 | 0.687 | 0.660 | 339,139,234 | 527,986,317 | 0.642 | 0.614 |
| `strings` | 356,594,968 | 377,402,921 | 0.945 | 0.937 | 311,668,799 | 368,737,670 | 0.845 | 0.827 |
| `maps` | 329,514,826 | 342,474,244 | 0.962 | 0.956 | 305,681,562 | 327,292,314 | 0.934 | 0.925 |
| `parse` | 288,333,549 | 326,615,088 | 0.883 | 0.865 | 254,960,184 | 337,019,916 | 0.757 | 0.725 |
| `invalid` | 220,574,554 | 448,632,999 | 0.492 | 0.439 | 204,225,545 | 445,016,447 | 0.459 | 0.408 |
| `enc_small` | 174,802,081 | 1,107,028,039 | 0.158 | 0.124 | 156,664,605 | 1,099,814,385 | 0.142 | 0.112 |
| `enc_strings` | 225,681,545 | 237,680,377 | 0.950 | 0.938 | 210,825,452 | 229,932,211 | 0.917 | 0.900 |
| `enc_floats` | 154,974,600 | 5,522,282,523 | 0.028 | 0.021 | 124,058,012 | 5,516,642,722 | 0.022 | 0.016 |
| `bytes` | 436,676,800 | 602,487,638 | 0.725 | 0.704 | 347,466,395 | 593,783,207 | 0.585 | 0.557 |

The bar (1.05) holds on both backends for every workload, so nothing of
std/json's text stays C on the default path. The closest are `maps`
(0.956 on arm64: most of it is the Map puts, the same on both paths),
`ints` (0.946), `enc_strings` (0.938), and `strings` (0.937).

## How it got there

- **First version** (every piece as above but Int reading through an
  `Int?` helper, `int_ok` and `int_at` each calling it): every workload
  but one within the bar; `ints` 1.111 on arm64 and 1.052 on LLVM (less
  setup; raw 252,306,297 against 230,804,086 on arm64). Each element read
  its digits twice through two calls and an Option.
- **Int reading** written out: `int_ok` checks at most 19 digits (JSON
  numbers have no leading zeros, so 18 cannot overflow; the 19th is
  compared with 922337203685477580 and the sign), and `int_at`, called
  only after it, accumulates with wrapping arithmetic and no checks:
  `ints` 0.946 and 0.900.
- `parse` (the parser alone) was 0.866 and 0.725 from the first version:
  the per string eight byte scan and one function for the token state
  machine (no call per token, the open containers' counts in locals)
  more than pay for the bounds checks.
- `enc_floats` and `enc_small`: the C wrote a Float with snprintf and
  strtod up to 17 times (n_shortest); num.xo's Dragonbox digits make it
  under 1/40 of the instructions.

## Other workloads

bench/perf/text at the same commit (Xo path, then `XO_LLRT_C=num`):
`base64` 162,928,712 and 403,636,146 on arm64, 129,436,491 and
418,834,351 on LLVM (step 3: 162,438,761 on arm64; to_base64 is now
append_base64 behind a call), less setup 0.394 and 0.293;
`time_parse` (a json.decode of List[Time]) 151,231,699 on arm64 and
97,146,124 on LLVM with std/json's parser now Xo on both of its paths
(step 3: 170,119,519 and 120,136,211).

## Correctness

- TestJSONTextDiff (internal/llvm/jsontext_test.go, new): a program that
  decodes each input into 15 types (Bool, List[Str], Map[Str, Str],
  List[Float], List[Int], Map[Str, List[Int?]], Str, Float, Int, a
  struct of optional fields and a default, List[Map[Str, Float?]], a
  tuple, Map[Str, Bool?], List[List[Int]], List[Bytes]) and encodes what
  decoded again, encodes Floats from random bits and edge texts, and
  encodes random UTF-8 strings with every byte encoding/json escapes;
  27,900 input lines (the C parser's corpus, 6,000 random valid and
  damaged texts, deep nesting to 3,000 levels, invalid UTF-8, lone and
  paired surrogates, 400 digit and out of range numbers, duplicate keys
  in small and large objects including escaped equal keys, trailing
  data, an escape at each offset 0 to 19 of a string, base64 strings).
  The Go backend's output is the reference; arm64 and LLVM, Xo path and
  C path, are identical to it, and the LLVM Xo build frees every
  allocation (XO_LEAKCHECK).
- Golden `json_text` (new) on Go, LLVM, and arm64; `json_text`,
  `native_json`, `text_table`, `codex_gaps` give want.txt on both native
  backends with either path; `str_ops_table` and `native_display` pass
  (TestGolden, TestGoldenRun); TestJSONParseDiff (the C path,
  now compiled with `-DXO_JSONTEXT_C`), TestNumDiff (both paths),
  TestLLRTCPath, TestIOFlagsJSONTextC pass.
- TestSelfhostSSA over the json programs (json_text, native_json,
  text_table, codex_gaps, toml_basic, bench/perf/json): 7 runs, 1,034
  functions identical at every stage.
- wasm (TestSelfhostWasm): 128 of 145 goldens pass, 17 unsupported, 0
  fail; native_json, text_table, codex_gaps, and json_text now run
  (they stopped at the C only `json_parse`, `rs_append_json_str`, and
  `rs_append_b64_quoted`). codex_gaps then showed a wasm runtime bug
  hidden until now: `clock_sleep_or_cancel` returned an i32 where the
  SSA builder types an Int (gotchas/wasm-runtime-result-type-mismatch.md),
  fixed in selfhost/wasm rt3.xo. wasmMinPass is raised from 116 to 128.
- `go test -short` over the repository passes.

## Sizes

- Runtime pack (`xo rtpack`, darwin/arm64, every variant and the TLS
  library): before (909ebf43) 17,392 KiB and 16,050,864 bytes of
  objects, key 84c0a71b...; after 16,752 KiB and 15,402,208 bytes
  (648,656 bytes, 4.0 percent, less), key 9a14615a.... Built twice
  after: byte identical.
- C runtime: xo_json.c (983 lines) and xo_rs_append_b64_quoted are in
  no pack variant; xo_std.c 1,407 lines before, 1,415 after (the guards).
  Xo: jsontext.xo 970 lines, jsontext_c.xo 39; num.xo 1,420 to 1,558
  (append_json_float, parse_float_at, append_base64).
