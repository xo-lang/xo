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

## Implementation (stage 1, Xo Lang 3, 2026-11-13)

- compiler/arm64 isel.xo `dead_from`: a Return's argument that is an
  Op.Variant has its words from 1 + the payload's words on dead; the
  K_RET operand is S_DEAD (mir.xo), which emit.xo's return moves skip
  (register and stack result words), and byref.xo `ret_sret` stores only
  the live words. arm64 only for now (dead_from is -1 on x86), returns
  only: list elements and struct fields still store every word.
- XO_PASSES=+deadw turns it on (passes.xo, off by default);
  +deadw_poison stores 0xDEADBEEFDEADBEEF in each dead word instead, as a
  pass name rather than a new environment variable, so it needs no new
  codegen input and is in the cache keys already.
- golden enum_dead_words (returns of each variant in registers and on
  the stack, List, Map and struct stores, copy, eq, hash, drop): the same
  output off, +deadw and +deadw_poison, on LLVM and Go, and XO_LEAKCHECK=1
  reports 0 live objects.
- closures.xo plain_words says why an Enum is never plain;
  plain_words_test.xo pins it.
- gcp BigQuery sample (whole program, XO_INLINE_STRICT=1): 4,259,996 to
  4,213,288 (-1.1%) with +deadw; Calendar -6.2 KB, aws presign -8.6 KB.

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
