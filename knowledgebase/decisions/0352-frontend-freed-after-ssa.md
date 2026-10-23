# 0352: the front end's data is freed once the SSA exists

Status: Accepted (lead, 2026-10-23); steps B, C and E implemented, step
D's tool (XO_PROF_HEAP_AT) landed

## Context

A native build kept every phase's data alive to its end: the checker
(every module's syntax trees, names, types and flow tables) through the
build's `c`, through `ssa.Builder.c`, and through the lowering's result,
and the SSA module before inlining next to the inlined one, through
emit, assembly and link. The scale suite's fns knob measured about
115 KB of xo memory per six line function, and xo's self build peaked
at 12.4 GB of heap with every cache off (knowledgebase/infrastructure/
scale-suite.md; Xo Lang 1's design note, 2026-10-23). XO_PROF_HEAP_AT
(step D's tool) showed what is alive once inlining is done: on fns-4096,
741 MB, two thirds of it the parser's trees.

## Decision

- A (dropped): writing the parse cache earlier frees nothing. It is an
  in-process map (Native.pc) that only `xo dev`'s builds in one process
  reuse, and the checker's modules hold the same trees. Xo Lang 2's
  decision 0351 handles parse reuse across builds on the input side
  (an on-disk entry per file); it never reads Native.pc.
- B: after the last lowering, a build for one target OS that is not a
  test build replaces the checker with `Checker.link_view(keep_parse)`:
  the modules (paths and `use c` prefixes for the link), the
  diagnostics, and the parse cache only when the caller reuses it
  (`Opts.reuse_parse`, set by xo dev). `ssa.Builder.c` and the lowering's
  builder take the same view. A test build and a build for several OSes
  keep the whole checker (test_infos, run_store and the next OS's
  lowering read it).
- C: on the last target, once the inlined module is verified and given
  to gen_module, the modules before and after inlining are dropped
  (`cur.m`, `mi`, the builder's), so only gen_module's output is alive
  through code generation and the link.
- E (2026-10-23, after D's first snapshots): lower() takes the link view
  itself, right after the SSA is built (and std/time's zone use read),
  through a `var` checker parameter, so the caller's checker goes too;
  prune_unwind, add_landings, cleanup, insert_rc and rc_cached read no
  part of it. Only for a native build for one target OS that is not a
  test and has no module check cache retry (which reads the checker
  after lowering).
- XO_KEEP_FRONT=1 keeps both (today's behavior, for comparison), and
  XO_KEEP_MODS=1 keeps the modules (C off); neither changes the output.
- D: XO_PROF_HEAP_AT=TEXT (std/debug, d449dc24c) writes the heap in use
  at a mem_label; the SSA copies the passes rebuild (add_landings,
  cleanup) are the next to examine with it.

## Consequences

One xo binary, the three modes (everything kept, B, B and C), caches
off, darwin/arm64:

| | kept | B | B and C |
| --- | --- | --- | --- |
| fns-16384 peak heap | 2,820 MB | 2,556 MB | 2,552 MB |
| fns-16384 max RSS | 2,855 MB | 2,586 MB | 2,585 MB |
| compiler/xo peak heap | 12,402 MB | 10,948 MB | 9,692 MB |
| compiler/xo max RSS | 10.52 GB | 9.97 GB | 8.87 GB |

The three xo binaries built are identical. fns' peak is now reached
before lowering ends (ssa, add_landings, cleanup), so its smaller
gain points at step D.

Step E (XO_PROF_HEAP_AT at cleanup on fns-16384 showed 1.43 GB live, 1.1
GB of it the parser's trees and the checker's tables, held through
lower()'s passes), one xo, everything kept against B, C and E:

| | kept | B, C, E |
| --- | --- | --- |
| fns-16384 peak heap | 2,824 MB | 2,020 MB |
| fns-16384 max RSS | 2.81 GB | 2.08 GB |
| compiler/xo peak heap | 11,616 MB | 8,864 MB |
| compiler/xo max RSS | 10.15 GB | 8.42 GB |

The live heap at cleanup on fns-16384 is now 284 MB, mostly the SSA
(renumber's lists).

## Alternatives

Keep everything alive (the behavior before); free per function instead
of per phase (a larger change to the passes); pass a separate
BuildFacts type instead of a reduced Checker (the reduced Checker keeps
every signature; a late reader of a dropped field would read an empty
value, which the identity check and the fixed point guard against).
