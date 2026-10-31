# 0448: Compiler memory: what holds it and the staged plan to cut it

Status: Accepted (owner, 2026-10-30). Proposed (xo-lang-4, 2026-10-30; the owner's priority of 2026-10-30
evening: compiler memory, a Mac bottleneck). Stage owners (lead): stage 2
xo-lang-4; stages 1 and 3 xo-lang-6, after 0381.

## Context

Measured 2026-10-30 on darwin/arm64 (the development Mac) at main
1c22a9304, with an xo built at that tree. Each case was run once
(scripts in ~/xo-lead/notes/xo-lang-4/scripts: mem-phases.sh,
mem-heap.sh, mem-scale.sh). The figures come from four sources:

- Peak RSS and peak footprint from `/usr/bin/time -l`.
- The allocator's mapped heap per phase from `--phase-times`
  (`debug.mem_mapped`, `mem_peak`).
- What is live from `XO_PROF=heap` with `XO_PROF_HEAP_AT` at the phase
  before the peak.
- What is allocated from the exit profile.

The three cases the owner named:

| Case | Peak RSS | Footprint | Mapped peak | Wall | Go |
|---|---|---|---|---|---|
| (a) self-build, cold arm64 (`xo build --backend=arm64 compiler/xo`, private caches) | 5.8 GB | 6.8 GB | 7.0 GB | 62 s | |
| (b) `xo test --std-module=native/base64`: cold / with std's test variant (0436) / after an edit | 579 / 336 / 305 MB | 552 / 302 / 280 MB | 593 / 336 / 302 MB | 1.34 / 0.43 / 0.24 s | |
| (b) `xo test --std-module=native/url`: cold / variant / edit | 609 / 344 / 295 MB | 579 / 308 / 271 MB | 619 / 342 / 305 MB | 1.33 / 0.57 / 0.26 s | |
| (c) coll (bench/perf/xo): warm / edit | 126 / 131 MB | 100 / 102 MB | 118 / 102 MB | 0.11 / 0.12 s | 68 MB |
| (c) conc-spawn (bench/perf/conc/xo/spawn): warm / edit | 109 / 113 MB | 90 / 90 MB | 77 / 73 MB | 0.04 / 0.05 s | 26 MB |

The self-build's peak is lower than the 7.5 to 9.5 GB recorded before.
(The 0.35.0 darwin heap change, 16 KiB size class limit, took 21%; see
CHANGELOG.) The warm and edit builds follow release-vs-go.sh:
std's variants are built, then the records build, then the third
unchanged build (warm), then a one-function edit.

### The allocation totals are right: 33 GB was a truncated count

The exit profile of the self-build estimates 543 GB allocated, from
33,901 samples at one per 16 MiB, none lost. toolchain.md said "a self
build (33 GB allocated)". The two disagree because the 33 GB figure was
cut off, not because the profiler scales wrong:

- At the default 512 KiB rate the profile keeps at most 65,536 samples.
  65,536 times 512 KiB is exactly 32 GiB, so that run's total stopped at
  the table's capacity. The about a million lost samples were reported
  and never counted.
- Scaling is Go's `1 / (1 - e^(-size/rate))` (compiler/xo prof.xo
  heap_scale). One warm coll build profiled three ways agrees:
  - rate 1 (every allocation, 975,323 objects of which 65,536 kept, at
    350 bytes each on average): about 290 MB;
  - 256 KiB: 253.6 MB;
  - 16 MiB: 282.9 MB.
- The self-build allocates about 540 GB in 62 s, about 8.7 GB/s across
  its worker threads.

toolchain.md's line is corrected in this change. Every allocation share
below is from a profile that lost no samples.

### (a) The self-build

Phases, mapped heap in MB at each phase's end, peak in brackets:

- check, front end, cleanup, insert_rc: rises to 3,651 (4,345) at
  insert_rc.apply;
- inline.apply: 4,780 (4,917);
- inline.verify: 4,814;
- fn_keys: 3,936;
- emit: 4,293 (7,036);
- hollow, write and link: 3,525 to 4,499.

So the peak is in emit, about 2.2 GB above what enters it.

What is live entering emit: 3.4 GB sampled after inline.verify, with
every function's post-inline SSA held at once.

