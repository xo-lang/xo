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
