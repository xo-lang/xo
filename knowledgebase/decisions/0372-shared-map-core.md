# 0372: one Map core per key class instead of per Map type (native size)

Status: Proposed (2026-10-24, size step 2 for the Xo Lead; step 1
measured, see Step 1 results; the design refined from it; nothing built).

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