| Allocator | Where | Live |
|---|---|---|
| ssa.renumber | passes.xo:1256 | 1.19 GB |
| arm64.inline_call | inline.xo:1100 | 678 MB (1.44 GB with its callees) |
| ssa.Func.add_param | ir.xo:867 | 624 MB |
| ssa.cse | passes.xo:914 | 256 MB |
| ssa.remove_blocks | passes.xo:1115 | 96 MB |
| Func.add, Func.jump, fold_tuple_fields, lexer string_lit | ir.xo:875, :891, passes.xo:1028, lexer.xo:1032 | 80 MB each |

What emit allocates: arm64.gen_part, the parallel per-part code
generation, is 48% of the build's allocation:

- G.canon_function, of which G.regalloc is 19.5% of the build. Of that,
  merge_ranges is 13% and its `list_sort_by[Rng]` 12%.
- G.isel: 9%.
- G.emit: 7%.

Its parts run at once, each holding its machine code, ranges and maps,
which is the 2.2 GB rise.

InsertRC (ssa.Rc.rc_func) is 35% of the build's allocation:

- Rc.group_releases (rc.xo:2213): 13.7%
- prelude.merge_sort[Int] (prelude.xo:2237): 10%
- map putk[Int]: 6.8%
- ssa.leaf_key (rc.xo:25)
- Rc.leaf_uses_fresh (rc.xo:597)
- Rc.verify_rc: 4.7%

### (b) xo test of a std module

Cold, live at fn_store, about 300 MB sampled: the parser's trees and the
checker's declarations of every std module the test links are still
alive at code generation.

| Allocator | Where | Live |
|---|---|---|
| parser parse_block | stmt.xo:16 | 37 MB |
| parser parse_fn_decl | parser.xo:944 | 32 MB |
| parser parse_postfix | expr.xo:869 | 28 MB |
| check.Checker.declare | decl.xo:85 | 25 MB |
| arm64.drop_weak_in | calls.xo:3793 | 16 MB |
| parser parse_type, unparenthesized_effect | parser.xo:1634, :1123 | 15 MB each |

With std's test variant, and after an edit, live at link.synthesize is
159 to 175 MB. The front end is still alive at link:

- parse_fn_decl: 31 to 38 MB
- unparenthesized_effect: 12 to 24 MB
- declare: 18 to 23 MB
- parse_file and Checker.force_body (stmt.xo:304): about 10 MB each

The cold runs allocate 3.4 to 3.9 GB:

- raw.bytes_buf (raw.xo:32): 313 to 324 MB
- num.append_int (num.xo:106): 244 MB
- map putk[Int]: 212 to 238 MB
- lexer string_lit: 114 to 124 MB
- merge_sort[Int]: 112 MB
- macho.split_operands (asm.xo:467): 82 MB

### (c) Warm and edit builds of small programs

Live at the peak phase is 7 to 18 MB, against 51 to 118 MB mapped and
109 to 131 MB of RSS:

