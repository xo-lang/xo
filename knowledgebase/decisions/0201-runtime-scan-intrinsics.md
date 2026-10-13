# 0201: SIMD and word at a time scan intrinsics for the runtime

Status: Accepted (2026-10-13, simd agent; decision 0195 prerequisite 4).
Implemented: `std/native/rt.xo` (the intrinsics),
`std/native/llrt/scan.xo` and its C path `scan_c.xo` (`XO_LLRT_C=scan`),
`internal/arm64/isel.go` and `selfhost/arm64/calls.xo` (inlineRT /
inline_rt), `internal/llvm/fromssa.go` and `selfhost/llvm/call.xo`,
`gen.xo` (the same, and the ctpop/cttz/ctlz declarations),
`selfhost/wasm/rt2.xo` and `asm.xo` (WAT and `i64.popcnt`),
`selfhost/macho/enc.xo` (the instructions), `internal/ssa/build_io.go`
and `selfhost/ssa/call.xo` (Bytes.to_str). Evidence:
`bench/results/scan-intrinsics-2026-10-13.md`.

## Context

Decision 0193 kept Str.split, Str.lines, Str.rune_count, and
Bytes.to_hex in C because their Xo versions missed decision 0180's bar
(split 1.09x and lines 1.13x on LLVM, rune_count 1.13x and to_hex 1.09x
on arm64): a C call to search and one to copy per piece cost more than
the C loop's memchr, and Xo's byte loops cost more than C's. Decision
0195 (no compiled C in the runtime) lists SIMD as prerequisite 4, and
the zero C audit (bench/results/zero-c-audit-2026-10-13.md, section
"(4) SIMD and word scans") sketched the intrinsics. Decision 0074 limits
`native/rt` to std's implementation modules: nothing here is user
visible.

## Decision

1. **Intrinsics** (std/native/rt.xo), lowered inline by both arm64
   emitters, both LLVM emitters, and the wasm runtime; never a call:
   - `rt.popcount(w)`, `rt.ctz(w)`, `rt.clz(w)` (64 for 0): arm64
     `cnt`/`addv`, `rbit`+`clz`, `clz`; LLVM `llvm.ctpop/cttz/ctlz.i64`;
     wasm `i64.popcnt/ctz/clz`.
   - `rt.bytes_eq16(b, i, c)` and `rt.bytes_high16(b, i)`: the 16 bytes
     from i compared with the byte c (or tested for the high bit), as a
     64 bit mask of 4 bits a byte (the lowest byte lowest). arm64: `ldr
     q30`, `movi` (constant c) or `dup v31.16b`, `cmeq` (or `cmlt #0`),
     `shrn v30.8b, v30.8h, #4`, `fmov`; v30 and v31 are scratch
     registers of the emitter, which never allocates d29 to d31. LLVM:
     a `<16 x i8>` load and compare, sext, a `<8 x i16>` shift right by
     4, truncate to `<8 x i8>`, bitcast to i64, which arm64 clang selects
     as the same instructions. The mask format is the one shrn makes
     cheaply (memchr's), not a bit a byte. Unchecked: 0 <= i, i + 16 <=
     b.len().
   - `rt.rs_put_word(s, i, w)` and `rt.rs_set_len(s, n)`: an 8 byte store
     into a new string from `rt.rs_with_cap` with room, and that string
     with a length (one `str`; nothing). They follow the `rs_` rule (the
     string is consumed and returned). Only Bytes.to_hex uses them; they
     are the "32 byte push primitive" of decision 0193 step 3 (d) in the
     raw memory style of the audit's (2), limited to a string the caller
     just made.
2. **Order of the work, as asked**: (a) word at a time (SWAR) over
   `rt.bytes_word` with popcount, ctz, clz; (b) NEON / `<16 x i8>`; (c)
   wasm SIMD128 only if the wasm runtime enabled it: it does not, so
   wasm computes the 16 byte masks from two words in WAT. The SWAR only
   variant missed the bar on lines (1.420x arm64, 1.210x LLVM), so the 16
   byte step is used where a run can be long; the word step serves texts
   under 16 bytes and the continuation count, where popcount over a word
   is enough (0.326x).
3. **std/native/llrt/scan.xo** (decision 0193's mechanism: a runtime
   module and its C path `scan_c.xo`, `XO_LLRT_C=scan`, the Go checker
   only): find_byte, find (strings.Index), eq_at, contains, starts_with,
   ends_with, has_at, count_cont, rune_count, span (rt.bytes_span's
   classes), split, lines, to_hex, utf8_valid, bytes_rune_count. Reading
   past a range is never needed: a range shorter than a step that ends
   at least a step into its value is read as the step that ends at its
   end, the bytes before the range shifted out of the mask. split (one
   byte separator) and lines take every match of a step in turn rather
   than one search per piece. Bytes.to_str calls the native prelude's
   `llrt_utf8_valid` (internal/ssa `scanRT`, selfhost/ssa `json_rt`).
4. **The bar holds on both backends** (bench/perf/text, instructions less
   setup, Xo over C, arm64 and LLVM): split 0.858x and 0.977x, lines
   0.984x and 0.989x, rune_count 0.326x and 0.323x, hex 1.011x and
   0.942x, utf8 (new) 0.857x and 0.805x, quote 0.948x and 0.897x, replace
   1.007x and 0.889x. So the Xo path is the default; per decision 0193
   item 5 the C path stays selectable until the lead's full tier passes,
   then decision 0198 deletes it.
5. **Not ported here** (the audit's (4) rows that need more than a
   scan): `xo_str_eq`/`xo_str_cmp` (the backends call them directly for
   `==` and ordering; an inline compare over eq_at is the next step),
   the copies (`str_slice`, `str_repeat`, `str_join`, `rs_append`: (2)
   raw memory), `bytes_decode` (a call per rune from unicode.xo, which
   has its own Xo decoder).

## Consequences

- The C functions xo_str_split, xo_str_lines, and xo_bytes_hex are called
  only by scan_c.xo; xo_str_find, contains, starts/ends_with,
  bytes_span, utf8_valid, and the rune counts remain called by other C
  and by the wasm runtime's own versions. Moving the first three under
  `#ifdef XO_SCAN_C` with an IOFlags entry is left to the deletion step
  (decision 0198) to keep this change out of xo_std.c and xo_bytes.c,
  which other ports edit now.
- The wasm backend runs the Xo scans (TestSelfhostWasm 130 of 149, from
  128): its WAT str_split and friends are no longer reached from the
  prelude.
- The arm64 emitters now write SIMD registers (v30, v31) in templates;
  selfhost/macho encodes `ldr q`, `cnt`, `addv`, `dup`, `movi`, `cmeq`,
  `cmlt`, `shrn`, `rbit`, `clz`.
- SSA goldens that print the prelude (closures, failing) change with any
  edit of scan.xo.

## Alternatives

- **Whole scan loops as intrinsics** (`rt.bytes_find_byte` lowered to a
  loop by each emitter, as the audit named them): four emitters and the
  WAT would each carry the loop; a few primitive operations and the
  loops in Xo give wasm the code for free and keep one source.
- **A bit a byte mask** (x86's pmovmskb, wasm's `i8x16.bitmask`): on
  arm64 it needs and/addp reductions (several more instructions per
  step); the nibble mask is three.
- **A wider push primitive for to_hex** (`rs_push_words` of 8 words): one
  call fewer for 17 to 32 byte inputs but still a call per 32 digits and
  a new C function; the store into a new string has no call at all.
- **SWAR only**: simpler emitters, but lines missed the bar by 42
  percent on arm64.
