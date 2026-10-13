# Scan intrinsics: SIMD and word at a time scans in Xo (2026-10-13)

Decision 0201 (decision 0195 prerequisite 4). Machine: the shared 12
core Apple M series Mac (darwin arm64), several agents running; every
run through the heavy lock, one at a time. Figures are instructions
retired (`/usr/bin/time -l`, the minimum of 5 runs of each workload,
`bench/perf/text/measure.sh`), the Xo path (std/native/llrt/scan.xo)
over the C path (`XO_LLRT_C=scan`, scan_c.xo over the C runtime), whole
process and less the setup (the `none` workload). Native builds used
the Go driver (`XO_SELF_DRIVER=0`); TestSelfhostArm64, TestSelfhostLLVM,
and TestSelfhostSSA show the self hosted compiler emits the same code
for these programs. Timings are noisy with other agents running and
are not used.

## What landed

- Intrinsics in std/native/rt.xo, lowered inline by both arm64 emitters
  (internal/arm64 isel.go, selfhost/arm64 calls.xo), both LLVM emitters
  (internal/llvm fromssa.go, selfhost/llvm call.xo), and the wasm
  runtime (selfhost/wasm rt2.xo), with the instructions in selfhost/macho's
  assembler:
  - `rt.popcount(w)`: arm64 `fmov d30`, `cnt`, `addv`, `fmov`; LLVM
    `llvm.ctpop.i64`; wasm `i64.popcnt`.
  - `rt.ctz(w)`, `rt.clz(w)` (64 for 0): arm64 `rbit`+`clz`, `clz`; LLVM
    `llvm.cttz`/`ctlz` (zero defined); wasm `i64.ctz`, `i64.clz`.
  - `rt.bytes_eq16(b, i, c)`, `rt.bytes_high16(b, i)`: 16 bytes compared
    with a byte (or their high bit), as a mask of 4 bits a byte. arm64
    `ldr q30`, `movi`/`dup v31`, `cmeq` (or `cmlt #0`), `shrn #4`,
    `fmov` (v30 and v31 are the emitter's scratch registers); LLVM a
    `<16 x i8>` compare, sext, and a `<8 x i16>` shift and truncate,
    which clang 21 selects as the same five instructions; wasm two
    words (SWAR: the runtime does not enable SIMD128).
  - `rt.rs_put_word(s, i, w)`, `rt.rs_set_len(s, n)`: a store of 8 bytes
    into a new string with room, and the string with a length (one
    `str`; no instruction). Only Bytes.to_hex uses them.
- std/native/llrt/scan.xo: find_byte, find (strings.Index), eq_at,
  contains, starts_with, ends_with, has_at, count_cont, rune_count,
  span (rt.bytes_span's classes), split, lines, to_hex, utf8_valid,
  bytes_rune_count. A range that ends at least a step into its value is
  finished by the step that ends at its end, its bytes before the range
  shifted out of the mask; only bytes of the value are read.
- Callers: the native prelude (Str.find, contains, starts_with,
  ends_with, trim_prefix, trim_suffix, split, lines, rune_count,
  Bytes.rune_count, Bytes.to_hex), strings.xo (replace), unicode.xo and
  text.xo (span), jsontext.xo (utf8_valid), regexp.xo and report.xo
  (find), toml.xo and csv.xo (has_at, no copy of the rest), and both SSA
  builders for Bytes.to_str (`llrt_utf8_valid`, internal/ssa scanRT,
  selfhost/ssa json_rt).

## Final (commit f2f48eb9)

| Workload | arm64 Xo | arm64 C | ratio | less setup | LLVM Xo | LLVM C | ratio | less setup |
|---|---|---|---|---|---|---|---|---|
| `none` | 24,561,574 | 24,635,414 | 0.997 |  | 21,448,945 | 21,512,727 | 0.997 |  |
| `split` | 99,172,506 | 111,555,939 | 0.889 | 0.858 | 88,254,474 | 89,909,222 | 0.982 | 0.977 |
| `lines` | 160,727,241 | 162,986,859 | 0.986 | 0.984 | 143,006,969 | 144,396,166 | 0.990 | 0.989 |
| `rune_count` | 42,816,534 | 80,576,690 | 0.531 | 0.326 | 39,360,844 | 77,012,835 | 0.511 | 0.323 |
| `utf8` | 116,296,764 | 131,733,133 | 0.883 | 0.857 | 107,363,033 | 128,208,218 | 0.837 | 0.805 |
| `hex` | 107,515,749 | 106,685,547 | 1.008 | 1.011 | 90,848,087 | 95,157,309 | 0.955 | 0.942 |
| `quote` | 142,925,797 | 149,479,002 | 0.956 | 0.948 | 127,227,632 | 139,488,189 | 0.912 | 0.897 |
| `replace` | 87,446,738 | 87,102,684 | 1.004 | 1.007 | 70,933,136 | 77,144,713 | 0.919 | 0.889 |
| `trim` | 166,611,696 | 166,716,763 | 0.999 | 1.000 | 157,308,270 | 157,380,925 | 1.000 | 1.000 |

The bar (decision 0180: within 5 percent of the C path, on both
backends) holds for every workload. The four that decision 0193 recorded
as lost: **split 0.858x and 0.977x** (was 1.09x on LLVM), **lines 0.984x
and 0.989x** (was 1.13x on LLVM), **rune_count 0.326x and 0.323x** (was
1.13x on arm64), **hex (Bytes.to_hex with from_hex) 1.011x and 0.942x**
(was 1.09x on arm64). The other workloads of bench/perf/text, which do
not use the scans, are within 0.983x to 1.015x (zone_fmt 1.015x on arm64,
debug 1.011x; neither calls scan.xo, so the difference is layout).
`utf8` is a new workload (Bytes.rune_count and Bytes.to_str of the
`strs` texts).

## How it got there

1. First version (16 byte steps; split and lines one find_byte call per
   piece; to_hex 16 bytes a call of rt.rs_push_words), less setup:
   split 0.916x and 1.066x, lines 1.055x and 1.090x, rune_count 0.325x
   and 0.324x, hex 1.333x and 1.186x, quote 0.938x and 0.865x, replace
   0.960x and 0.851x. The bench program measured the second version
   unchanged at first: `.tools/bin/xo` embeds std/native, so it must be
   rebuilt after a std edit (gotchas/xo-binary-embeds-std-native.md).
2. split and lines take every match of a 16 byte step in turn (one scan
   of the text instead of one find per piece), full steps then one tail
   step, and the mask's one bit a byte is `0x1111111111111111` (a
   logical immediate on arm64; `0x8888...` is negative and cost a four
   instruction constant each step): split 0.860x and 0.979x, lines
   0.984x and 1.002x. to_hex with the second word hexed only when
   needed: 1.139x and 1.040x.
3. to_hex storing its digits into a new string (rt.rs_put_word) instead
   of two rt.rs_push_words calls for a 17 byte input: 1.010x and 0.946x.
4. A SWAR only variant (8 bytes a step through rt.bytes_word and an
   exact zero byte test, no rt.bytes_eq16), measured to decide whether
   the vector step is needed: lines 1.420x and 1.210x, split 0.954x and
   1.034x, replace 1.054x on arm64. So the 16 byte step stays; the word
   step serves texts under 16 bytes and count_cont, where popcount over
   a word already gives 0.326x.

## Correctness

- TestScanDiff (internal/llvm/scan_test.go): 82,097 input lines (needles
  of 1 to 20 bytes at every offset of texts of 0 to 70 bytes, near
  misses, separators of 0 to 3 bytes, \r\n, quotes, control bytes, 0x7F,
  multibyte runes, 3,000 random UTF-8 texts and byte strings, 27 invalid
  or edge UTF-8 sequences at 21 offsets each), the Go backend the
  reference, arm64 and LLVM, Xo and C paths, the LLVM Xo build free of
  leaks. A planted off by one in the tail mask failed 77,235 lines.
- Golden scan_table (internal/llvm/testdata/golden): Go backend, LLVM,
  arm64, arm64 with share probes, both paths; str_ops_table, text_table,
  unicode_table, native_display, json_text, regexp_table, show_table,
  csv_basic, toml_basic, codex_gaps, misc on both backends; TestJSONTextDiff;
  cmd/xo TestNoGCTests prop and same.
- TestSelfhostArm64, TestSelfhostLLVM, TestSelfhostSSA over eleven of
  those goldens: every function identical; TestSelfhostMachoXoTests with
  the new encodings (checked against clang 21); TestSelfhostWasm 130 pass,
  19 unsupported, 0 fail of 149 (str_ops_table, native_prelude, and
  scan_table first failed on a missing `i64.popcnt` opcode in the self
  hosted WAT assembler, and newtypes on a `$eq8` helper name the wasm
  backend also generates for type equality;
  gotchas/wasm-rt-name-collides-with-generated.md).
- After merging origin/main (10e71072, with decision 0200's atomics and
  the wasm rotates): the same targeted runs pass; TestSelfhostWasm 132
  pass, 19 unsupported, 0 fail of 151.
