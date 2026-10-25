# 0372: one Map core per key class instead of per Map type (native size)

Status: Accepted (owner, 2026-10-24 23:00, with the landing conditions
below; built the same day: stage A in compiler/ssa map.xo and mapcore.xo,
stage B in maplog.xo and maphyb.xo, see As built).

## Context

compiler/ssa generates a Map's or Set's operations once per Map type
(map.xo, maphyb.xo, maplog.xo: `xo.map.put[Map[K, V]]`,
`xo.maph.slowput`, `xo.mlog.record` and 22 more). A release
bench/perf/http/xo (CRUD) on 3a435412a with the hollow pass (decision
0371) has 117 such functions, 149,304 bytes of its 1,473,008 bytes of
__text (10.1%).

- 43 of them (60,612 bytes) are decision 0368's `#spec` clones for
  constant Str keys. They fold nothing, and 0368's addendum of
  2026-10-24 stops them.
- The other 74, about 89 KB, are 25 operations instantiated for four
  Map types: Map[Int, User] 25,124, Map[Str, Int] 24,504, Map[Str, Str]
  23,740, Map[Str, dyn] 14,684.
- The largest instance of each operation sums to 32,680 bytes.

The four instances differ only in:

- the key's hash and compare (Int against Str);
- the key's and value's width in words (an Int one, a Str two, a struct
  its fields');
- their retain and release.

The probe, the log and fork directory walk (decision 0291), the growth
and the rebuild are the same code. Go has one runtime map for every
type: a type descriptor, plus fast variants by key class
(mapaccess1_faststr, _fast64).

## Decision (proposed)

1. **Key classes.** A key class is word-sized integer keys (Int, the
   sized integers, Rune, Bool, enums without payloads), Str (and Bytes
   keys), or any other key (through a descriptor's hash and eq
   functions).
2. **One core per key class.** Each class gets one generated core per
   module: `xo.mcore.put[str]`, `[int]` and `[any]`.
   - The value is opaque words.
   - Each core takes a value descriptor: its width in words, and its
     retain and release functions, or none for plain data.
   - The descriptor is a constant per Map type in __DATA,__const, passed
     in one register.
3. **Thin per-type wrappers.** The per-type functions that remain are
   thin. Each passes the descriptor, and they inline at their call
   sites. A typed get's result is read from the value slot the core
   returns.
4. **Opt-in to start.** XO_MAP_CORE=on turns it on until the --ab bars
   below are met, then it becomes the default. The per-type path stays
   for the Go and wasm backends.

## Estimate

From CRUD's numbers after 0368's addendum (which removes the 60.6 KB of
clones, CRUD 1,467,848):

- 88.7 KB of map code remains, in four Map types.
- **One core per key class** (str, int): the three Str-keyed types' 72.4
  KB become one core of about 24.5 KB, plus their wrappers and
  descriptors (about 5 KB). That saves about 42 to 48 KB.
- **One core for every key** (a key descriptor too, `[any]`): about
  32.7 KB plus wrappers. That saves about 50 to 55 KB, but Str and Int
  probes become indirect calls of the key's hash and eq.

So 45 to 55 KB, 3 to 3.7% of CRUD's __text. Each further Map type of a
class costs a wrapper (well under 1 KB) instead of about 24 KB. xo
itself, which has many Map types, gains more; to be measured in step 1.

## Steps

1. **Size prototype.** Build CRUD with the three Str-keyed types' core
   functions merged by hand at the assembly level: one instance plus a
   descriptor load, with the value copy and release through the
   descriptor. Measure __text, and check that the CRUD tests pass.
2. **Speed bar.** bench/perf/maps (and json maps, CRUD) under quiet
   `xo bench --ab`, A = per-type and B = core. Bar: wall within 1.03 on
   every row. Get and put of Str keys are hot in CRUD and json.
   Go's faststr variants show the key class keeps the probe direct; the
   value's indirect release is the cost to watch.
3. Build the cores in compiler/ssa, behind XO_MAP_CORE, with tests over
   each key class and value widths of 1, 2 and n words.

## Step 1 results (2026-10-24)

**Size model.** A source-level prototype of the layout, in synthetic
programs with 1, 2, 4 and 8 `Map[Str, V]` types (V = Int, Str, Float,
Bool, List[Int], (Int, Str), Bytes, List[Str]) and the same work on each.
The core form is a `Map[Str, Int]` index into a typed `List[V]`, plus a
free list. Release __text, xo 94028338e:

| Map types | today | today's map code | core form | core's map code |
|---|---|---|---|---|
| 1 | 265,668 | 24,908 | 268,004 | 25,452 |
| 2 | 295,564 | 51,544 | 271,748 | 26,096 |
| 4 | 350,872 | 101,480 | 277,864 | 26,096 |
| 8 | 444,252 | 185,416 | 291,124 | 26,096 |

So an added Str-keyed Map type costs 25.5 KB of __text today and 3.3 KB
in the core form. xo itself has 2,499,724 bytes of map code: 1,750
functions over about 107 Map types (4.2% of its __text). With about 10
key types, the estimate is 10 × 25 KB of cores plus 107 × 3.3 KB of
value code, about 0.6 MB, so about 1.9 MB saved.

**Folding identical code instead.** Measured, and too small to replace
the core:

- All of xo: 300,564 bytes of byte-identical functions (0.5%). CRUD:
  6,868.
- xo's map code alone: 228,568 bytes with exact callees, or 607,460 with
  callees that differ only in name folded too.

**Speed of the source model.** 200k Str keys, value Rec{Str, Int},
instructions (min of 3):

| phase | typed | core | extra |
|---|---|---|---|
| put 200k + iterate | 262.0M | 309.0M | +47M |
| get, 2M | 328.9M | 346.8M | +17.9M (about 9 a get) |
| remove 100k | 24.5M | 28.6M | +4.1M |
| Int keys, whole run | 262.1M | 303.3M | 1.157x |

XO_PROF puts the put phase's extra in a second probe: Xo source cannot
insert or find in one probe, and `Map.update`'s closure made it 6M
worse. A compiler-built core probes once, so the put row is the
source model's cost, not the design's. The get row is the indirection
from index to value. The wall comparison is not measured yet: one
unquiet run gave 0.20 s for the core and 0.31 s for typed, and a quiet
`--ab` follows.

**Per operation, today.** In CRUD's instances, six of the 25 operations
have the same size for every V: map.find, map.scan, mlog.find,
mlog.findx, mlog.probe and mlog.eachpos. They read only the keys. The
lists representation already keeps the values in their own `List[V]`
(keys, vals, live, slots, n). Only the log representation (decision
0291) holds V inside its entry (`MapEntry {h, prev, org, k, v}`), and
its functions are most of a type's 24 KB.

## Refined decision (proposed, from step 1)

No value descriptor and no indirect calls. Make the key side generic
over K only, and keep a small typed value side:

- **A. Lists representation.** find, scan, put's probe and insert, the
  rebuild of the key lists and slots, and remove's probe become
  functions of K that take and return the key side (keys, live, slots,
  n) and the entry's index. The typed part is the write of `vals[idx]`
  (a set or a push) and the compaction of vals in a rebuild, inlined at
  the caller. This is today's work in one probe, so its speed cost
  should be close to none.
- **B. Log representation.** MapEntry drops `v`. A table keeps a
  parallel value log (`vdir: List[List[V]]`) with the same chunking as
  its entry log: the same positions, appended in place the same way.
  Every log function (claim, record, each, findx, probe, the fork walk)
  becomes a function of K, and reading or writing a value at a position
  is a typed two-load operation.
- **Behind XO_MAP_CORE** until each stage passes the map benches under
  quiet `--ab` (wall within 1.03), the goldens on every backend, and the
  fixed point. Then it becomes the default, with an addendum here.

## Acceptance (owner, 2026-10-24 23:00)

Accepted with these landing conditions (Xo Lead):

1. The map benches in a quiet `xo bench --ab`, wall within 1.03 of the
   typed form.
2. The concurrent map tests under `--race` (internal/llvm
   TestGoldenTasks, TestRaceSyncTable and TestRaceSyncTableArm64), plus
   a reader and writer stress run at several XO_THREADS.
3. DS-2's maps fuzz area clean, with and without `-release`.
4. xo's own build time and peak RSS (a cold build of compiler/xo) not
   worse beyond noise, since xo has about 100 Map types.
5. A seed bump, called out here, if the core needs new runtime
   intrinsics. It needs none: it uses only rt.slot_cas, slot_store,
   slot_load, clz, obj_addr, log.push and log.set, which older seeds
   build.

Stage A stays behind the pass `map_core` (XO_PASSES=-map_core) until the
conditions pass; then the default flips with the default-change full
suites. Stage B replaces the log representation with no runtime switch
(two copies of the log would be the larger risk) and lands as its own
commit, so one revert undoes it.

## As built (2026-10-24, branch xo4-mapcore)

- **Stage A** (compiler/ssa map.xo, mapcore.xo):
  - The lists representation's find and scan take the key lists
    (keys, live, slots), named by the key type: `xo.map.find[K]`.
  - put's key side is `xo.map.putk[K]`: the probe, the insert or the
    rebuild of the key lists (`xo.map.rebuildk[K]`) in one pass. It
    returns the entry's index and whether the values list is set,
    pushed, or compacted first (`xo.map.vcompact[V]`, a function of the
    value type).
  - The fe, fn and part cache keys carry `mapcore`.
- **Stage B** (maplog.xo, maphyb.xo):
  - A log map is `{kl: MapLogK[K], vdir, vpar}`. The key side keeps
    0291's eight fields, with `MapEntry[K] {h, prev, org, k}` and
    `MapLevel[K]`; a lists level keeps keys, live, slots and n.
  - The value log vdir has the key side's chunk layout: own entry i's
    value is at i's slot and offset. `MapVLevel[V] {vdir, lv}` is one
    per parent level.
  - The K-only functions are probe, find, findx, each (fresh entries
    plus each one's find result), claimk, recordk, flatten, table,
    putpos, eachpos, ktolog, kfork and entry. The typed wrappers are
    claim, record, put, remove, vals (values of find results), kv,
    list, tolog, fork, oflog and vpos. The per-V functions are vtable
    and vfork.
  - A record appends the value after recordk has appended the entry and
    stored its index slot. Readers of other values read values only
    inside their own views, so the order is safe; an index slot past a
    view is read for its key and prev only, as before.
  - mlog_iterdir and for_mlog had no callers and are gone.

- **Speed fixes**, found on instructions before any timing:
  - The first cut sent every put through putk's tuple of lists:
    bench/perf/xo putget was 1.37x main's instructions.
  - Every log put called claimk and unpacked its tuple: copymut 1.27x,
    branch 1.17x.
  - Now xo.map.put keeps the found-key update and the insert with room
    in the table in line, and calls putk only to grow the table. The
    hybrid's cold putrest and listput always call putk.
  - The typed claim tries its table's next position (slot_cas) in line
    first. claimk retrying the same expected length fails exactly when
    that did, because the claim cell only grows.
  - A table's value log is made at its first value: a fork or a first
    table starts with the shared empty list.

Release builds, main 4dbcd4e1c against the branch on it (tw431
fe647f95d), one xo each:

| program | main __text | core __text | change |
|---|---|---|---|
| synthetic 8 Map[Str, V] | 444,252 | 388,280 | -12.6% |
| bench/perf/http/xo (CRUD) | 1,467,848 | 1,449,924 | -1.2% |

Instructions, branch over main, minimum of runs:

| row | ratio |
|---|---|
| perf/xo putget | 1.006 |
| perf/xo get | 1.002 |
| perf/xo update | 1.002 |
| perf/xo iter | 1.001 |
| perf/xo copymut | 1.003 |
| perf/xo branch | 1.038 |
| perf/xo insrem | 1.001 |
| perf/xo copyset | 1.004 |
| perf/json maps | 0.991 |
| perf/json small | 0.999 |
| 200k Str keys to a struct (put, get, remove, iterate) | 0.990 |
| the same with Int keys | 1.007 |

- Outputs are identical in every row, and CRUD's 11 tests pass.
- branch's 3.8% is the fork's second directory, for the value log, and
  its value level.
- Stress: the map_log_stress golden (concurrent copies of a 2,000 key
  table written by four tasks while four read it, plus a Mutex store's
  writes and snapshots, every read checked). internal/arm64
  TestMapLogStress runs it 5 times at each XO_THREADS of 1, 2, 4 and 8,
  and its `--race` build at 2 and 8. No bad read and no race report, on
  main and on the branch.

Wall, quiet `xo bench --ab` (A = main 4dbcd4e1c, B = the branch). The
second window (40 rounds, 30 for the typed rows) is the one with the
RSS fix; the first (20 rounds) covers the rest:

| row | wall B/A | range |
|---|---|---|
| perf/xo putget | 0.956 | 0.945 to 0.964 |
| perf/xo get | 1.000 | 0.909 to 1.01 |
| perf/xo update | 0.897 | 0.874 to 0.927 |
| perf/xo iter | 0.996 | 0.971 to 1.044 |
| perf/xo copymut | 0.998 | 0.984 to 1.014 |
| perf/xo branch | 1.027 | 1.007 to 1.038 |
| perf/xo insrem | 1.002 | 0.99 to 1.009 |
| perf/xo copyset | 1.009 | 0.971 to 1.037 |
| perf/json maps | 1.005 | 0.996 to 1.015 |
| 200k Str keys to a struct | 1.011 | 1.005 to 1.018 |
| the same with Int keys | 1.026 | 1.017 to 1.033 |

Peak RSS, B/A: Str keys 1.009, Int keys 0.923, putget 0.983, copymut
1.007; the first cut was 1.15 and 1.2 (gotchas/ssa-tuple-of-lists-call-costs.md).

The race bar (Xo Lead, 2026-10-25): on the Mac,
- TestMapLogStress's `--race` runs at XO_THREADS 2 and 8;
- internal/llvm TestTSanSharedMaps (maps shared under a Mutex in all
  three Map forms);
- TestGoldenTasks, TestRaceSyncTable and TestRaceSyncTableArm64.

The linux/arm64 CI run failed the same three tests the same way main
does (Linux clang, lld and no TSan runtime): known Linux issues, best
effort. Its plain TestMapLogStress runs passed.

xo itself, its s2 of the fixed point (s2 and s3 identical): __text
60,609,056 on main, 59,642,480 on the branch
(-966,576, -1.6%; the step 1 estimate of 1.9 MB counted the log's
typed wrappers as shared). A cold build of compiler/xo (caches off),
two runs alternating: main 32.45 and 34.47 s with 9.33 and 8.37 GB peak
RSS, the branch 35.08 and 33.87 s with 8.68 and 8.85 GB, within the run
to run spread.

**Known costs.**
- branch (many one-key forks of a 50k map) and the Int keyed struct
  map are within the 1.03 bar on their point estimates, but their
  ranges reach 1.033 to 1.038. Both are about 1% more instructions.
  branch's share is the fork's value directory and value level, beside
  the key side's.
- A program with one Map type is about 3 KB larger, from the shared
  functions plus the wrappers.

**Follow-ups.**
- One directory for a fork's entries and its values. The lazy value
  directory is in already and did not move branch.
- Shrink the hybrid's typed slow paths further: slowput 2.6 KB and
  slowrem 1.4 KB a type.
- Mark putk and vcompact cold, as the old rebuild was. Measure it on its
  own.
- Flip map_core's default only after its default-change suites.

## Consequences

- The value retain or release becomes an indirect call, or a
  descriptor flag test for plain data.
- A Map of a large struct value copies by width, not by the type's own
  field moves.
- Cache keys: the cores are module functions, and a Map type added in
  one file no longer adds 24 KB of code to the per-function cache.

## Alternatives

- **Identical code folding in the linker** (merge byte-equal functions).
  The instances are not byte-equal, since the hash, eq and widths
  differ.
- **Fewer operations per type** (merging maph's slow paths into one).
  This is orthogonal and smaller.
- **Keep per-type maps and only drop the specialization clones.** That
  is 0368's addendum, already taken. This ADR is the next step.
