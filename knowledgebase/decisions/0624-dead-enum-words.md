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

## Implementation (stage 2, x86, Xo Lang 1, 2026-11-13)

- dead_from no longer returns -1 on x86; x86emit.xo's K_RET skips S_DEAD
  words as emit.xo does (x86_res_locs counts them as GP words, as
  abi_locs does, so later words keep their registers and stack slots);
  ret_sret's stores are machine independent MIR, shared with arm64.
- enum_dead_words on linux/amd64 (xo asm --backend=amd64, against
  XO_PASSES=+shareret, gotchas/xo-passes-baseline.md): 47 fewer lines
  with +deadw (the `movl $0` of dead result registers and stack words);
  +deadw_poison writes the pattern instead. The golden joins the amd64
  per-push sample (scripts/docker-amd64-sample.txt).
- Still off by default, returns only.

## Implementation (stage 3a, list elements, Xo Lang 1, 2026-11-13)

- calls.xo store_elem takes the first dead word (dead_from of the stored
  argument) for list.set, list.push, log.set and log.push: an element
  stored from a Variant literal writes only its live words (or the
  poison pattern). A value from a call or a load is not a Variant op, so
  it still stores every word.
- enum_dead_words pushes and sets Line and Dot literals: 57 fewer lines
  against +shareret on darwin/arm64 (47 from returns, 10 from the
  element stores' zero pairs); the same output in the three modes,
  XO_LEAKCHECK=1 0 live.

## Implementation (stage 3b, structs and tuples, Xo Lang 1, 2026-11-13)

- isel.xo dead_mask: one Bool per word, a Variant's from dead_from and
  a Struct's or Tuple's from its fields' in order (followed to a depth
  of 4; a layout that is not its fields' words in order has none). The
  Return's operands, ret_sret, the sret_mk field stores (byref.xo
  store_live_at: each run of live words by store_words_at, the dead runs
  poisoned with +deadw_poison) and store_elem take the mask.
- enum_dead_words returns and pushes a Holder and a Pair built from
  variant literals and returns a tuple: 95 fewer lines than +shareret on
  darwin/arm64, 143 on linux/amd64; the same output in the three modes
  and on LLVM, XO_LEAKCHECK=1 0 live.

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
