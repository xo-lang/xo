# A new caller of a large function uninlines it elsewhere

Status: Observed (2026-10-14, zero-C audit B13 stage 2,
bench/results/string-list-objects-2026-10-14.md).

## Symptom

A change that only adds Xo code made unrelated workloads slower: with
std/native/llrt/obj.xo calling `alloc.alloc` and `alloc.free` from new
places, bench/perf/alloc strings (no list code at all) took 1.023 times
the instructions of the base on arm64, text split 1.051, lines 1.037.
`otool -tV` showed `cabi.heap_alloc` shrink from 73 instructions to 4
and `cabi.heap_free` from 17 to 4: they now branch to alloc.alloc and
alloc.free instead of holding a copy, so every C allocation (xo_alloc,
xo_free: the C string objects) pays a level more.

A second case of the same kind, in clang: once the lists left xo_rt.c,
`rs_append` was `xo_resize`'s only caller and clang inlined it there
(rs_append 182 to 402 instructions), so the in place append saved more
registers on every call (LLVM json enc_small 1.051, text replace 1.028).

## Cause

selfhost/arm64/inline.xo copies a function over its size limits
(INLINE_MAX_INSTRS 40, INLINE_MAX_BLOCKS 12) only when it has at most
INLINE_FEW_SITES (2) call sites in the whole program and the copies fit
INLINE_FEW_BUDGET. The count is program wide, so a third caller anywhere
makes the function out of line at every site, including hot ones that
had it inlined before. clang's heuristics differ, but a static function
with a single caller is inlined whatever its size.

## Fix

- Keep the call sites of the heap's large functions at two: obj.xo
  reaches alloc.alloc only through `halloc` and alloc.free only through
  `free1` (small, so inlined into its own callers without counting
  against the limit). `halloc` stays out of line by calling itself on a
  path never taken (a function on a call cycle is never inlined).
- Mark a C helper `__attribute__((noinline))` when a change leaves it
  with one caller on a hot path (xo_resize).
- When an unrelated workload moves, compare per function sizes of the
  two binaries: `otool -tV bin | awk '/^_[A-Za-z0-9_.$]+:$/{f=$1;next}
  f{c[f]++} END{for(k in c) print k, c[k]}' | sort`, then `join` the
  two lists and look for functions that shrank to a branch.
