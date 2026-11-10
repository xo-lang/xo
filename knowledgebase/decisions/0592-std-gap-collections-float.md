# 0592: Float's type constants and the collection helpers the std audit found missing

Status: Accepted (owner, 2026-11-10, relayed by the lead: "the owner
approved the std audit's gaps"). Drafted by Lang 3. Implemented in stages:
Float's constants first (this commit), then Map.extend and Map.clone, then
List.clone, concat, binary_search, max and min.

## Context

bench/results/std-audit-2026-11-10b.md ranks the Go standard library
symbols that Go programs use and Xo lacks. Among the top 40: math.Inf,
math.NaN, math.Pi, math.E and math.MaxFloat64 (Xo had `is_inf` and
`is_nan` to test values it had no way to name), maps.Copy and maps.Clone,
and slices.Clone, Concat, BinarySearch, Max and Min. Decision 0587 gave
the integer types their bounds as type constants (`Int.max`) through the
prelude's `@static` doc marker, and left `Float.PI`, `Float.INF` to a
later decision.

## Decision

1. Float has seven type constants, declared in the prelude with
   `@static` as 0587's integer bounds (lower case, as `Int.max`):
   `Float.inf`, `Float.nan`, `Float.pi`, `Float.e`, `Float.max`,
   `Float.min` (`-Float.max`, the most negative finite Float, as
   `Int.min` is the most negative Int) and `Float.smallest_nonzero` (Go's
   math.SmallestNonzeroFloat64, a subnormal). Negative infinity is
   `-Float.inf`.
2. Map gets `extend(other)` (maps.Copy: every entry of other put into m,
   other's value winning) and `clone()` (a Map with a table of its own).
3. List gets `clone()` (a List with a buffer of its own), `concat`
   (slices.Concat over a list of lists), `binary_search` (slices.
   BinarySearch on an Ord element: the index found or where it would go),
   and `max()` and `min()` (slices.Max and Min: None on an empty list).

A Map or List copy already shares its storage until one is written (core
5.1), so `clone` is not needed for value semantics; it gives the copy
storage of its own at once, as `Str.clone` and `Bytes.clone` do, for code
ported from Go and for dropping a large shared buffer.

## Consequences

- Programs name infinities, NaN and the Float limits without
  `Float.parse("inf")` or literal digits; std's literal copies (std/rate's
  `INF`, which was `Float.max`) can move to them after the next seed
  (0587's staging: xo builds itself with the previous release's checker).
- On the Go backend the Float constants are Go variables
  (std/rt/intbounds.go): Go folds constant expressions exactly, so
  `Float.max * 2.0` would be a Go compile error where Xo gives +Inf, and
  Go has no infinite or NaN constant.

## Alternatives

- **Upper case `Float.PI`, `Float.INF`** (std.md's earlier wording): reads
  as Go's math.Pi but not as 0587's `Int.max`; one case for every type
  constant is easier to guess.
- **A std/math module of constants**: a second place to look; Xo's Float
  methods already sit on the type.
- **`Float.min` as the smallest positive value** (C's DBL_MIN): ambiguous
  in exactly the way that trips ports; `smallest_nonzero` says what it is.

## Progress

- 2026-11-10: Float's constants (91837d5de).
- 2026-11-10: Map.extend and Map.clone, golden map_extend_clone. Timing
  on the Mac, 100k Int keys: extend 3.1 ms against maps.Copy 4.7 ms;
  clone 3.3 ms against maps.Clone 0.31 ms, since Go copies the table and
  Xo's clone rebuilds it. Next: a clone that copies the map's four lists
  (keys, vals, live, slots: the slots index the entry lists and removed
  entries stay marked, so a copy of each is a valid clone) through an SSA
  helper in compiler/ssa/map.xo.
- 2026-11-10: List.clone, List.concat, binary_search, max and min, golden
  list_helpers (slices_test.go's cases). Timing on the Mac, 1M Ints:
  clone 0.55 ms against slices.Clone 0.35; concat of three 1.85 ms
  against 1.2; binary_search 44 ns against 30; max and min 0.8 ms
  against 0.63. A List[Int] of 64 or more elements is copied with one
  memory copy per list (ints_concat; 0.85 and 2.5 ms pushing); other
  element types push each element (as `a + b` does). Next: one SSA list
  copy (a memory copy plus a retain per element for counted elements)
  for every element type, `+` on lists and Map.clone's four lists;
  rt.cast between List[Float] and List[Int] is not native yet (XO1201).
- 2026-11-10: `Map.clone` on the native backends copies storage in bulk
  (compiler/ssa maphyb.xo maph_clone, maplog.xo mlog_clone; call.xo
  routes the call there before the native body): `xo bench --ab`
  bench/perf/mapclone wall 0.182 on linux/arm64. The Go backend keeps
  Map_clone (std/rt/map.go).