| Program | Live | Mapped at that phase | Mapped peak | RSS minus footprint (clean pages, xo's 118 MB binary) |
|---|---|---|---|---|
| coll | 14 to 15 MB | 83 to 88 MB | 118 MB | 26 to 29 MB |
| conc-spawn | 7 to 18 MB | 51 to 59 MB | 77 MB | 19 to 23 MB |

So most of a small build's memory is heap the allocator keeps mapped
after it was freed: 4 to 8 times what is live.

Allocation per warm build is 167 to 298 MB. Live at the peak phase:
- raw.bytes_buf (raw.xo:32): 3 to 7 MB
- macho.half_decode (stdhalf.xo:382): 4 to 5 MB
- check.ExR.obj and ExR.ints (export.xo:479, :257): about 2 MB each

Allocated over the run:
- raw.bytes_buf: 47 to 50 MB
- dwline.cstr (dwline.xo:527): 29 to 34 MB
- dwline.parse_info (dwline.xo:1382 to 1412): 9 to 12 MB, because a warm
  link reads the DWARF line tables again every time
- map rebuildk[Str] and putk[Int]: 3 to 11 MB each

## Decision (proposed)

### Targets

Measured as above, on darwin/arm64:

| Case | Today | Target |
|---|---|---|
| (c) coll and conc-spawn warm and edit builds | 109 to 131 MB RSS | under 60 MB RSS, mapped heap within 2 times live |
| (b) `xo test --std-module` with std's test variant, and after an edit | 295 to 344 MB | under 200 MB |
| (a) self-build mapped peak | 7.0 GB | under 4.5 GB, with wall no slower by `xo bench --ab` (decision 0340) |

Cold `xo test` (about 600 MB) is not a target of its own: the variant
is the steady state, and a user's first `xo test` builds it in the
background.

### Stages, ranked by what the profiles say they recover

1. **Allocator retention** (lever 1; xo-lang-6, after 0381). This helps
   every case, and is all of (c)'s gap. Freed spans are given back to
   the system, or reused across size classes, on a policy that keeps
   mapped heap within about 2 times live:
   - continue decision 0383's scavenge of big classes and cross-class
     reuse into the small and middle classes;
   - give back a thread cache's free lists when the thread parks;
   - add a scavenge at phase boundaries (`lap`), where the build knows
     the live heap just fell.

   Measured on (c) first: mapped against live per phase, RSS, and the
   `--ab` wall of a warm build. Then (a) and (b).
2. **Free the front end before code generation and link** (xo-lang-4).
   The parser's trees and the checker's declarations stay alive through
   emit and link in `xo test` (159 to 300 MB) and through the self-build.
   The build already has a `front_end_freed` phase:
   - find which roots keep the trees in the test path: the test
     harness's TestInfo, the run cache's recorder, the std variant's
     near match (0436), `Checker.force_body`;
   - drop them after the SSA module is built.

   Target (b): live at link.synthesize under 60 MB, RSS under 200 MB.
   Checked with `XO_PROF_HEAP_AT=link.synthesize` and the cmd/xo,
   internal/arm64 and stdtest suites.
3. **The self-build's code generation peak** (xo-lang-6). Two parts:
   - Free each function's post-inline SSA once its part is emitted
     (renumber, inline_call and add_param hold 2.5 GB entering emit), so
     the live set falls as emit proceeds instead of staying until
     fn_store.
   - Bound emit's concurrent parts by memory as well as CPUs: the 2.2 GB
     rise is the parts running at once.

   Target (a), mapped peak under 4.5 GB, no slower by `--ab` on the
   linux/arm64 host and the Mac.
4. **Allocation churn** (not yet assigned). These cost time more than
   peak:
   - InsertRC's per-function maps and sorts (group_releases,
     merge_sort[Int], leaf_key: about 20% of the self-build's allocation);
   - the warm link's DWARF line reading (dwline cstr and parse_info,
     about 40 MB a warm build): keep the parsed line tables of std's
     store beside it, as the store keeps its objects;
   - raw.bytes_buf's growth: preallocate where the final size is known.

   Each is its own change with an `--ab` row.
5. **The checker's peak in `xo test` with std's test variant**
   (proposed by xo-lang-4 with stage 2's result, 2026-10-30; not yet
   assigned). After stage 2, case (b)'s RSS is set by the checker, not
   the link: `check.prefetch` maps 180 to 189 MB and `check.dirs` 247 to
   260 MB, before anything can be freed. Most of it is std's parsed
   function bodies, which a build against the variant does not lower:
   the variant's facts already describe them. Parse std's bodies lazily
   in that build (the checker's lazy mode, decision 0388's pending
   trees), so only the bodies the program's own code reaches are
   parsed. Measured as stage 2 was: `--phase-times` mapped heap at
   check.dirs, and max RSS of `xo test --std-module=native/url` and
   native/base64 with the variant warm.

Each stage reports the table above again, before and after, and the
profile of the case it targets.

### Stage 2 result (xo-lang-4, 2026-10-30)

The root in the test path was the strip's own condition: decision
0352's `link_view` ran only for a build that was not a test, because
`finish` listed the tests (`test_infos`, which reads the files' trees)
after the link. `run_store` and `program_pure` read only the modules,
which the view keeps. `xo test` now lists its tests before `lower` (and
again after the module check cache's retry, which checks the program
anew), and both strip sites drop `!tests`. TestFrontEndFreedByDefault
checks that `xo test` prints `front_end_freed` and still runs its test.

Measured on the Mac with one xo, std's test variant warm,
`XO_SEPCOMP_BG=0`, the bin and test caches off, three runs each, against
`XO_KEEP_FRONT=1` (the old test path):

| `xo test --std-module=` | live at link.synthesize | mapped at link.synthesize | max RSS |
|---|---|---|---|
| native/url, kept | 155.8 MB | 298 MB | 307 to 310 MB |
| native/url, freed | 14.7 MB | 213 MB | 282 to 285 MB |
| native/base64, kept | 133.9 MB | 276 MB | 290 to 293 MB |
| native/base64, freed | 7.9 MB (samples lost) | 218 MB | 262 to 266 MB |

Live at link.synthesize is under the 60 MB target. RSS is not under
200 MB: the mapped peak, 247 to 260 MB, is reached in `check.dirs`,
before anything can be freed, and the 200 MB of mapped heap over 15 MB
live at the link is stage 1's retention. RSS for (b) now waits on stage
1 and on the checker's own peak (`check.prefetch` maps 180 to 189 MB of
parsed std): a stage 5 candidate is parsing std's bodies for the variant
lazily, since the variant's facts already cover them.

