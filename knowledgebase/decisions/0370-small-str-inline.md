# 0370: store a short Str's text inside its 16 byte value

Status: Accepted as an opt-in flag (owner, 2026-10-25): `xo build --inline-str`,
arm64 only, default off. The measured trade is below; it does not meet the
bar for a default (about 1% code size, wall within noise on the Str, json and
http benches). Proposed by Xo Lang 3, 2026-10-24.

## Context

A `Str` is two words (a pointer and `len | off << 32`, decision 0330; the
pointer's low bit is already a tag) pointing at a heap string object: a 16
byte header (reference count and capacity, `S_HDR`, std/native/llrt/obj.xo),
then the text. The allocator adds an 8 byte block header and rounds the
block up to a size class (16 byte steps to class 64).

**Found by:** the 2,000,000 Str sort probe (bench/results/
sortstr-2026-10-24.md): building the Strs alone is 114.7 MB max RSS against
Go's 77.0 MB (1.49), before the sort adds anything.

**Measured** (one xo, COMMIT 0a49d0560, DIRTY empty, 2026-10-24, `"key${n}"`
Strs of 4 to 10 bytes in a `List[Str]`):

| N | Xo max RSS | Go max RSS |
|---|---|---|
| 500,000 | 30.7 MB | 22.8 MB |
| 1,000,000 | 58.7 MB | 40.8 MB |
| 2,000,000 | 114.7 MB | 76.8 MB |

Slope 1M to 2M: Xo 56.0 bytes a Str, Go 35.9. The Xo bytes are the 16 byte
value in the list, plus one object: `XO_HEAPSTATS=1` after dropping 100,000
Strs lists them as class 2, size 32, 100,000 free blocks, 4,000,000 bytes,
so 40 bytes with the block header (16 header + up to 16 text bytes in a 32
byte class). 24 of the 56 bytes are overhead Go's string headers
(16 bytes, no per string object header) and its tighter classes avoid. One
long Str costs little: a 2,000,000 byte text is 5.87 MB max RSS against
2.69 MB for an empty program.

## Decision (as proposed; accepted only as the opt-in flag, see Status)

A Str of at most 15 bytes keeps its text in the value itself: the two words
hold the bytes and a length/tag in the last byte (tag bit in the pointer's
low bit, which is free for a real pointer: objects are 8 byte aligned). No
object, no reference count, no free. Longer Strs and every view of a long
Str are unchanged. A string builder (`reserve`, `rs_new`) promotes to an
object on its first append past 15 bytes.

Expected saving: 40 of the 56 bytes for a 4 to 10 byte Str (about 14 B/Str
left against Go's 35.9; the heap block disappears, so it is also one
allocation and free fewer per Str).

## Costs and what must be checked first

- Every Str operation on three backends (arm64, amd64 native and LLVM)
  branches on the tag: retain and release (no object when inline), equality,
  hash, compare, slice (a view of an inline Str is a new inline Str),
  concat, index, `raw.data`/C string conversions (the text must be
  addressable: inline text lives in a stack slot or register pair, so
  `raw.data` needs a spill rule), FFI (`use go`), `Bytes`/`Str` conversion,
  json and the Str sort's gathers (`list_sort_str`).
- Uniqueness checks (`reserve`'s "unique, room") must treat inline as
  unique and copy-on-grow.
- The leak check and `heapstats` counts change meaning (fewer objects).
- The Go backend is unaffected (Go strings).
- Hot paths (map keys, `Str ==`) gain a tag test and lose a load; the
  net is not known.

## Measurement plan (step 1, nothing built)

1. A hand model in one file: a `ShortStr` struct of two Ints with the
   operations above, `--ab` against `Str` on a 2M key build, a map of 1M
   keys and the sort (instructions, wall, max RSS), quiet window.
2. Audit: `xo refs` over the string entry points in obj.xo and the three
   backends; count the sites that assume an object.
3. Go/no-go from the model: max RSS at 2M below 1.2 of Go's, hot Str
   benchmarks (bench/perf strings, json, maps) within noise.

## Step 1 measured: the hand model (2026-10-25)

A `ShortStr` of two Ints (up to 15 bytes of ASCII, big-endian, the length in
the low byte of the second word; `derive Eq, Hash, Ord`) against `Str` for
the keys `"key${n}"` (4 to 10 bytes), N = 2,000,000, `--release` xo, max
RSS from `/usr/bin/time -l` (the wall is one unquiet run, not a result):

| Workload | Str | ShortStr | Change |
|---|---|---|---|
| a `List` of N keys, built and summed | 137.2 MB | 57.2 MB | -58% (Go: 76.8 MB) |
| a `Map` of N keys, then N lookups | 227.7 MB | 147.7 MB | -35% |
| the list sorted (`sort()`) | 201.3 MB | 89.3 MB | -56% |

So the memory side holds: 28.6 bytes a key in the list (16 for the value, the
list's growth slack, the baseline 2.7 MB), under Go's 35.9, and the Map and
sort peaks fall with it. The model does not show the cost: a struct of two
Ints has no tag test, no `raw.data` and no object path, which is where the
real implementation pays (every Str operation branches on the tag). The wall
numbers (build 0.17 against 0.19 s, map 0.81 against 0.82 s) are inside one
unquiet run; the sort (0.47 against 0.12 s) compares the Str sort with a
generic `Ord` sort of integers and says nothing about the final design.

Audit (step 2, counted not read): references to the Str layout or kind
outside tests: compiler/ssa 210, compiler/arm64 48, compiler/llvm 26,
std/native/llrt 134, std/native 151 (`xo refs` and grep for `Kind.Str`,
`str_t(`, `"str.`, `S_HDR`, `str_at`, `str_len`, `is_str`); the SSA's string
operations are `str.concat` (43 sites), `str.concat_int`, `str.of_bool`,
`str.eq`, `str.cmp`, `str.concat_float`. The unknown with the most weight is
`raw.data(s)`: every FFI, syscall and `use c` site takes the address of the
text, which an inline Str does not have; it needs the spill rule above and
an audit of the std/native callers.

## Prototype measured (2026-10-25, branch si, `--inline-str`, arm64 only)

The prototype keeps a Str of at most 11 bytes in its two words (bit 0 of
the first word is the tag; the first seven bytes in its bits 8 to 63, the
last four in the high half of the second word, whose low half is the length).
The 11 byte limit is the 16 bytes minus the tag byte and the 4 byte length
(a 15 byte form needs the length in the tag byte and a length branch in every
operation: the hand model's 15 was a model, not this layout). Producers
(concat, interpolation, push, view, slices) make inline results when they
fit; `Str.bytes()` of an inline Str is an object (Bytes is never inline)
unless every use only reads it (borrow, isel `reads_borrowed`: Bytes.len,
is_empty, b[i] and reference counts; any call, store, return or block
argument is an escape whatever the callee). A text address (`raw.data`)
of an inline Str points at a spill in the caller's frame, so a function that
keeps the address beyond its call copies the text first (h2srv `fields_of`,
the socket write path takes `.bytes()`).

Correctness (all with the flag): 377 of 377 llvm goldens (including the new
`net_short_str` for the poller write path and `inline_bytes_escape` for a
short Str's `bytes()` stored, returned and captured), the arm64 run corpus
(26), the std corpora (38, `xo test --inline-str`), stdfuzz seeds 1, 7 and 11
(5200, 7800 and 7800 case lines) byte-identical to the unflagged build, plain
and `--release`. Not done: a flagged `internal/arm64` and `cmd/xo` Go test
run (the Go tests pass no flags), the audit of all 305 `raw.data` and
`bytes_addr` sites (call-scoped by construction except the ones named above),
amd64 and LLVM (the flag is arm64 only).

Memory (quiet window 08:50, 20 rounds, flag on against off, `--release`):
1M `"key${n}"` list 63.0 to 23.9 MB (0.38), `Map[Str, Int]` 124.4 to 85.3 MB
(0.69), sort 102.1 to 63.0 MB (0.62), json bench 0.95.

Speed (same window, B/A; instructions are deterministic): map wall 0.84 and
sort 0.75 (less memory traffic) but json wall 1.073 (instructions 1.065),
build 1.22, text 1.53 (instructions 1.80: `text.parse_int` begins with
`s.bytes()`, which allocated an object for every short digit string; fixed by
the borrow, text now 1.09). Instruction ratios after the borrow, per bench:
strsplit 0.999, bytessearch/bytessplit/bytespredicate/trimchars 0.995 to
1.003, unicode 1.010, httpparse 1.011, bytestransform 1.012, timeformat 1.024,
json 1.057 (enc_small 1.22, maps 1.19, strings 1.14, ints 1.11, parse 1.01),
text 1.09, jsontime 1.12, netip 1.21. The bar (no wall regression beyond
noise on Str, map, json and http benches) is NOT met yet: the string heavy
encoders and parsers pay a tag test and a spill per Str operand, and a
promotion (`bytes()` passed to a call) per short Str.

Why 11 bytes and not 15 (measured as shares, not built): the 11 byte layout
keeps the length where every consumer of a Str reads it, the low 32 bits of the
second word, so `Str.len`, the bounds checks, `str_len` in every backend and the
runtime's `an & LEN_MASK` are unchanged and a tag test is the only addition. A
15 byte form (7 bytes in the first word above the tag, 8 in the second) has to
put the length in the tag byte, so every read of the length becomes a branch or
a select on the tag (about three instructions at each of the hundreds of sites
that read it, and in every bounds check), and an inline Str can no longer be
treated as a view with an offset. The gain is the share of Strs that fit: in
the repository's json and http bench sources 71% fit 11 bytes and 78% fit 15
(approximate; source literals), for common HTTP header names 50% and 82%. The
2M key test (4 to 10 bytes) fits either way. With the instruction overhead
this prototype already shows (json 1.06, text 1.07, netip 1.12), a form that
adds a length branch to every operation is the wrong direction; if the 11
byte form is adopted, a 15 byte one can be measured later behind the same tag.

Corpus share (source literals of bench/perf json and http, 249 strings, an
approximation, not a runtime corpus): 71% fit 11 bytes, 78% fit 15; common
HTTP header names (28): 50% and 82%. The 2M test's keys (4 to 10 bytes) all
fit either way.

Code size (2026-10-25, release, darwin/arm64, bench/perf/http/xo `__text`):
1,410,412 bytes without the flag, 1,544,984 with `--inline-str` (+9.5%, file
1.9 to 2.1 MB), against a bar of about 1%. `--pack-lists` alone: no change.
The growth is in the 126 KB added to functions that exist either way (14 KB
are new helpers): 3345 tag tests, 846 spill blocks of about five instructions,
184 inline views of about 37 instructions (roughly a quarter of the growth),
935 calls of the `inl_` runtime helpers. Ranked ways to shrink it: constant
lo views shorter and variable ones back to the runtime call; the spill block
as one shared helper; tracking fresh, literal and heap-known Strs in the SSA
to drop their tag tests; an Xo side fast path in place of call sites.

Flag-off identity (2026-10-25): 48 goldens (every 8th) built with `--release
--debug-info=off` by the merge-base xo (own std) and by the branch xo (own
std) have the same disassembly once addresses, hashes and fault-location
literal-pool comments (std line numbers moved) are masked, except tls_client:
one runtime function at 0x1000e3270 has an LSDA (exception table) entry in
the base build and none in the branch build, which makes `__eh_frame` 8 bytes
smaller and moves later adrp page numbers; `__text`, `__const`, `__cstring`
and `__data` are the same size. OPEN: which function loses its landing pad and
why (the arm64 changes are all behind `inl_str()`; a std or SSA change is more
likely); harmless for correctness, to be explained before landing.

## What did not work (2026-10-25)

- A tag test before each Str drop's call, so an inline Str skips the helper:
  CRUD `__text` +77 KB more and every instruction row worse (netip 1.089 to
  1.109); the helper's own first instruction already is that test.
- A size-aware inliner (limits x0.75 for flagged builds): CRUD `__text`
  -46 KB (+5.1% instead of +8.4%) for +4 to +5 points of instructions (json
  1.10, text 1.10, netip 1.16, jsontime 1.16); raising the limits 25% to keep
  small helpers inlined changed nothing.
- A shared out-of-line spill helper: not viable, the spill must live in the
  caller's frame, so the call costs what the inline cold block does.
- The words of a pushed value returned in registers instead of through the
  frame (a tuple export): more instructions, same cycles.
- A literal prefix as immediate words in the push (`"[" + i`): no change in
  any row, and it moved the unflagged `__text`.
- A scalar short path in `concat_o`: no change (jsontime 1.116 to 1.114).
- A pre-sized object for JSON's output: no change.
- Measurement slips worth knowing: a script whose `cd` failed compared stale
  binaries and reported netip 1.024 (withdrawn); an xo built before a revert
  was used for a later run. Scripts that measure start with `set -eu`, `cd DIR
  || exit 1`, and rebuild both sides in the run.

## What the flag costs and buys (2026-10-25, quiet window, 20 rounds)

Memory (max RSS, flag on over off): a list of 1M `"key${n}"` Strs 0.375,
`Map[Str, Int]` 0.685, its sort 0.616, json bench 0.94. Wall: build 0.77,
sort 0.65, map 0.84 (better); unicode 1.005, strsplit 0.99 (noise); json
1.052, text 1.091, jsontime 1.044, netip 1.154 (worse; netip measured before
the `reserve` change). Instructions: json 1.058, text 1.082, netip 1.089,
jsontime 1.114, unicode 1.011, strsplit 1.003, httpparse 1.011. Release
`__text` of bench/perf/http/xo: 1,410,412 off, 1,529,688 on (+8.4%; +76 KB
without the inliner, so 36% of the growth is inlined copies).

## Flag-off build against main (2026-10-25)

Semantics and the goldens are identical with the flag off, but the object code
is not byte for byte: a release CRUD server's `__text` is 1,444,784 bytes
against 1,444,720 (+64 B), hello 121,560 against 121,496. One function
differs in size: `obj.rs_push_word` is no longer inlined into
`cabi.obj_rs_push_word` (a 30 instruction function of its own, 14 fewer in the
wrapper; 16 instructions net). Ninety other functions differ only in operands
(stack offsets, labels) and the std line numbers that fault-location constants
carry (the guards added lines to obj.xo and prelude.xo; `scripts/identity`
trips on those too). The cause: each `if inlstr.built() && ... { return ... }`
guard folds away at build time but the folded statement `if` leaves one dead
`() = tuple` SSA value (ssa/expr.xo `if_head` returns `b.unit()`), which the
arm64 inliner counts as an instruction. `obj.rs_push_word` went from 43 to 44
instructions, `reserve` from 33 to 34, `push_slow` from 25 to 26: the inliner's
decision classes are unchanged for all 2,799 functions of the CRUD server
(its class is "small" in both builds; the extra instruction is what takes
the copy in `cabi.obj_rs_push_word` over the caller's budget). The general fix
is a separate commit: a folded guard should cost the inliner nothing.

## Alternatives

- A headerless packed class for small strings (an 8 byte header with
  the count in the block header): saves about 16 of the 40 bytes, keeps
  the object model, but still a pointer and an allocation per Str.
- Arena or interned key tables: per-program, not the default.
- Half-size merge scratch in the sort: rejected, the gather peak is the
  same 5n words (bench/results/sortstr-2026-10-24.md).
- Leave it: Go-sized Str lists stay 1.5 to 2.5 times Go's memory.
