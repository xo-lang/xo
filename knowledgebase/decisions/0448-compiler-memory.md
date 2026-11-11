# 0448: Compiler memory: what holds it and the staged plan to cut it

Status: Accepted (owner, 2026-10-30). Proposed (xo-lang-4, 2026-10-30; the owner's priority of 2026-10-30
evening: compiler memory, a Mac bottleneck). Stage owners (lead): stage 2
xo-lang-4; stage 1 xo-lang-6, after 0381; stages 3 and 4 xo-lang-2 (lead,
2026-10-30: stage 3 moved from xo-lang-6, paused for two days, and comes
after stage 4).

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

## Stage 4 shipped: allocation churn (xo-lang-2, 2026-10-30)

The self-build's allocation (XO_PROF=heap at 16 MiB, each side's xo
building the same compiler/xo, the linux/amd64 host): 724.9 GB before,
525.3 GB after, -27.5%; every amd64 golden passes (460). Wall and
max_rss by `xo bench --ab` on the linux/arm64 host (the self build, ten
rounds): InsertRC's changes wall 0.992 (0.971 to 1.005, within noise),
cpu 0.991 (0.986 to 0.993, better), max_rss 1.006 (0.994 to 1.023, within
noise); sort_by in place wall 0.965 (0.954 to 0.979) and cpu 0.992 (0.987
to 0.996) better, max_rss 1.014 (0.998 to 1.031) within noise; the whole
stage after dce_keep's fix (0deb19338 against 2ca1d8a72) wall 0.959
(0.954 to 0.968) and cpu 0.966 (0.963 to 0.968) better, max_rss 1.001
(0.991 to 1.004) within noise. dce_keep compacting in place first kept
each block's capacity (2M `Inst`s in the landing pad functions) through
regalloc and emit, +3% max RSS; it now copies the kept instructions to a
list of their size when fewer than a quarter remain.

| change | site before (share) |
|---|---|
| list_sort_by sorts its index list in place (merge_sort_in_place), each list sized once; the unused merge_sort deleted | merge_sort[Int] 47.7 GB (6.6%), 93% of it regalloc merge_ranges' sort_by[Rng] |
| Rc.group_releases: a run releasing fewer than GROUP_LEAVES leaves keeps its instructions with no maps; a group's keys grow in place (the list read from a map, pushed and stored back, copied per leaf); one-group check without a Set | group_releases 72.6 GB cumulative (10%), leaf_key 22.4 GB of it |
| imm_at and leaf_group read the leaf path in place (Str slices are views) instead of splitting it | scan.split 38.9 GB (5.4%), 80% from immortal_leaf |
| leaf_uses_fresh sizes its uses once from the memoized leaves | 28.3 GB cumulative |
| Rc.liveness keeps the out sets as local key sets (RcFunc.live_out was never read); Rc.walk's live set holds keys, a branch's held leaves come from a local map | map.put[Map[Int, Leaf]] 35.6 GB cumulative (to 18.1) |
| arm64 dce_keep compacts in place (was a reversed copy and back) | 17.4 GB (2.4%) |
| fold_tuple_fields records the uses of tuples only, in place | 16.7 GB (2.3%) |
| arm64 held_uses grows its lists in place (`update`) | 15.5 GB (2.1%) |
| macho split_operands sizes its list for three operands | 8 GB on amd64 |
| sepcomp_undefined maps std's store (read_mapped) instead of copying it | raw.bytes_buf: 35 of a warm coll build's 47 MB |

The DWARF bullet of stage 4 above was a profiler artifact: dwline.cstr
and parse_info (about 50 MB of the warm coll build's exit profile) were
the XO_PROF_HEAP_AT snapshot's own symbolization, which the sampler
counted into the exit profile; a build without XO_PROF never reads its
DWARF. heapprof now pauses sampling while a snapshot is written
(heapprof.pause), so the exit profile counts the program's allocation
only.

Next, by size after these: map.putk[Int] 42.9 GB (Map growth; there is
no Map presize, which would need a std API), arm64 solve_live 37.9 GB,
x86_sink_loads 29.3 GB, the remaining scan.split 27.8 GB (x86 callers),
and the copy-modify-store sites `xo vet` reports as XO0532 (about 59 in
compiler/).
## Stage 1, as built (xo-lang-6, 2026-10-30)

Four changes to where the heap keeps freed memory, all in std/native/llrt
and compiler/xo:

- The large block cache (heapos.xo keep/take: 16 mappings, 64 MB; on
  darwin every block over 16 KiB since mid_max fell to 16 KiB) kept its
  mappings resident: a freed block's pages stayed in the footprint until
  the mapping was reused or the process ended. keep now gives back the
  pages past the mapping's first 16 KiB on darwin (MADV_FREE_REUSABLE:
  lazy, the kernel takes the pages when memory is short and a reuse
  before that costs nothing; under the cache's lock so no taker writes a
  page the advice drops) and take marks them in use again
  (MADV_FREE_REUSE, so the footprint counts them). `XO_HEAP_CACHE_ADVISE=0`
  is the old behavior, for a comparison; the switch word lives in the
  cache's table, read with no allocation.
- The 16 KiB class's full magazines are scavenged as the 32 KiB and
  larger classes' were (decision 0383): since decision 0425 its blocks
  are whole pages with their links in the chunk's table. A scavenged
  page block carries PADVISED in its table word until it is freed again,
  so it is advised once.
