# 0508: A list builder reuses its receiver at the receiver's last use

Status: Accepted (owner, 2026-11-04, through the Lead: "508 accept"); proposed by xo-lang-4 the same day

## Context

bench/perf/micro13's sort1M ran `xs.freeze().sort()`, and Xo's `sort`
built a second 8 MB list that Go's in-place `slices.Sort` never makes
(1,529 page faults against 1,003 on the Mac; bench/results/
sort1M-in-place-2026-11-04.md). The Lead asked for an ADR on handing a
dead temporary's list to `sort`, `map`, `filter` and the other builders.

What the investigation found first: the compiler already hands the
reference over. At an argument's last use the caller passes its reference
to a parameter the callee consumes, so `fn f(xs: List[Int]) -> List[Int]
{ var a = xs; a.sort_in_place(); a }` called as `f(xs.freeze())` writes the
frozen list's own array (1,042 page faults, the same as no sort). `sort`
copied only because of list_sort's own body, which read its parameter
again after giving it to rt.any_of; that was fixed in std alone
(98e218988: darwin/arm64 max RSS 0.59 for sort1M_copy, i7 laptop 78 to 75
ms, Go 75). So no compiler change is needed: a builder can reuse its
receiver by moving it into a `var` and writing it, and copy-on-write copies
only when another binding still holds the list.

How common the shape is (a builder called on a call's result, `f(..).m(`,
origin/main 2026-11-04, `_test.xo` files left out; the counts include
Option.map and similar methods with the same name, so they are upper
bounds):

| method | std and compiler | bench programs |
| --- | --- | --- |
| map | 19 | 30 |
| filter | 10 | 14 |
| sort_by | 7 | 17 |
| reverse | 4 | 2 |
| sort | 52 | 7 (fixed by 98e218988) |

Most std sites run on small lists (`keys()`, `values()`, `split(..)`),
where reuse saves one allocation per call; the large-list cases are in the
bench programs and user code that streams data through `map`.

Measured on prototypes written as user code (darwin/arm64, 1M Ints made
by a function and frozen, `xo bench --ab --no-guard --rounds=15
--warmup=1`, --release, A = the std builder, B = the prototype):

```
map, f: Int -> Int, written into the receiver
wall_ms               6.852       5.665       4.159% 3.142%     0.827   0.813 to 0.854    better (n=15)
max_rss_kb            27568       19744       0% 0%             0.716   0.716 to 0.716    better (n=15)
filter, kept elements compacted into the receiver (half kept)
wall_ms               6.19        6.71        1.931% 2.213%     1.084   1.076 to 1.108    worse (n=15)
max_rss_kb            19744       19744       0% 0%             1.0     1.0 to 1.0        within noise (n=15)
```

## Decision

Std only; no language, checker or compiler change, and no change to any
result:

1. `List.map` whose result element type is the receiver's (`U` = `T`,
   tested on empty lists as list_sort does, so the test takes no reference)
   moves the receiver into a `var` and writes each result over its
   element. When the call is the receiver's last use the list's own array
   is written; when another binding holds it, the first write copies it,
   as every `var` list write does, so that holder never sees a change. A
   closure that captures the list holds a reference, so it reads the
   original elements.
2. `sort_by` of a receiver nothing else holds runs `sort_in_place_by` on
   it, if a `--ab` on sort_by's bench rows shows it is not slower (the
   in-place form sorts indices by keys and then permutes, a different
   algorithm from sort_by's); otherwise it stays as it is.
3. `filter` stays as it is: compacting in place was 1.084 of its wall time
   and saved no memory, since its result is usually smaller than its
   receiver and the receiver's array is freed at the same point either way.
4. `reverse` may swap in place by the same rule; with 6 sites it goes last.

Each step lands with its std/changes entry and an `--ab` on linux/arm64
and darwin.

## Consequences

- `xs.map(f)` with the same element type, on a list nothing else holds,
  allocates nothing: 0.827 of the wall time and 0.716 of the peak memory on
  1M Ints. Chains such as `lines.map(trim).filter(..)` gain on the `map`
  step.
- The rule is the one users can already write themselves (`var a = xs`,
  then write `a`), so it adds no concept to the spec; spec/std.md gets a
  sentence per builder, like the one `sort_in_place` has.
- A `map` whose closure faults part way leaves the receiver half mapped;
  only the builder held it, so nothing can observe it.
- Element types with counted fields (Str, lists, structs holding them):
  writing over an element releases the old one, which the closure has
  already read; the prototype covered Ints only, so step 1 adds a test with
  Str and struct elements.

## Alternatives

- Compiler: rewrite `x.m(f)` at x's last use into an in-place form. Not
  needed: the reference already moves at a last use, and a builder that
  writes a `var` gets the copy-on-write check for free.
- A `map_in_place` method for users. Adds API for what the builder can do
  itself; a user who wants it can already write the loop.
- Leave the builders alone: the sort case was the large one and is fixed.
  The map figures above say there is more to gain on large lists.

## Implementation notes (2026-11-05)

- Step 1 sees f once as a `fn(T) -> T` (`rt.any_get` of `rt.any_of` of
  f), not each result as a T: converting each element through Any boxed
  it and ran 10.2 times main's time on 1M Ints.
- The type test costs two small allocations per call, so the in-place
  path starts at MAP_IN_PLACE_MIN = 256 elements. With no threshold four
  Ints ran 2.4 times main's time (Mac); at 32, lists of 32 ran 1.072
  (1.043 to 1.095) of main's time on linux/arm64 (bench/perf/listmap mid).
- The fn(T) -> T Option exposed a native compiler bug: ssa type_text
  named a function type over a struct name two modules declare without
  the module, so two modules' types became one (XO1201 at build).
  Fixed with the regression golden fn_type_same_name_structs
  (gotchas/fn-type-name-drops-module.md); compiler/cli's modules.Change
  is renamed ModChange so a seed without the fix still builds xo.
