# 0372: one Map core per key class instead of per Map type (native size)

Status: Proposed (2026-10-24, size step 2 for the Xo Lead; nothing
built).

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
