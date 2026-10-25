# 0369: pack the elements of a List of a sized integer, Float32 or Rune

Status: Proposed (Xo Lang 3, 2026-10-24; for the owner's Sunday review,
nothing built)

## Context

A `List[T]` element is 8 bytes for every integer width on the native
backends (arm64 and amd64, `G.esize`, compiler/arm64/gen.xo) and on LLVM
(`Gen.layout`, compiler/llvm/gen.xo): the SSA type table has one `Int` kind
for `Int8` to `UInt64`, `Rune` and `Duration`, and one `Float` kind for
`Float32`; only `Bool` has a 1 byte element. `Bytes` is its own packed type.
The Go backend maps `UInt32` to Go's `uint32`, so it packs. The spec said
nothing (core.md 2 now has a representation note; gotchas/
sized-int-lists-take-eight-bytes.md).

**Found by:** the Str sort kept its index lists as `List[UInt32]` to cut the
merge phase's memory (bench/results/sortstr-2026-10-24.md). `--ab`, 15
rounds, clean xos: max RSS 21424 against 21440 KB (1.001), wall 1.028 within
noise, instructions 1.002. The cause is the representation, not the sort.

**Measured** (probe: 10,000,000 pushes of `i % 1000` into a
`List[UInt32]` and into a `List[Int]`, `/usr/bin/time -l` max resident set
size, one xo, 2026-10-24):

| backend | List[Int] | List[UInt32] |
|---|---|---|
| native arm64 | 82,706,432 | 82,706,432 |
| LLVM | 82,788,352 | 82,804,736 |
| Go | 91,717,632 | 51,757,056 |

So 8 bytes an element natively (the 80 MB of lists is the whole difference
from an empty program), and the Go backend, with Go's slice growth, packs.
A `List[UInt8]` of 1,000,000 elements is 8 MB against 1 MB as `Bytes` or as
a packed list. The output is the same on all three; only memory and cache
traffic differ.

## Typical uses that pay

- Byte like lists: `List[UInt8]` (a decoded pixel row, a table of states, the
  result of mapping over a `Bytes`), 8 times too big.
- Sized integer tables: lookup tables of `UInt16`/`UInt32` (CRC, Unicode
  ranges, perfect hash seeds); small, so memory is not the cost, cache
  footprint is.
- Index lists in algorithms: the Str sort's `a`, `b` (n indices below 2^32;
  the saving would be 8n of its 24n byte merge phase for 200k Strs, about
  1.6 MB of 8 MB), argsort results, graph adjacency, `List[Int32]` ids.
- Lists of `Rune` (a `Str` decoded to code points) at 8 bytes against 4.
- Not helped: `List[Int]`, `List[Float]`, `List[Str]`, structs (a struct of
  two `Int32` is two words; this ADR leaves aggregates alone).

## Decision (proposed)

Give the element type a width. A list of a type whose value is one 64 bit
word and whose range is narrower stores `esize` 1, 2 or 4 bytes (`Int8`,
`UInt8`, `Int16`, `UInt16`, `Int32`, `UInt32`, `Float32` as an `f32`,
`Rune` as 4), a register still holds the widened value.

- SSA: the type table keeps the integer width (a `width` and `signed` on the
  `Int` kind, or a Kind per width) so `rt.list_esize` and the element
  load/store lowerings can see it; everything that compares kinds (inlining,
  cse, constant folding) keeps treating them as integers.
- arm64: `G.esize` returns the width; element loads become `ldrb`, `ldrh`,
  `ldr w` and the signed `ldrsb`, `ldrsh`, `ldrsw`; stores `strb`, `strh`,
  `str w`. Each replaces the 8 byte `ldr`/`str` one for one, so no added
  instruction; the index scale is `lsl #0..#2` rather than `#3`, which the
  addressing mode already carries. `Float32`: `ldr s`/`str s` with `fcvt`
  to and from the register's double (one extra instruction a load and a
  store).
- amd64: `movzx`/`movsx`/`mov r32` loads and `mov` of the narrow register as
  the store (x86rt.xo and the x86 emitter use `g.esize` already for the
  runtime calls).
- LLVM: the list's element becomes `i8`/`i16`/`i32`/`float` in the typed
  GEP, a `zext`/`sext` after the load and a `trunc` before the store; LLVM
  folds them into `ldrb`/`movzx` itself.
- Runtime: every routine that takes `esize` (push, insert, concat, slice,
  sort, compare, hash, equality, `to_bytes`, JSON and the codecs that walk a
  list's memory) already receives it from `rt.list_esize`; each one that
  reads elements as words rather than through `esize` needs a pass (a grep
  of std/llrt and std/native for word loops over list memory is step 1).
- `Bool` is the precedent: 1 byte, widened on load, already in all three.

Overflow semantics are unchanged: a value is range checked where it is
made (`UInt8(x)` faults), so a narrow store never truncates a legal value.

## Cost

- Work: SSA type width plus four lowerings plus an audit of word loops over
  list memory; the audit is the risk (a missed word loop reads 8 bytes of a
  4 byte element, silently). Roughly one ADR step per backend after the SSA
  step; the native seed would need the minor version bump rule of
  CLAUDE.md only if the compiler's own code used packed lists (it does not
  have to).
- Speed: loads and stores are one instruction as before; scans over a
  packed list touch 2 to 8 times fewer cache lines (to be measured with
  `--ab`, not claimed), `Float32` pays one `fcvt` a load or store.
- Compatibility: no source change; memory, not output, differs. The
  cache key (cache_id) changes with the backend sources as always.

## Measurement plan (before building)

1. A hand written `Bytes`-backed experiment is not equivalent; the proof is
   a `List[UInt8]` sum over 10M and a 1M `List[UInt32]` argsort in a
   scratch branch where only `G.esize` and the arm64 element loads and
   stores change, `--ab` against main, with max RSS and wall. Stop if the
   scan is not faster or the sort index case is slower than 1.0.
2. The audit grep above, with the count of word loops found.

## Alternatives

- Do nothing, document it (this commit: the gotcha and the spec note).
- A `PackedList[T]` or `Buf[UInt32]` type: no change to `List`, but a second
  list API for every algorithm (sort, search, join) and nothing existing
  benefits.
- Use `Bytes` for byte lists: already the answer for bytes; it covers neither
  `UInt16`/`UInt32` nor `Rune`.
- Pack in the Go style only for lists above a size: two representations per
  type, a branch on every access.