- `debug.mem_trim()` (std/debug, native; the Go backend's is empty):
  the calling thread's free lists and spare magazines of the page
  classes are advised, each block once (darwin). xo's `lap` calls it
  after the phases that free the most: front_end_freed, emit and link.
- Linux gets none of the three allocator changes, for now. Every Linux
  variant measured cost the warm coll build's wall: a trim after each of
  the 25 phases 1.047 (arm64), three trims 1.043, the cache's advice as
  MADV_FREE instead of MADV_DONTNEED 1.039 with the RSS win gone; the
  two attribution runs on linux/amd64 (the same 4 KiB pages, an empty
  queue) put the cost in the allocator side, not the mapped store read:
  V1 (the store copied again, the advice kept) 1.03 worse, V2 (the
  advice off, the store mapped) 1.003 within noise. MADV_DONTNEED frees
  the pages at once and the next phase faults them back in (4 KiB at a
  time), while darwin's reusable pages cost nothing until memory is
  short (0.992 there). Linux keeps decision 0383's behavior (the 32 KiB
  and larger classes' magazines scavenged, the large cache resident); a
  lazy Linux variant (MADV_FREE for the trims and the 16 KiB scavenge,
  whose RSS win then shows only under memory pressure) is a follow-up
  with its own `--ab`.
- xo's undefined symbols check (main.xo sepcomp_undefined) read the std
  store into a copy; it maps it as the link does (decision 0402). The
  copy was the warm build's mapped peak: 118 MB against 83 MB in the
  phase before. The same change landed first with stage 4 (xo-lang-2,
  the table above), so this commit carries only the allocator side and
  the trims; the measurements below include the mapped read.

Case (c), coll (bench/perf/xo) on darwin/arm64, `/usr/bin/time -l`,
base b7f4a38aa against this change (gate111, 2026-10-30 22:06):

| Build | Max RSS before | after | Footprint before | after | Mapped peak before | after |
|---|---|---|---|---|---|---|
| warm | 125 MB | 93 to 95 MB | 101 MB | 55 MB | 118 MB (undef) | 79 to 83 MB (72 with a trim after every phase) |
| edit | 126 MB | 95 MB | 101 MB | 55 MB | 118 MB | 99 MB |
| cold | 349 MB | 352 MB | 332 MB | 325 MB | 329 MB | 329 MB |

The large cache alone (the first prototype) took the footprint from
100 to 73 MB and left the max RSS at 123 to 125 MB: the undef copy was
the RSS peak. At exit the warm build holds 53 MB in the cache (advised,
not resident), 15 chunks (60 MB mapped, the touched pages resident) and
18 MB on class free lists, 16 MB of it in the 4 KiB to 8 KiB classes
and the small ones. The cold build's 196 to 205 MB on free lists is in
the 4 KiB and 8 KiB classes too: prefixed blocks whose link is their
first word, with no whole 16 KiB page to give back. What is left of
(c)'s target (under 60 MB RSS) is xo's own text (the binary's clean
pages, 26 to 29 MB of RSS, not footprint) and those classes: a page run
allocator for the 2 KiB to 8 KiB classes, or per chunk page accounting,
is the next lever (stage 1b, not started). Output identity: the binary
coll built with the switch off and on is byte identical; against the
base xo it differs only by std's debug module, whose asm text for the
program's own parts is identical (gate111b). Wall: `xo bench --ab` of
the warm build, darwin (ab111) and linux/arm64 (ab-host edit mode):
see the rows below.

Wall rows, `xo bench --ab` of the warm coll build (edit mode: the front
end and the std half from warm caches, the bin cache off, 10 rounds):

| Host | Before | After | B / A |
|---|---|---|---|
| darwin/arm64 (ab111, quiet window, 0 loaded rounds) | 105.0 ms | 104.3 ms | 0.992, within noise (0.973 to 1.003); instructions 1.002; max RSS 0.76 |
| linux/arm64 (ab-host stage1-coll, a trim after every phase) | 107.6 ms | 112.6 ms | 1.047, worse (1.035 to 1.059); max RSS 0.865 |
| linux/arm64 (ab-host stage1-coll2, trims after front_end_freed, emit, link; the cache still MADV_DONTNEED) | 107.3 ms | 111.9 ms | 1.043, worse (1.035 to 1.054); max RSS 0.864 |
| linux/arm64 (ab-host stage1-coll3, three trims, the cache MADV_FREE) | 107.7 ms | 111.8 ms | 1.039, worse (1.023 to 1.047); max RSS 0.987 |
| linux/amd64 (stage1-v1: the store copied again, the advice kept) | 214.7 ms | 221.1 ms | 1.03, worse (1.016 to 1.036) |
| linux/amd64 (stage1-v2: the advice off, the store mapped) | 216.1 ms | 216.7 ms | 1.003, within noise (0.984 to 1.011) |
| linux/arm64 (ab-host stage1-final: as landed, the advice darwin only) | 107.3 ms | 106.8 ms | 0.996, within noise (0.983 to 1.003); max RSS 1.001 |

Fixed point (fp111 and, with the three trims, fp112; bootstrap-xo.sh s2
and s3 with the tree's xo as s1): ok, s2 367 s and s3 126 s on a loaded Mac. The self build on darwin
(selfrss111, loaded Mac, caches off, default jobs; a footprint figure,
not a wall claim): max RSS 5456 to 4896 MB, peak footprint 7095 to 6642
MB, mapped peak 7426 to 7171 MB (emit), mapped at inline 5017 to 4634,
at the link's end 3736 to 3313.

Worker threads' free lists (the self build's emit workers) are not
trimmed yet: pool.xo cannot import heapos (heapos imports pool), so an
idle trim goes through cabi.xo heap_grow's flag path as the scavenge
does; measured next on case (a) with stage 3.

### Stage 3: a smaller `Inst` (xo-lang-2, 2026-10-31)

The machine instruction: its fifteen Bools are bits of
one `fl` word (`Inst.has(IF_*)`, `fl_set`), 43 words down to 29. Output is
byte-identical (scripts/identity, 235 compiles, 0 differing). On the Mac
(the self build, two rounds) max RSS 5921, 5972 -> 5837, 5868 MB, emit's
peak 6113, 6156 -> 6016, 6026 MB, emit 18.0, 18.4 -> 17.9, 18.0 s. linux/arm64
`xo bench --ab` (the self build, ten rounds, 0deb19338 against the
change): max RSS 0.972 (0.961 to 0.981) and CPU 0.992 (0.991 to 0.999) better, wall 0.997 (0.989 to 1.017) within noise.

The fault exit next: `recover`, `fcond`, `ffn` and `fargs` move into
`fx: List[FaultExit]`, the immortal empty `[]` on instructions without
one (a probe of the self build after regalloc: 9,709,737 instructions,
44,150 with a fault exit, 0.45 percent), read through `Inst.recover()`,
`fcond()`, `ffn()` and `fargs()`; 29 words down to 23. Output is
byte-identical (scripts/identity, 236 compiles, 0 differing). On the Mac
(the self build, two rounds) emit's peak 6133, 6035 -> 6111, 6081 MB,
within noise: MIR lives one function at a time per emit worker, so an
`Inst` cut pays only through the largest functions in flight together.
The heap in use when half the parts are done (XO_PROF=heap, two rounds
each) shows the cut: the instruction arrays (isel.xo G.add) 413, 595 ->
316, 482 MB, all in use 3.25, 3.42 -> 3.14, 3.07 GB; the peak falls
elsewhere in the emit.
linux/arm64 `xo bench --ab` (the self build, ten rounds, e3630dd65
against the change): CPU 0.996 (0.994 to 0.998) better, wall 0.996 (0.993 to 1.008) and max RSS 1.005 (0.988 to 1.017) within noise.

### Stage 3 as built (xo-lang-6 and xo-lang-2, 2026-10-31)

Lang 6's stage 3 work (branch xl113) rebased onto stage 1, then split by
its linux/arm64 cost (lead, 2026-10-31: a stage may not be slower):

- an idle emit worker returns its free memory (heap_grow TRIM from the
  pool's idle path);
- a part drops each function's body as soon as its text is placed, not
  at the part's end (the first wave's eight largest parts held theirs
  through their whole emit);
- `XO_EMIT_MEM` bounds the parts running at once by the heap's mapped
  bytes (off by default; toolchain.md).

Held: ssa.pack_values, which copies every function's value table to its
size before the emit (insert_rc's and the inliner's first push into a
full table doubled it). As one serial pass on the main thread it cost
0.3 to 0.6 s of the self build's wall (`phase pack`); it comes back on
the emit's worker count, each table in a cell of its own, after its own
--ab.

Gates: arm64 goldens, the fixed point (s2 and s3 identical), and
TestCodegenInputsClassified (XO_EMIT_MEM: none).

| the self build | change | max RSS | wall | CPU |
|---|---|---|---|---|
| linux/arm64 --ab, self=a, 10 rounds | all four (with serial pack_values), 14a71b5e6 vs e73cd7a12 | 0.986 (0.974 to 0.994) better | 1.007 (1.0 to 1.014) worse | 1.003 (1.001 to 1.006) worse |
| linux/arm64 --ab, self=a, 10 rounds | trim, body drop, XO_EMIT_MEM (bd0bd05a5 vs 5a4a8be01) | 1.005 (0.988 to 1.009), within noise | 0.996 (0.989 to 1.007), within noise | 0.999 (0.997 to 1.002), within noise |
| linux/arm64 --ab, self=a, 10 rounds | pack_values on jobs tasks, over the line above | (measured with its own change) | | |
| linux/amd64 --ab, self=a, 10 rounds | all four | 1.0 (0.988 to 1.015) | 1.015 | 0.993 (all within noise, wide spreads) |
| linux/amd64, `XO_EMIT_MEM=5G` against off | the bound | 0.915 (0.905 to 0.923) better | 2.247 (2.163 to 2.264) worse | 0.821 better |
| Mac emit peak, two rounds, loaded | main / body drop / all four | 6132 to 6196 / 6076, 6047 / 5989, 6023 MB | | |

The 5 GB bound serializes most of emit on the amd64 host, so it stays
off by default.

pack_values packs after the inliner. Packing earlier (ssa.renumber's
tables at their size, before inlining) saved 70 to 240 MB through
insert_rc but cost 200 to 300 MB from emit on
(gotchas/exact-ssa-tables-before-inline.md).

### Stage 3: where emit's peak is, and the live sets (xo-lang-2, 2026-10-31)

Probes labelled the heap after N parts (XO_PROF=heap, XO_PROF_HEAP_AT,
reading the membudget counter and the chunks, no free list walk:
gotchas/heapstats-walk-is-exit-only.md). The counter rises to its peak
between parts 32 and 40, and every midpoint snapshot before missed it:

| parts done | chunks (MB) | counter (MB) | peak so far (MB) |
|---|---|---|---|
| 1 | 2492 | 4024 | 4250 |
| 8 | 3052 | 4817 | |
| 24 | 3124 | 4727 | 5371 |
| 32 | 3184 | 5749 | 5804 |
| 40 | 3160 | 5213 | 6143 |
| 48 | 3160 | 5091 | |
| 64 | 3124 | 4848 | |
| 96 | 3156 | 4928 | |

At 32 parts the top holder is regalloc's solve_live, 1.01 GB: the live
in and out sets (`List[List[Int]]`, one list per block) of the largest
functions in flight; then isel's G.add 468 MB, add_param 442 MB,
Func.add 332 MB, the part text 286 MB. The small classes' free lists at
the first part hold 752 MB of 2.57 GB of chunks, so allocator retention
is not the peak (decision 0450 stays a stub). The sets now hold
`Int32` (virtual registers are below 2^31): output byte-identical
(scripts/identity, 236 compiles, 0 differing); the Mac's emit peak
6040, 6018 -> 5422, 5499 MB (two builds each); linux/arm64 `xo bench
--ab` (the self build, ten rounds): wall 0.982 (0.971 to 0.989), CPU 0.991 (0.988 to 0.995) and max RSS 0.918 (0.906 to 0.925), all better.

The live-out sets next: they are the union of the successors' live-in
sets, so `Live` keeps the successor lists and `Live.out(k)` merges them
when the allocator asks (a block with one successor gets that list as
is). Output byte-identical (236 compiles, 0 differing); the Mac's emit
peak 5544, 5423 -> 5041, 4980 MB over the Int32 sets; linux/arm64 `xo
bench --ab` (ten rounds): wall 0.974 (0.965 to 0.986), CPU 0.973 (0.970 to 0.982) and max RSS 0.931 (0.931 to 0.937), all better (three rounds, 118dfe1ce against the change).

The live ranges next. With the Int32 sets, the computed live-out sets
and decision 0452's growth curve, emit's peak on the Mac was 4.7 to 4.9
GB, and the heap at 32 parts had the register allocator's ranges
(`List[List[Rng]]`, regalloc.xo push_rng) on top: 949 MB of 3.06 GB in
use, then Func.add 316 MB, add_param 267 MB, solve_live 136 MB. A
range is now one Int, start << 32 | end (positions are below 2^31), read
through rng_s and rng_e; a packed list sorts by start, then end, which
merges as before. Output byte-identical (237 compiles, 0 differing); the
Mac's emit peak 4934, 4915 -> 4498, 4479 MB, peak footprint 4.67, 4.64
-> 4.35, 4.33 GB; linux/arm64 `xo bench --ab` (three rounds): wall 0.991 (0.990 to 0.992) and CPU 0.991 (0.985 to 0.992) better (a three round verdict on an effect under 3%), max RSS 0.887 (0.878 to 0.895) better (0579e8426 against the change).

### Stage 3: a slimmer `ssa.Value` (xo-lang-2, 2026-11-01)

A struct field is a word whatever its type, so packing needs accessors:
`pad` and `inl` are the halves of `Value.pi`, `ty` and `block` of `Value.tb`
(`pack_pi`, `pad_of`, `inl_of`, `with_pad`, `with_inl` and their `tb` twins in
compiler/ssa/ir.xo; ids fit in 32 bits). A value is 11 words, was 13. The sites
were rewritten by script from `xo refs` positions. Output is unchanged: identity
on linux/arm64 and darwin/arm64, 119 programs each, 0 differing.

linux/arm64 self `--ab` (d9fdb4a6f vs 9cdb7cd63, 3 rounds, every effect under
3%, 2 of 3 rounds flagged loaded, mostly by the build's own threads): wall 0.992
(0.991 to 0.993), CPU 0.994 (0.986 to 0.996), max RSS 0.984 (0.978 to 0.988).
Mac peak footprint, two rounds each: 4012 and 4181 MiB to 3860 and 4008 MiB
(about -4%). The Mac's heap counter peak did not move (3975 and 3971 MB to 3974
and 4018 MB), and neither pair is enough to claim more than the direction.
Next word: `pos` (two words and a string per faulting value).

### Stage 3: what was held or not landed (xo-lang-2, 2026-11-01)

Exact list capacity (newcap, branch l2-newcap2, e55e2b7b0 vs 2c968c4ec): held.
linux/arm64 self --ab, two runs: wall 1.042 and 1.039, CPU 1.034 and 1.032,
max RSS 0.929 and 0.941 (3 rounds; rounds at load 5.5 to 8.3). linux/amd64:
wall 1.035, CPU 0.931, max RSS 0.939. The inliner is not the cause (one tail
branch differs). A Linux profile shows samples +3.1% and +1.9% with no hot
function and signs that flip between runs, which fits locality or layout but
does not prove it. Next if revisited: hardware counters on the host, or exact
capacity only for n >= 2.

Renumber with_capacity (Lang 6): not landed, a net loss when measured.

Pack values (Lang 6's Value list compaction, on the jobs tasks; branch l2-s3p2,
d9fdb4a6f vs 3e43c0269): not landed. linux/arm64 self --ab, 3 rounds, 0 loaded:
wall 1.009 (1.003 to 1.01), CPU 1.003 (1.002 to 1.005), max RSS 1.174 (1.167 to
1.179) worse. Before decision 0452 the same idea measured max RSS 0.96 with wall
1.009; the cause of the swing is not established (the copies now outlive their
originals on another path, unverified).
XO_DROP_FALLBACK (Lang 6's experiment, branch xl96c, which drops the pre-inline
module before inlining, so a function that fails native code generation cannot
fall back): not built. linux/arm64, off then on, 3 rounds: emit's counter peak
5861, 5881, 5841 MB to 5557, 5533, 5590 MB (about -5%), max RSS 4962, 5007,
4967 MB to 4899, 4866, 4959 MB (-0.2 to -2.8%, in KB/1000 units), wall 37 to 38 s
unchanged. Shipping it needs the fallback to lower the function again from
source, a design with a real cost for under 3% of RSS; revisit it if RSS at
emit becomes the limit. The experiment's branch and worktree are removed;
e762fe114 is the commit.

Idle worker trim (xl113), the body drop and XO_EMIT_MEM landed in fcfda87f7, so
Lang 6's stage 3 worktrees hold nothing else.

### Stage 3: hollow's peak was pinned texts (xo-lang-2, 2026-11-01)

The Mac's heap counter peak of a self build was in hollow (4440 MB), past emit
(3985 MB), though RSS's plateau is earlier (3.3 to 3.9 GB from 20 s to 36 s).
A 16 MiB heap profile charged the live bytes to the hollowed texts: 993 MB by
the 80th of the parts. Each task now rewrites its own text, assembles it,
scans it and drops it (`hollow_objects` in compiler/xo/asmparts.xo), and
`undef_scan`'s names are clones. Dropping the texts alone moved the peak 43 MB,
since the names were views that pinned each text (decision 0330). Output is
unchanged: identity on linux/arm64 and darwin/arm64, 119 programs each, 0
differing.

Mac, two rounds each: counter peak 4441 and 4436 MB to 4009 and 3981 MB, hollow
phase 2213 and 2227 ms to 1492 and 1538 ms, peak footprint 3852 and 3856 MiB to
3810 and 3832 MiB, max RSS 4202 and 4208 MB to 4159 and 4144 MB. linux/arm64
self `--ab` (c9a2c0392 vs 3d0eb1b80, 3 rounds, every effect under 3%): wall 1.0
(0.991 to 1.005), CPU 1.004 (1.001 to 1.012, read as worse), max RSS 0.995 (0.978
to 1.013, noise). So this removes hollow's spike and moves the counter's peak
back to emit; it did not lower Linux's high-water mark, which is set elsewhere.
Left in hollow: the parts themselves (718 MB by `drop_weak_in`) stay live through
it; freeing them needs a check that `hollow_late` and `incr_texts` no longer read
them.

### Stage 3: `pos` and `line` share a word (xo-lang-2, 2026-11-01)

`Value.pos` was a string, two words, set on 78,661 of 6,514,986 values (1.2%). It
is now an index into `Func.poss`, packed with `line` in `Value.lp` (`pack_lp`,
`line_of`, `with_line`, `has_pos`, `Func.pos_of`, `Func.set_pos` in
compiler/ssa/ir.xo). A value is 9 words, was 11. Output is unchanged: identity on
linux/arm64 and darwin/arm64, 119 programs each, 0 differing; amd64 sample 48 ok.

The heap profile at the 24th part shows the bytes: `Func.add` and `add_param`
630 MB to 516 MB. linux/arm64 self `--ab` on the hollow fix (5b1b00225 vs 8ff5fe4a8,
3 rounds, 0 loaded, every effect under 3%): wall 0.986 (0.986 to 0.99), CPU 0.985
(0.983 to 0.994), max RSS 1.002 (0.992 to 1.01, noise). An earlier run on f3e809947
read wall 0.999, CPU 0.998, max RSS 1.008, all noise, so only one of two shows the
speedup. Mac peak footprint, alternating, 3 rounds under other agents' load: 3916,
3802 and 4008 MiB to 3783, 3765 and 3850 MiB (about -3%, lower in each pair). The
Mac's counter peak (3977 to 3979 MB) and Linux's max RSS did not move: about 2 GB
is in use of about 4 GB mapped at the peak, and retained free memory takes up what
the live values give back. So further live cuts (an args pool, `succs` on `Block`)
wait for retention to move.

### Stage 3: retention, three attempts that did not land (xo-lang-2, 2026-11-01)

About 2 GB is in use of about 4 GB mapped at the self build's peak, so retention
limits it now, not live size. None of these three is landed.

**A lower `mid_max`.** `XO_HEAP_MID_MAX=8192` on the Mac's self build, 3 alternating
rounds under load 9 to 13: counter peak 3974, 3974, 3970 to 3706, 3705, 3710 MB
(-6.7%), max RSS 4097, 4019, 4174 to 3672, 3734, 3617 MB (-11%), peak footprint 3835,
3970, 3845 to 3764, 3773, 3770 MiB (-3%). Its cost: every 8 to 16 KiB block becomes a
mapping. As a runtime default, darwin `--ab` (10 rounds, 0 loaded): `alloc grow` wall
1.738, `alloc mixed` 1.2, a ring of 8 to 16 KiB buffers 1.11, and max RSS 1.04 to 1.06
on all three, so it is closed as a default. For xo alone, a linux/arm64 self `--ab` (3
rounds, all flagged loaded): wall 2.133 (2.121 to 2.148), CPU 1.276, max RSS 0.836.
Wall far past CPU suggests threads waiting on mapping calls (not measured). A darwin
only setter stays unproven: the Mac's wall read flat (53, 51, 41 to 52, 48, 52 s) under
too much load to claim, and darwin was not measured in a quiet window. Branch
`l2-xomid` holds `debug.mem_mid_max`.

**`mem_trim` at each part's end.** Mac, 3 alternating rounds: footprint 3752, 3757, 3761
to 3738, 3789, 3774 MiB, counter 3998, 3966, 3978 to 3970, 3974, 3966 MB: no change.
Max RSS read 4076, 3999, 3767 to 3763, 3857, 3692 MB, which is noise of its own size.

**Regions per function.** Heap samples tagged by the function being emitted (Mac, rates
8 and 16 MiB agree; about 99 GB of volume): about 89% of the bytes die in their
function, about 11% later in its part, none outlive the part. The sites that outlive a
function are mostly its generator's per function lists (`G.add`, `G.inherit`, regalloc),
which `begin_func` replaces at the next function, and fcache's encodings (`subst_tokens`,
`put_field`, about 1.6% of the volume). Lists like those come at a steady rate through a
function, so a region empties only at its end, and one function is large:
`build_native` about 6.5 GB, `ssa.build_program` about 3.7 GB. Plain regions per function
would raise the peak. Functions under 64 MB hold 54% of the volume, 64 to 512 MB 30%, and
above that 16%, so regions capped at 64 MB reach about half, for a gain bounded by the
760 to 876 MB of retained free bytes.

