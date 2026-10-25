# 0370: store a short Str's text inside its 16 byte value

Status: Proposed (Xo Lang 3, 2026-10-24; for the owner's Sunday review,
nothing built)

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

## Decision (proposed)

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

## Alternatives

- A headerless packed class for small strings (an 8 byte header with
  the count in the block header): saves about 16 of the 40 bytes, keeps
  the object model, but still a pointer and an allocation per Str.
- Arena or interned key tables: per-program, not the default.
- Half-size merge scratch in the sort: rejected, the gather peak is the
  same 5n words (bench/results/sortstr-2026-10-24.md).
- Leave it: Go-sized Str lists stay 1.5 to 2.5 times Go's memory.
