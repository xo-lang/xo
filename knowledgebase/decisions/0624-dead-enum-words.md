# 0624: Dead enum words are not written

Status: Accepted (lead, 2026-11-12, with the conditions below; Xo Lang 3).
Implementation pending: behind a pass flag, off by default.

## Context

The arm64 and x86 backends build an enum value as its tag word, the
active variant's payload words, and a zero for every word the largest
variant needs beyond those (compiler/arm64 isel.xo `variant()`). A
function returning `Result[T, E]` therefore writes xzr into all of T's
words at every Err exit, and a list element or a struct field stores them
too. On aws-43's gcp BigQuery sample (whole program, XO_INLINE_STRICT=1)
the binary has 27,562 `str xzr, [sp, ...]` (110 KB); the derived
decoders alone have 4,895. No comment or ADR documents the zeroing.

## Decision

Words of an enum value outside its active variant's payload are left
unwritten (a DEAD marker that stores and moves skip), on arm64 and x86.
LLVM keeps `zeroinitializer` and stays the zeroing reference.

The audit (~/xo-lead/notes/lang3/dead-enum-words-audit.md, 2026-11-12)
found no reader of those words that does not first check the tag:
reference counting (rc.xo agg_rc, one_leaf_enum, agg_rc_at, leaf_call,
x86rt.xo x86_agg_rc), equality (closures.xo eq_at; plain_words excludes
Enum from the struct byte compare), hashing (ssa map.xo), Any, and the
in-process debugger all dispatch on the tag; there is no tracing GC.

Conditions (lead):

1. A pass flag, off by default at first; arm64 and x86 only.
2. The same push carries XO_DEAD_POISON=1 (dead words get an odd poison
   pattern instead of nothing) and an enum golden (List, Map, struct;
   copy, eq, hash, drop, heap counts); the arm64 goldens, the std module
   tests and the fixed point run under poison, with leakcheck and the
   debug heap.
3. plain_words' Enum exclusion is pinned by a comment and a test, so a
   later struct byte compare or byte hash cannot read dead words.
4. Turning it on by default is a separate push under the default-change
   rule: the touched packages' full suites, poison over goldens, std and
   the fixed point, sizes (__text + _ptext on bq, cal, aws; xo's
   __text), and one `xo bench --ab`.
5. The bytes are never written to I/O: no enum crosses the C ABI
   (std/native/llrt cabi.xo: Int, Float and Str only) and no serializer
   copies raw words. A later byte-level serializer, or struct passing
   through `use c`, must zero or skip dead words.

## Consequences

- Stored bytes in list elements and result areas become run dependent;
  nothing reads them as values.
- lldb and gdb show garbage in an enum's inactive union members (DWARF
  union, compiler/arm64 debug.xo dw_enum); display only.
- A constant variant of 32 or more words (zimage.xo ZERO_IMAGE_MIN) needs
  its dead words as `.quad 0` in the image so it keeps the memcpy path.

## Alternatives

- A static zero image plus filling only the read fields (measured
  2026-11-12: a memcpy costs at least 20 B per exit, more than the stores
  it replaces for two constant words).
- Keep zeroing (the default until condition 4).