**Regions capped at 64 MB, built (never pushed, refs/recover/l2-region).** Blocks up to
8 KiB of a function under the cap, from 256 KiB regions behind `XO_EMIT_REGION`; one xo,
off then on, 3 alternating rounds on a quiet Mac: peak footprint 3742, 3757, 3753 to
4223, 4212, 4208 MiB (+12%), max RSS 4060, 4075, 4070 to 4486, 4524, 4524 MB, counter
3937, 3933, 3929 to 3933, 3933, 3933 MB (regions are not counted), emit 10.5, 10.6, 10.8
to 10.5, 10.3, 10.2 s. Regions peaked at 452 to 466 MB, and that is about what footprint
gained: emit's small allocations were running on memory the heap already held from
earlier phases, and those free lists did not shrink when regions took the traffic. The
mechanism is cheap but has nothing to save while that memory stays. The two builds'
outputs were named differently, so this run did not check that their output matches.

The counter rises 398 MB in emit (3539 to 3937 MB), with regions or without, so that rise
is not in blocks of 8 KiB or less. The next experiment is a heap profile at emit's peak
of what is above 8 KiB.

### Stage 3: emit's peak was one copy of all texts (xo-lang-2, 2026-11-01)

Regions and a different part order left the Mac's counter peak within 4 MB, so it was
read at checkpoints: 3539 MB through every part and the exports, then 3928 MB by emit's
end. `drop_weak_all` copied every part's text (about 1.1 GB) while the parts, the files
list and its own argument held the originals. Each task now takes its text from a cell
(`take_text` in compiler/arm64/parallel.xo), so the original goes when its copy exists.
Output is unchanged: identity on linux/arm64, 119 programs, 0 differing; the self build's
output byte-identical on darwin/arm64 in each round below.