### Stage 3 first result (xo-lang-2, 2026-10-31)

What emit holds is live data, not allocator retention. An earlier reading
(2.2 GB live halfway through emit against about 7 GB mapped, so most of
the peak looked like retention) rested on a heap profile that missed a
big block's growth; with that fixed (e459813a2,
gotchas/heap-profile-missed-grown-blocks.md), 4.6 GB is live at
`emit_half`: the functions in flight's machine IR 2.8 GB (their
instruction lists 1.5 GB, regalloc's ranges 0.7 GB), insert_rc on the root
thread 0.5 GB, the inliner's copies 0.6 GB, the finished parts' text 0.4
GB. The inline fallback's pre-inline module, kept until verify, was 1.1
GB at `inline.verify`.

Most of the instruction lists were waste. Counting each function's
instructions after isel and after each pass showed the largest
`ssa.Builder` methods making 50 to 90 times what they kept, all of it
dropped by the first `mir_dce`: every landing pad loaded `b.m` (592
words) whole to release one of its fields
(gotchas/arm64-isel-dead-field-loads.md). A field read only read by field
reads of it now loads only the words they read. The output is unchanged
(scripts/identity: 117 programs on darwin/arm64 and linux/arm64, 234
outputs, none differing). Self build on the Mac, cold caches, two rounds:

| | emit peak (heap counter) | max RSS | emit |
|---|---|---|---|
| before | 8068, 7424 MB | 7619, 7059 MB | 26.0, 29.5 s |
| after | 6784, 6745 MB | 6599, 6580 MB | 21.0, 22.0 s |

On linux/arm64 (`xo bench --ab`, the self build, ten rounds, 64a2dd7b9 against
the change): max RSS 0.903 (0.869 to 0.913), CPU 0.933 (0.928 to 0.937),
wall 0.99 (0.978 to 0.999), all better.

What is left at the peak is the in-flight functions' real IR (an `Inst`
is 43 words: its 15 Bools a word each), regalloc's ranges, and insert_rc
on the root thread; the next levers are a smaller `Inst` and the
inline fallback's copy (a shell of each function, its body read back from
the insert_rc cache, cut `inline.verify`'s peak by 0.5 GB but not emit's).

## Consequences

- One ADR, the plan and its targets; each stage lands as its own change.
- Stage 1 changes runtime behavior that every Xo program sees: when the
  heap returns memory. It gates as decision 0383's changes did: the full
  suites of std/native and the heap tests, and a self-build with the new
  xo (s2).
- toolchain.md's self-build allocation figure is corrected (33 GB was
  the 512 KiB rate's 65,536-sample cap, about 540 GB is the total). Its
  advice to pick a rate of about the bytes allocated / 50,000 stands:
  16 MiB for the self-build.
- The heap profile's lost samples cap its totals. `xo prof` printing
  "the totals count only the kept samples" when samples were lost
  would have prevented the confusion, a one-line follow-up.

## Alternatives

- Go's approach to (c), a garbage collector's scavenger returning
  memory in the background: Xo frees at once by reference count, so the
  same policy applies to spans freed, with no collector. Stage 1 is that.
- Streaming compilation, freeing each function's front end and SSA as
  soon as its machine code exists: the end state of stages 2 and 3, but
  the inliner needs callees' SSA, so freeing waits for the inline
  pass's last use. Stage 3 frees after emit, and a later stage can free
  earlier where the call graph allows it.
- Lowering the self-build's parallelism to cut the emit peak: a slower
  build. Stage 3 bounds it by memory only when memory is short.
