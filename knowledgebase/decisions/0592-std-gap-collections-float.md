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