Mac, 3 alternating rounds: counter peak 3940, 3932, 3931 to 3539, 3539, 3546 MB (-10%),
now inline.apply's. Peak footprint 3755, 3741, 3762 to 3551, 3668, 3684 MiB. Max RSS 3996,
4060 to 3668, 3671 MB in the two rounds without memory pressure; the third ran with walls
of 90 and 182 s and is no reading. linux/arm64 self `--ab` (38006f0b3 vs 3dc41c015, 3
rounds, all flagged loaded): wall 0.999 (0.995 to 1.003), CPU 1.0, max RSS 0.994 (0.99 to
1.021): no claim on Linux, where the high-water mark is set elsewhere.

Linux's mark, placed (489e2f331, linux/arm64, `/proc` sampled 5 times a second against the
phase laps, 3 rounds on a loaded host, 2026-11-01): max RSS 4223, 4143, 3894 MB, nearly all
anonymous (file pages 27 to 28 MB), reached in inline.apply in one round and at emit's end
in two. RSS is about 3.6 GB at ssa's end and 2.8 to 2.9 GB after the front end is freed,
then back to 3.7 GB by insert_rc's borrows and plan while the counter stays flat at about
2928 MB, so that rise is pages touched again, not new mappings. The places are good to the
phase, not the second: the plan lap contains the borrows and uniq_var_params laps.

### Stage 3: spilling the parts' texts during emit does not lower the peak (Xo Lang 1, 2026-11-10)

The 2026-11-08 heap profile of aiplatform/v1's cold test build read 1.56 GB in use at the end
of emit, all of it the parts' asm text (raw.str_join in gen_part), so the plan was to assemble
or spill each part as it finishes. A probe (local branch, not pushed) wrote each part's text
to the build's spill directory as gen_part finished and read the texts back after the
workers, so no finished text was held during emit. Measured with stage2 xos, caches off:

| Build | max RSS, texts held | max RSS, texts spilled | wall |
|---|---|---|---|
| xo self build (linux/arm64 host 1, exclusive, 4 rounds each) | 4.70 to 4.73 GB | 4.67 to 4.70 GB | equal (36.2 to 36.7 s) |
| aiplatform/v1 test build (host 2, 2 and 1 rounds) | 4.71 to 4.79 GB | 4.80 GB | equal |

Emit's mapped peak fell about 100 MB (self 5,580 to 5,654 MB against 5,511 to 5,539) and the
build's peak, at assemble, did not move (5,705 to 5,782 against 5,747 to 5,772 MB). The peak
is the first wave of large parts' working memory, and Linux's allocator gives no freed pages
back (heapos.xo cache_advise is darwin only), so the texts made later reuse pages that are
already resident. Not pursued. The paths left: the first wave's working sets (emit_mem_limit,
the order of the largest parts) and a Linux release policy measured with `--ab` (stage 1's
MADV_DONTNEED cost the warm coll build 3 to 4.7% of wall).

### Case (c) on Linux: the ELF half's raw tables (xo-lang-2, 2026-11-01)

coll's warm and edit builds read 172 to 177 MB of max RSS on linux/arm64 against 85 MB on
the Mac (anonymous about 146 MB, file pages 27 MB), so (c) is not darwin accounting. The
largest step is link.store_prepare, 41 to 45 MB on the counter against 6 on the Mac: std's
half is read whole and decoded. A census of the 27.0 MB file: `.rela*` section bytes 13.9
MB, `.symtab` 2.5, `.strtab` 1.1, for a 2 MB program. The parser has already turned those
into relocs and syms and no link reads the bytes again, so capture_half no longer carries
them: 9.2 MB. `xo bench --ab` of coll's edit build (ab-host, 20dce828a vs 5a4a854b5, 3
rounds, 0 loaded): wall 0.987 (0.981 to 0.992), CPU 0.988, max RSS 0.902 (0.893 to 0.929),
171 to 154 MB. No Mach-O change: its half has no such sections (3.6 MB).

Open: Linux's counter runs over 1 GB above the Mac's at the same commit (emit 4720 to 4853
MB in the two rounds that have an emit row, against 3539 to 3546 on the Mac; it already
reads 4128 to 4425 at insert_rc.apply). Not THP (measured below, 2026-11-01). First check when this resumes: the
targets' size class limits (heapos.xo mid_default: 16 KiB on darwin, about 256 KiB elsewhere,
so Linux keeps freed 32 KiB to 256 KiB blocks mapped and counted). Paused 2026-11-01: the
owner moved priority to case (c).

