# 0226: List objects called from generated code without a C level

Status: Accepted, implemented (2026-10-14, B13 stage 2 of the zero-C
audit; std/native/llrt/obj.xo; measurements in
bench/results/string-list-objects-2026-10-14.md).

## Context

Zero-C audit item B13 (decision 0195) moves the list objects (a new
list, growth, the copy of a shared list, the release of a list's
elements, the drop worklist) from std/llrt/xo_rt.c to Xo. The first
port (commit 3da29c8b, reverted by 082476d4) kept the C functions as
entry points: generated code called `xo_rl_grow`, which found the
thread's heap in C and called the Xo export, which called obj.xo, which
called alloc.xo. Each level cost a frame on arm64, and decision 0180's
bar (Xo within 5 percent of the C's instructions) was missed on arm64:
alloc small 1.266 (linux/arm64), 1.201 after shrink wrapping (decision
0224), nested walk 1.036, text split 1.058. clang inlines the C's own
levels; the arm64 emitter cannot inline across the C boundary.

## Decision

1. obj.xo has entry points that find the heap themselves:
   `rl_new_n`, `rl_grow_n`, `rl_free_n`, `rl_copy_n`, `rc_defer_n`
   (cabi.xo exports `xo_x_obj_*_n`), and arm64 generated code calls
   them (and `xo_x_obj_rl_share`) directly instead of `xo_rl_new_n`,
   `xo_rl_grow`, `xo_rl_free`, `xo_rl_copy`, `xo_rl_share`,
   `xo_rc_defer` (selfhost/arm64 calls.xo, rc.xo, closures.xo).
2. The heap words: process word 10 (`rt.static_word`) is the single
   threaded heap record, 0 once several threads run; words 11 to 13 are
   the addresses of three C functions reached through `rt.call_c2`: the
   thread's heap record (heap_slow, for the threaded case, after the
   thread local word 3), and the real-time section paths of xo_rl_free
   and xo_rc_defer (the C's section queue, rt_defer, and its rtstats
   counts stay C). The C main sets them before anything allocates (a constructor does in the C test programs with their own main, XO_RT_NO_MAIN; xo's ELF linker takes no constructors, gotchas/runtime-c-no-constructors.md);
   xo_rt_threads_begin sets word 10 to 0 when it clears solo_heap.
   Element size and drop reach the real-time list path through two words
   of the heap record (pend_esize, pend_drop), since rt.call_c2 passes
   two.
3. The common cases make no call: `alloc.alloc_fast` (a small block from
   its class's free list or the chunk, else 0; no real-time section) is
   inlined into them, so a new list, the first push to an empty literal,
   and the free of a list without element drops have no frame; every
   other case is a tail call (a growth writes the new capacity before the
   resize, which copies the header with it).
4. The C entry points stay for LLVM generated code and the seed compiler
   (both call them by name) and forward to the same exports, a level LTO
   folds into the caller. C code that makes a list (xo_rl_new) passes a
   heap word as before.
5. obj.xo keeps alloc.alloc and alloc.free at one call site each (halloc,
   free1): the arm64 inliner copies a large function into at most two
   call sites program wide, and a third caller made cabi.heap_alloc and
   heap_free (every C allocation) call out
   (gotchas/arm64-inline-few-sites-new-callers.md). xo_resize is marked
   noinline for the same reason in clang.

The string objects (rs_new, rs_append, the builders) stay C; this
decision does not cover them.

## Consequences

- On the Mac, arm64 and LLVM, B over A (A main with the C list objects,
  B this change): every workload of bench/perf alloc, nested, text,
  json, and xo is within the bar (worst 1.021, text split on arm64;
  alloc small 0.967 on arm64), and the self build of xo is 1.000 of the
  instructions with identical output. The results file has the tables.
- The list code lives in Xo once; xo_rt.c keeps the entry points, the
  real-time paths (rl_free_c, rc_defer_c), and xo_rl_new for C callers.
  Its line count did not fall (comments and glue: 125 lines added, 89
  removed); deleting the entries waits for a seed that emits the
  `xo_x_obj_*_n` calls on every backend (LLVM still names the C).
- Generated arm64 code depends on cabi.xo's `obj_*_n` exports; a
  program's runtime must link obj.xo (it always does: cabi.xo uses it).
- A bug in these paths breaks the compiler built with them, which is
  then the seed of the next rebuild (gotchas/broken-xo-becomes-next-seed.md);
  internal/llvm/testdata/golden/list_objects covers the fallback that
  did (the empty literal's first push of large elements).

## Alternatives

- Keep the C entry points and tune obj.xo (3da29c8b, then with shrink
  wrapping): measured, 1.20 on alloc small; the levels themselves cost.
- Export symbol aliases instead of branches: saves one instruction per
  call, not the C level.
- LLVM generated code calling the exports too: the C entry already
  folds under LTO, and the export's signature (i64) differs from the
  declarations' (ptr), which would need casts in every call site; not
  needed for the bar.
- Inline the whole allocator (raise INLINE_FEW_SITES): a compiler wide
  change in code size for one runtime module.