## Case (c): the lists of std's objects (xo-lang-2, 2026-11-01)

A small build touched five lists of every std object, 5.2 to 5.9 MB each, about 28 MB of 85:
decode_std_export's, relocate_export's, install_std's, new_module's (install left no room, so
the program's first object copied the list) and force_body's copy (the builder's lazy check
writes objects while the driver's checker still holds the list). They now touch two:

- Records go once relocated: `c.std_record` is cleared and the decoded record dropped before
  install_std, whose list is assigned whole.
- check_dirs_bound and check_dirs_fetching kept `let fresh = c` for a restart, which held the
  records to the end of the check. The copy no longer has them; a restart decodes them
  again (TestStdRecordBindRetry).
- Room in untouched pages: install_std's list has 1/8 extra, so the program's objects do not
  copy it. decode's and relocate's have 3/2, because the large cache reuses a mapping only when
  it is big enough: install_std takes decode's block and force_body's copy takes relocate's.
- An Obj is about 20% smaller (a list goes from 5.4 to 4.3 MB): realtime_span and the three
  elided fields became zero or one element lists, and let_span became let_at, its start, the
  only part read.

Mac, 3 rounds, no stats, RSS / footprint MB, 5a4a854b5 to this change: coll warm 84-85 / 52 to
65-66 / 40-42, edit 84 / 52-53 to 65-66 / 41-42; conc-spawn warm 74-76 / 47 to 55 / 34-35, edit
75-76 / 47 to 56-57 / 34-35. So conc-spawn meets the 60 MB target and coll does not yet. Go on the
Mac for comparison: coll's edit link 66-68 / 29-30, compile 44-45 / 34-35.
linux/arm64 `--ab` of coll's edit, 3 rounds, 0 loaded: wall 0.96 (0.949 to 0.962), CPU 0.953,
max RSS 0.833 (0.818 to 0.849); conc-spawn's edit: wall 0.896, max RSS 0.825 (134 to 111 MB). Self build: wall 1.001, max RSS 1.001, within noise, so the
smaller Obj does not move the self build's peak.

Tried, negative: text layout (0403 stage 1) from 80 profiles of small builds at 10 kHz (89,311
samples, 1,885 functions): coll 85 to 83 MB, __TEXT resident 23.4 to 21.6. Sampling misses the
functions that run once, which is most of the touched text. Freeing cached mappings' pages at
once (MAP_FIXED) took RSS to 59 MB but cost 1 to 3% wall, dropped. XO_THREADS=1 saves 3 to 4 MB on the
Mac with the same wall; on Linux it saves about 20 of 154 MB (2 threads about the same, 4 about
10) with the same wall: an open lever for Linux.

Std's facts are read when the builder needs them, one variant of the two, about 2 MB less. That
read would take the freed block force_body's copy reuses (coll rose to 70 MB), so the builder
makes c.objs its own first (Checker.own_objs). Mac: coll warm and edit 65-66 / 41-42 to 64-66 / 41-42,
conc-spawn unchanged. linux/arm64 --ab of coll's edit, 3 rounds, 0 loaded: wall 0.99 (0.99 to
1.004), CPU 0.993, max RSS 0.969 (0.926 to 0.999), inside what an A/A control read on this host (0.969), so no Linux RSS claim.

Decision 0453 orders xo's code by the functions small builds run, which meets case (c)'s RSS target on the Mac:
coll 58-59 MB, conc-spawn 52-54 (footprint 39-40 and 34-35), with Linux at 117 and 101 MB. Not met: "mapped heap within 2
times live". At coll's link the counter reads 76 to 84 MB and the resident heap 45 against 15 live (3 times); the
counter charges each 4 MiB chunk whole, and a dozen are touched only lightly. Next, ranked: which classes spread over those chunks and why
they stay partly used (allocator retention, owner acceptance needed); a one-shot release of the two
6.4 MB cached lists when the front end is freed (clean pages, darwin only, must keep wall within 1%); std's half
decode (3.25 MB live at the link); Obj's 22 Bools at a word each (168 B of about 536); XO_THREADS on Linux (about
20 MB).

Retention counted (coll warm, Mac, 2026-11-01, on a tree with module_jobs below; `vmmap` per mapping at each phase, heap profile, XO_HEAPSTATS): the
4 MiB chunks are 6 through the front end (14.6 MB resident) and 16 from cln.store on (19.6 MB resident at the link), 11 of
them under 1 MB touched: the extra ten appear when the back end's tasks start on other threads. So the counter's 60 MB of
chunks is mostly untouched space. Resident heap at link.synthesize is 39 MB (chunks 19.6, large blocks 12.9, other 6.8) against 14.9 live, 2.6
times, and at the link's end about 33 MB. Free lists at exit hold 15 to 16 MB with no outlier class (64 B 1.8, 32 B 1.6, 16 KiB 1.4,
8 KiB 1.3). Allocator retention is therefore not the main gap on the Mac: chunk pages track the front end's peak of small
objects (about 17 of 28 MB live at fe.build). The levers are that peak and the large blocks' clean pages.

Linux: transparent huge pages. The second linux/arm64 host has THP `always`, and first touch of a chunk can fault 2 MB, which fits
Linux's higher RSS and its response to thread count (xo-lang-3 found the same on TLS connections). coll's edit, 3 rounds,
RSS MB: default 121-125, THP off (PR_SET_THP_DISABLE) 88, XO_EMIT_JOBS=1 98, both 84; conc-spawn 90-98, 83, 88, 76. The wall of those rows is not
comparable (the wrapper's python start). xo-lang-3 owns the MADV_NOHUGEPAGE measurement.

Tried, held: the back end's tasks scaled with the module, 1 + functions / 256 (refs/recover/l2-modulejobs is local only;
the change is `module_jobs` over emit_jobs at the inline, gen, passes and rc sites). linux/arm64 --ab, 3 rounds, 0 loaded: coll's edit wall 0.995, RSS 0.931;
conc-spawn's wall 1.018 (0.983 to 1.024), RSS 0.893; bench/perf/http's edit wall 1.038 (1.023 to 1.038), RSS 0.998, worse; self 1.007 within noise. Function count
does not separate coll (339) from the HTTP server (494), whose bodies are larger. Most of its RSS gain may be THP's. Mac: about 1 MB.
Decide again after the THP result, with values rather than functions as the measure.

Relocation (2026-11-01). A warm build's records sit at the offsets they were recorded at: a trace of coll
reads all four deltas 0, with only 106 of 107 module, module object and file ids renumbered. So nearly every
list inside an object maps to itself, and relocate_export now shares each such list instead of copying it
while both records are live (the walk still reports a reference it cannot map). A fast path returning the
records whole never hit, because of those ids. Mac, 3 rounds, RSS / footprint MB: coll warm 59-60/40-41 to
56-57/37-38, edit 55-56/36-37; conc-spawn warm 52/33-34 to 49-50/30-31, edit 49-50/30-31. Live at fe.build the rows
Reloc.obj_rec (1.88 MB) and objs_of (0.9 MB) leave the profile.

Std's facts (2026-11-01). The builder's map of std's facts (about 2.8 MB) and the facts file (2 MB, which every
extern's name and body viewed) stayed live to the link's end, 5 of its 15 MB. The builder drops the map before
the passes (Builder.drop_std_facts) and an extern keeps no view (StdFact.without_views). Live at link.synthesize
15.0 to 10.6 MB. Mac, 3 rounds: coll warm 56-57/37-38 to 52-53/33-34, edit 55-56/36-37 to 51-53/33; conc-spawn warm
49-50/30-31 to 46/28, edit 49-50/30-31 to 45-46/27-28.

Relocations (2026-11-02). macho.IReloc was six words, and a half's relocations were 3.3 of a small build's 15 MB
at the link's end. It is two now, addr and the rest packed; the linker's output is unchanged. Mac, 3 rounds: coll
warm 52-53/33-34 to 51/32, edit 51-53/33 to 50-51/31-32; conc-spawn warm 46/28 to 44-45/27-28, edit 45-46/27-28 to
43-44/27. Mac --ab of coll's warm build, 3 rounds: wall 0.985 (0.966 to 1.02, within noise), instructions 0.998,
max RSS 0.96.

Mapped against live (2026-11-02, coll, Mac, same stack): the counter's peak is 78 to 86 MB with about 26 MB live at
fe.build and 9 MB at link: "mapped within 2 times live" is not met. Chunks 11 to 17 (each thread that allocates takes
a 4 MiB bump chunk and a 4 MiB page chunk, mostly untouched), free lists 13 to 14 MB, large cache 10 to 11 MB. Next,
ranked: unmapping the large cache at phase ends (needs acceptance), a thread's first chunks smaller, the counter
charging a chunk as it is used.

The large block cache (2026-11-02, the owner accepted the change to debug.mem_trim). mem_trim now also unmaps the
cached large mappings of 2 MiB and over, on every target (CACHE_DROP_MIN), and xo trims at front_end_freed, emit and
link. A kept mapping stays in the RSS: on darwin as reusable pages outside the footprint, on Linux with no advice at
all. So on darwin most of this is the RSS reading and on Linux it is real memory. Dropping every mapping cost Linux
its reuse: coll edit wall 1.008, 1.023, 1.032, 0.999 and conc-spawn 1.033 even with the trim only at emit and link.
The 2 MiB bound keeps what the next phase reuses: Linux coll edit wall 0.991, 0.992, 1.001, RSS 0.902, 0.920, 0.936;
conc-spawn wall 1.008, 1.010, 0.993, RSS 0.888, 0.974, 0.951. Full rows: bench/results/cache-drop-2026-11-02.md.

Self build with THP off (2026-11-01, second linux/arm64 host, THP `always`, 3 alternating rounds, 8222cbb7c):
max RSS default 3830, 3767, 3827 MB, PR_SET_THP_DISABLE 3800, 3873, 3860; emit's counter peak 4510, 4448, 4512
against 4509, 4543, 4576. THP does not explain the Linux counter gap and does not move the self build's RSS,
though it does move small builds (above). Linux's counter (about 4500) now reads about 700 MB over its own max
RSS (about 3800), while the Mac's counter (3539-3546) sits near its footprint (3551-3684), so the gap is mostly
mapped and never resident: the real memory differs far less than the counter. Next for the parked item: the
targets' size class limits (heapos.xo mid_default) with the counter read per class.

Obj's 22 Bools in one word (2026-11-02, not landed; the lead declined the churn). Through `fn (o: Obj) f()` and
`fn (var o: Obj) set_f()` methods (d749e20a8) it cost memory: each setter on `c.objs[i]` moved the whole Obj out of
the list and back (the getters inline to a bit test), coll warm RSS 46 to 58 MB, Mac --ab wall 1.064 (gotcha
list-element-setter-moves-struct). With the bits in a one word `ObjFlags` field read as
`x.flags.f()` (branch l2-objflags, 80a8523a0, about 280 sites): heap counter at check.setup 26 to 24 MB, fe.build
40 to 37, chunks 56 to 48 MB, link peak 78 unchanged; coll warm RSS/footprint 46/30-31 flat, conc-spawn footprint
27 to 25-26; Mac --ab coll warm wall 1.005, instructions 0.998, max RSS 0.999. Kept as a branch.

### Case (c) on Linux: a small module's tasks (xo-lang-2, 2026-11-02)

XO_THREADS=1 saved about 20 MB on Linux (above). The compiler's lever is its own task count: each
task of a module's parallel stages (the inliner, the passes, the landing pads, insert_rc, the
code generator) that runs on another worker thread gives that thread its own heap chunks.
linux/arm64 (12 cores), `xo bench --ab` edit builds, 3 rounds, 0 loaded, every emit_jobs site
capped at k (refs/recover/l2-thr-k1, k2, k4 on ae104ff32):

| k | coll RSS / wall | conc-spawn RSS / wall | HTTP server RSS / wall | self wall / RSS |
|---|---|---|---|---|
| A/A | 1.021 / 0.993 | 0.999 / 0.976 | | |
| 1 | 0.923 / 0.976 | 0.905 / 0.975 | 0.895 / 1.029 (worse) | |
| 2 | 0.913 / 1.0 | 1.0 / 0.995 | 0.943 / 0.996 | 2.378 / 0.862 |
| 4 | 1.013 / 0.993 | 1.0 / 1.049 (worse) | | |

So two tasks keep a small build's wall and drop up to 9% of its RSS, one task costs the HTTP
server 3% wall, and the self build needs every task. module_jobs caps the module sites at
2 + functions / 1024 (coll 339 functions, the HTTP server 494: 2 tasks; xo itself: emit_jobs).
The parse prefetch (bcache.xo) takes 2 + files / 64. Base 28cdc166b, 3 rounds, 0 loaded:

| | coll RSS / wall | conc-spawn RSS / wall | HTTP server RSS / wall | self wall / RSS |
|---|---|---|---|---|
| module sites only | 0.932 / 0.989 | 0.973 / 0.951 | 0.98 / 1.015 (noise) | 1.001 / 0.993 |
| and the prefetch (landed) | 0.934 / 0.994 | 0.976 / 0.999 | 0.954 / 0.997 | 0.985 / 0.992 (2 of 3 rounds loaded) |

ab-host has no unchanged warm rebuild mode, so these are edit builds; the Mac is unaffected in
practice (XO_THREADS=1 saved 3 to 4 MB there). XO_EMIT_JOBS still sets every count.

Predicted against measured: this section's list put XO_THREADS on Linux at "about 20 MB" (20 of
154 MB, coll's edit with XO_THREADS=1, measured before THP dense and the front end's cuts). Landed,
the saving is 5.6 MB on coll (85.0 to 79.4 MB), 5.4 MB on the HTTP server (117.2 to 111.8) and 2.0 MB
on conc-spawn (83.2 to 81.2): 2.4 to 6.6% of max RSS, about a quarter of the estimate. Most of the
estimate's 20 MB went with decision 0456 (a lightly used chunk no longer faults 2 MiB pages) and
the smaller front end before this change, so fewer threads had less left to save; one task
(k = 1) would add 2 to 4 MB more at 3% of the HTTP server's wall.

Tried, negative: a thread's first bump region as a 256 KiB slice of a shared chunk
(refs/recover/l2-slices, f9f0e3228): coll RSS 0.979 wall 1.0, conc-spawn 0.98 / 1.035 worse,
HTTP 0.978 / 1.017 worse, self 0.993 / 1.004. With THP dense (0456) a lightly used chunk costs
few pages, so sharing it saves little and the shared lock costs wall.


### Case (c) on Linux: allocator retention is at most about 8 MB at the peak (xo-lang-2, 2026-11-02)

Allocator retention: at most about 8 MB at the Linux peak; the peak is the link's own data
(store_prepare 36 to 64 MB, resolve to 74). Measured on host 1 (linux/arm64, 12 cores, THP
`always`), natively, in ab-host's edit environment (a private HOME, `xo dev stdcache` first, std
half on, no background separate compilation, no bin cache), coll's warm build with the xo of
the small module's tasks above (ea1661b98). A measurement-only xo (refs/recover/l2-freelists,
a219e9994, never landed) prints the bytes on every free list (every record's lists and spare
magazines and the central lists) as `--phase-times`' second figure:

| phase | heap counter MB | on free lists MB |
|---|---|---|
| check | 24 | 1 |
| fe.build, ssa | 41 | 0 |
| front_end_freed | 34 | 2 |
| cln.store, insert_rc, inline | 38 | 9 to 12 |
| emit | 36 | 15 |
| link.store_prepare | 64 | 8 |
| link.resolve (the peak) | 74 | 8 |
| link.layout | 72 | 14 |
| link.write | 62 | 35 |

Max RSS 78 to 82 MB. The 36 MB on free lists at exit (13 to 15 chunks; the 256 and 128 KiB page
classes 11.8 and 7.1 MB, 160 bytes 8.7 MB, 64 bytes 2.0 MB; the large cache 16 mappings, 10 MB),
which made the chunks look three times live, is freed in link.write, after the peak, so it does
not set the RSS. XO_THREADS=1 gives 76 to 80 MB, 11 to 12 chunks.

What the link holds at link.resolve (XO_PROF=heap at one sample per 64 KiB, XO_PROF_HEAP_AT
"past phase link.resolve"; 36.1 MB in use): macho.elf_half_decode 18.2 MB (std's ELF half
decoded, elfhalf.xo:402), raw.bytes_buf 10.3 MB (9.2 MB of it the cache files cache_get read
through fs_read), macho.ELinker.gc_sections 5.5 MB (elfstrip.xo:115), maps of names 1.4 MB.
So the half is held twice, as its file's bytes and decoded. Next (the new item 2): decode the
half from views of its file's bytes or drop the bytes once decoded, and see what of the decoded
18 MB resolve and layout read.

The decoded 18 MB is not relocations: coll's half has 51,628 sections in 40 objects (one per
function) and 3,497 relocations, and each section allocated its own empty relocation list. They
now share the empty literal's list (elfhalf.xo): elf_half_decode in use 18.2 to 11.3 MB at
link.resolve, store_prepare's rise +28 to +20 MB. linux/arm64 ab-host edit builds, 3 rounds, 0
loaded: coll max RSS 0.942, wall 0.983; conc-spawn 0.903, 0.975; the HTTP server 0.974, 0.943. The rest of the
11.3 MB is about half section records (112 bytes each in a probe of 100,000: about 5.8 MB) and
half symbol records (105,356).

Two more, measured A then B in one environment on host 1 (heap profile at link.resolve, coll):

- The half's symbols: 78,725 of coll's 105,356 are locals of its dead sections (the mapping and
  section symbols of functions the program does not use), and nothing reads the ones no
  relocation names. The half leaves those out when it is written (elfhalf.xo
  drop_dead_locals) and moves the remaining indexes down (relocations, a group's signature,
  the globals map); a half file is keyed by xo's self_id, so none written before is read.
  Dropping them at decode instead took in use 29.3 to 25.2 MB but cost coll's edit 1.5% wall
  (two passes over every symbol each link); dropping them at write also shrinks the file, so
  its bytes (raw.bytes_buf) fall 9.9 to 7.5 MB.
- ELinker.gc_sections set `dead` on each store_dead section, all of them dead already in the
  half; the write copied each object's section list, which the half shares (5.75 MB,
  gotchas/shared-list-write-same-value.md). It now writes only a change.

Both: in use at link.resolve 29.3 (main at 902e37b4c, with the empty relocation lists) to
16.9 MB, 36.1 before those; coll, conc-spawn and the HTTP server link byte-identical to main.
linux/arm64 ab-host edit builds against 902e37b4c, 3 rounds: max RSS coll 0.896,
conc-spawn 0.930, the HTTP server 0.927; wall 0.984, 0.935, 1.011 (within noise). What is left at link.resolve: the
decoded half (8.2 MB) and the half file's bytes (7.5 MB, viewed by every kept name and section).

Then the half's dead sections (30,446 of coll's 51,628 records, one per function the program does
not use): the half leaves them out when it is written (elfhalf.xo drop_dead_sects), and what
still names one (a global symbol of a dead section, a group member) names one dead section at the
end of its object instead, which every reader of a symbol's section checks first (defines, the
symbol's address, the symbol table). Same environment, A then B: in use at link.resolve 16.2 to
12.7 MB, the half file's bytes 7.5 to 5.9 MB, max RSS 67.2 to 60.5 MB; coll, conc-spawn and the
HTTP server link byte-identical. linux/arm64 ab-host edit builds against 0906f6390, 3 rounds:
max RSS coll 0.873, conc-spawn 0.850, the HTTP server 0.904; wall 0.972, 0.949 (within noise), 0.955.

Where the Linux peak is after these (4cc56a650, coll edit build on host 1, 2026-11-03): the heap
counter peaks at 52 MB in link.write (74 before, at link.resolve), the front end at 41 to 43 MB,
max RSS 60.2 MB (78 to 82 before). The link's own live data is small now: 12.6 MB in use past
link.layout (the half file's bytes 6.3 MB, the output sections 2.4 MB from ELinker.place, the
decoded half 1.6 MB) and 2.1 MB at link.key, against a counter of 40 MB there. So most of the
peak is now what the front end and emit freed and the heap kept (about 13 MB in free page blocks
of 16 KiB and over and 9 MB in smaller classes at exit), which turns around the retention census
above (at most about 8 MB of a peak the link's data then dominated).

Tried and dropped: giving Linux the free page blocks back (MADV_DONTNEED) at the trim after emit
only, before the link, which reuses little (refs/recover/l2-lintrim, measurement only). It lowers
the RSS during the link (a build with VmRSS printed per phase: 88.4 to 83.9 MB after emit, 99.1 to
95.7 MB in link.write) but not the peak, which the front end sets first (VmHWM 98 MB at ssa in
that build, a link reaching it again only). ab-host against 4cc56a650, 3 rounds: coll edit max
RSS 0.992, wall 1.008; conc-spawn 0.951 (within noise), wall 0.968; the self build 0.998, wall
0.999 (all within noise); so it was not proposed for the owner's acceptance. Darwin already
advises those blocks (MADV_FREE_REUSABLE) at every trim. The next Linux lever is the front end's
own peak: check.prefetch (+43 MB of heap in that build) and ssa.

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

## A program's first build on Linux: std's store symbols (xo-lang-2, 2026-11-03)

Past link.layout of a first build (coll on linux/arm64) 21.6 MB was in use, 15.6 MB of it
elf_store_prepare's symbols: as in std's half (above), most are locals of std's dead sections
that no relocation names. elf_store_prepare now leaves them out as it reads each stored object
(the half's drop_dead_locals made a copy of each list, and the freed copies raised an edit
build's max RSS 3.5%). Outputs byte-identical (coll, conc-spawn, the HTTP server, first and second
builds); first build max RSS 188.5 to 183.4, 119.7 to 115.1 and 212.6 to 202.3 MB (one run each);
edit builds within noise (coll RSS 1.000, HTTP 1.023, wall 1.016 and 1.014, ab-host, 3 rounds).

## A first build: the identity hash and std's sum (xo-lang-2, 2026-11-03)

A build with a new xo, or the first after std changes, has no std record and no memo of xo's own
identity. On darwin coll's first such build peaked at 140 MB of heap for 61 MB in use (max RSS 170
MB against 108 for the next build), and its ssa phase took 574 ms against 12:
- self_id read the whole executable (79 MB) to hash it when its memo was missing: 75 MB of the
  peak at once. It now reads a megabyte at a time (the id is unchanged: it matches shasum).
- fe_keys keyed every std body whose tree was loaded, and std_tree_sum parsed every std file its
  memo lacked for their key, though under separate compilation std's bodies come from its store
  (with the record their trees are not loaded, so none was keyed). Neither is done now under
  separate compilation (ssa.fe_std_keyed).
Darwin coll first build: max RSS 170 to 111 MB, ssa 574 to 74 ms. Linux/arm64 (one run each, the
first program in a fresh cache): 188 to 161 MB; outputs byte-identical (coll, conc-spawn, HTTP,
first and second builds).
What is left of a first build once std's record and xo's memo exist (linux/arm64, conc-spawn
after coll warmed the cache): heap peak 67 MB against 52 for the next build, all of it
link.half_save (15.5 ms, the ELF half written for a new link key, decision 0400).
## The large block cache bounded by the bytes in use (xo-lang-2, 2026-11-03; accepted (owner, 2026-11-03))

Refined by decision 0492 (owner, 2026-11-04): on amd64 a freed mapping one of the last 16
requested sizes could use passes this bound (x86 json was 1.75x under it); arm64 keeps it as is.

Accepted (owner, 2026-11-03): the retention change ships in 0.37.0.

Lang 4's 0.36.0 comparison found coll putget's max RSS 1.18x of v0.35.0 on darwin and 1.32 to 1.39x
on linux/arm64. The bisects name 188398ede (decision 0452's curve; mostly fixed by e68bf2fac's 4n
growth room) and 4c397e82b (decision 0425: blocks of 16 KiB and over as page and large blocks).
What is left is heapos.xo's large block cache (up to 16 mappings and 64 MB): a freed mapping it
keeps holds its touched pages, which count in max RSS on both systems (darwin's
MADV_FREE_REUSABLE does not lower it: putget 8,624 KB with XO_HEAP_CACHE_ADVISE=0 or not). At exit
putget held 14 mappings (8 MB) for a heap peak of 6.4 MB.

Without the cache (measurement only, refs/recover/l2-nocache): darwin putget max RSS 0.624 (wall
1.003), push 0.630 (wall 0.909); linux/arm64 putget 0.683, push 0.701 (wall 0.692). But xo's
builds need it: linux/arm64 coll edit wall 1.039, conc-spawn edit 1.072, the self build 1.018.

Proposed: keep a freed mapping only while the cache stays under a quarter of the bytes the program
has in use (membudget.mapped()), besides CACHE_N and CACHE_MAX. A small program caches little; xo,
with tens of MB to GB in use, keeps its reuse. Against main (3 rounds each):

| Run | wall | max RSS |
|---|---|---|
| darwin coll putget | noisy (load 6) | 0.761 (8,240 to 6,272 KB; v0.35.0 7,296) |
| darwin coll push | noisy (load 6) | 0.644 |
| darwin xo build of coll (no bin cache) | 0.991 | 0.817 |
| darwin self build | 1.000 | 0.998 |
| linux/arm64 coll putget | 0.921 (within noise) | 0.847 |
| linux/arm64 coll push | 0.664 | 0.762 |
| linux/arm64 coll edit build | 1.013 (within noise) | 0.970 (within noise) |
| linux/arm64 conc-spawn edit build | 1.021 (within noise) | 0.997 (within noise) |
| linux/arm64 self build | 1.003 (within noise) | 0.991 (within noise) |

Pushed as f3f9552ac after the owner accepted the retention change (2026-11-03).

## Addendum: the darwin size class limit is 64 KiB (owner, 2026-11-04)

Bisecting the owner's v0.36.0 Intel Mac evaluation (Xo Lang 1) found
d3b9a5458, the 16 KiB darwin size class limit of 2026-10-30, as the
darwin/amd64 map1M step from 141 to 154 ms (its parent 141; its own
binary with XO_HEAP_MID_MAX=262136, 141). Since then the large block
cache (decision 0492) and the advice skip for wanted sizes (adb71f90a)
hold the pages of 16 KiB to 256 KiB blocks either way, so 16 KiB no
longer lowers the self build's footprint; it still lowers its max RSS.

Intel Mac, the 13-perf micro (darwin/amd64, main adb71f90a, best of 7
interleaved; 0.35.0: map1M 139):

| limit | json10kx20 | map1M | sort1M | micro max RSS MB |
|---|---|---|---|---|
| 16 KiB (was) | 104 | 145 | 90 | 55.5 |
| 32 KiB | 104 | 140 | 89 | 57.4 |
| 64 KiB | 106 | 138 | 83 | 58.7 |
| 128 KiB | 104 | 139 | 84 | 57.7 |
| 256 KiB | 108 | 138 | 84 | 58.1 |

Mac (darwin/arm64), main's xo building compiler/xo, two runs each
interleaved, caches off:

| limit | wall s | peak footprint MB | max RSS MB |
|---|---|---|---|
| 16 KiB | 45.2 / 41.6 | 3624 / 3611 | 3406 / 3418 |
| 64 KiB | 43.2 / 38.4 | 3383 / 3597 | 3927 / 3613 |
| 128 KiB | 45.2 / 36.9 | 3608 / 3369 | 3562 / 4127 |
| 256 KiB | 41.6 / 36.4 | 3402 / 3468 | 4114 / 4215 |

Decision (owner, 2026-11-04): MID_DARWIN is 64 KiB. It gives back the
whole map and sort loss at the same footprint, for 6 to 15% more self
build max RSS than 16 KiB (half of 256 KiB's 22%). Linux and Windows
keep MID_MAX. XO_HEAP_MID_MAX still sets it (1024 to 262136), and a
value outside that range is now reported at start.


## Addendum: the front end's trees during fe.build (xo-lang-4, 2026-11-05)

Lang 1 measured the front end half of a cold build of s3 and dynamodb
(the aws SDK's two services in one program, ~/xo-lead/notes/lang1/buildcmp)
on the linux/arm64 host: the check ends at about 460 MB, fe.build adds
about 525 MB of SSA, and the peak (about 1,080 MB RSS) is at the end of
fe.build, where every parse tree and checker record is still alive. A
heap profile there (`XO_PROF=heap XO_PROF_HEAP_AT=fe.build`) held
368 MB under parse_block, 236 MB of it trees that force_body parsed for
std/native bodies during the build.

Step 1 (5075f64cc): the builder gives each lazily parsed body its span
back once it built it (Checker.unparse_body), and the checker empties
Checker.bodies after the flow pass, the list that kept every body
check_lazy checked alive (gotchas/checker-bodies-list-keeps-trees.md).
s3 and dynamodb, cold, xo bench --ab n=3: max RSS 0.842 (1,108 to
932 MB), wall 0.989, cpu 0.986. The self build: within noise.

Step 2: the program's own bodies, parsed in full with their files, are
given their spans back before the build (Checker.unparse_bodies, called
by build_native before lower), so the builder parses each again as it
builds it and lets it go after; every rebuild (the conc guess, the front
end cache's retries, a missing std tree, another target) starts from the
same checker and parses on demand. The module scans of uses_os_tasks and
uses_budgets, which walk every body, run once before (ssa.module_scans,
Checker.pre_scans), and skip a file that does not spell `tasks` or
`with_budget`. `XO_KEEP_TREES=1` keeps the trees (the output is the same).
s3 and dynamodb, cold, --ab n=3 against step 1: max RSS 0.882 (932 to
822 MB), wall 1.002 and cpu 0.998 (within noise).
The one-line edit rebuild of s3 and dynamodb (warm caches, n=5): wall
1.005, cpu 1.012, max RSS 0.997, all within noise. xo's self build
(ab-host.sh self, n=3) pays wall 1.007 and max RSS 1.014: its peak is
insert_rc, after link_view freed the trees anyway, so it gains nothing
and pays the second parse. Shipped default-on (lead, 2026-11-05: on,
provided the edit loop stays within about 2%). Output identical
(scripts/identity, 75 programs, 0 differing).

## Addendum: huge programs, cleanup's peak (xo-lang-4, 2026-11-07)

A generated program past a bl's reach (Azure 1's GCP compute/v0.alpha
is the largest real case; a synthetic one of about 170 MB of text) ran
past host 2's 15 GB budget (exit 70) in the middle passes, before any
link: every function's SSA is live from fe.build through emit, about 140
heap bytes per byte of text. A 2,000 instance sample (13 MB of text) on
the Mac: fe.build 553 MB, add_landings 1,429, cleanup 2,132 (peak 2,594),
emit 1,899. A heap profile at cleanup (`XO_PROF_HEAP_AT=cleanup`) put
880 MB of 1.32 GB in use in renumber's value tables (`vals.push`), about
10 M values of 88 bytes, some 25 a source line.

Landed: cleanup in waves of 1,024 functions, each taken out of the
module before its cleaned form is made (the whole module was held twice
at the peak), and renumber's tables made at their size: peak 2,594 to
2,160 MB, after cleanup 1,921, at emit 1,723. Output unchanged.

Next: a census of the values by op after cleanup, to size what the
landings (an `rt.landing` call, a branch and two blocks for each
operation that can fault) and the rest of a line add; then fewer values
or a part-wise pass pipeline (insert_rc's apply through emit by part,
each part's SSA freed after its emit), with an ADR of its own if it
changes the pipeline.

## Addendum: stage 3's step 1, the peak is the inliner (xo-lang-2, 2026-11-09)

Stage 3 started from "emit and link hold 6.0 to 6.44 GB" (the phase
lines' heap figures). Measured again on GCP aiplatform/v1, a cold test
build (backend and binary caches off), xo at 2e07e3aee, linux/arm64 host
3, resident size sampled every 0.5 s beside the phase ends:

| phase end | phase line "heap" | resident |
|---|---|---|
| insert_rc (6.0 s) | 2,365 MB | about 2.7 GB |
| inline (12.9 s) | 4,616 MB | 5.2 to 5.9 GB |
| inline.verify (14.0 s) | 4,660 MB | about 5.9 GB |
| emit (56.9 s) | 6,545 MB | 3.9 to 4.2 GB |
| assemble (64.0 s) | 6,544 MB | about 4.5 GB |

`/usr/bin/time` max 6.77 GB. The phase lines' "heap" is the memory the
allocator has mapped, not what is resident: through emit the scavenger
and the age passes (decisions 0383, 0425, 0533) give back most of it
(XO_HEAPSTATS at exit: 4.6 GB on free lists, 2.0 GB of it 64 KiB
blocks, 1.5 GB of 48 byte blocks). The resident peak is the inliner.

In use (XO_PROF_HEAP_AT, 16 MiB rate): at the end of inline.apply
4.06 GB in 26.4 M objects: ssa.renumber's value tables (passes.xo
1446) 1.51 GB, arm64.inline_call (inline.xo 1288) 1.06 GB,
Func.add_param 686 MB, ssa.cse 320 MB. At the end of emit 1.54 GB,
97.6% the parts' assembly text (raw.str_join in gen_part). So spilling
the parts' text (the c1 design) would lower a phase that is not the
peak; it is not stage 3's first step.

Next step (proposed): the inlined module's values. A census by op and
by function after inline.apply (Lang 4's census after cleanup, extended
to the inliner's copies), then the largest of: (a) Value's args and
succs as ranges of one per-function table instead of a List each (26 M
objects; small lists are likely much of the 1.5 GB of 48 byte blocks freed
at exit), (b) a callee's body dropped once its last call site is
inlined and it is not otherwise reached, (c) inline in waves as cleanup
does (2026-11-07), each wave's functions lowered to parts before the
next. Measured with scripts in ~/xo-lead/notes/xo-lang-2/0448
(heapat.sh, rss.sh) on host 3, peak resident and wall against main.
