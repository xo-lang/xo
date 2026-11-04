# 0489: Structs whose heap leaves are all enums borrow like enums

Status: Accepted (owner, 2026-11-04). Refines decision 0149.
Implemented in `compiler/ssa/rc.xo` (`enum_like`, used by `borrow_scan`).

## Context

Decision 0149 borrows an enum parameter unless every path from the entry
to a return consumes it, and keeps decision 0046's rule (owned once any
path consumes it) for every other type, because a borrowed List, Map or
Str that a path consumes is retained there and then copied at every in
place update (its first all types version made the checker 25 times
slower). bench/results/part-borrow-2026-11-02.md tried struct parameters
with some leaves consumed and parked it: every saving came with a
quadratic case through a Str or List leaf.

A struct whose only heap leaves are enums has nothing to update in place,
yet it took the struct rule. std/template's `Val{v: V, boxed: Bool}` is
one: `dot`, returned by eval_command on one path (`Dot(_) => Ok(dot)`),
became owned there and in every function that passes it down (the
pipeline, walk, walk_list, range_once), so each node of a render retained
and released it: `dup native.template.V` was 23% of bench/perf/template
render's profile (template render 1.16 to 1.21 of Go).

## Decision

- A struct or tuple parameter whose every leaf holding heap memory is an
  enum, or itself such a struct or tuple, takes decision 0149's rule: it
  is borrowed unless every path to a return consumes it, and a path that
  consumes it retains it there.
- Every other struct or tuple keeps decision 0046's rule.

## Consequences

- template render: `dup V` 23% to 7% of the profile on the Mac; linux/arm64
  `--ab` bench/perf/template render 0.923 (0.916 to 0.923, n=3), xo's self
  build 0.999 (0.997 to 1.001, within noise, n=7), bench/perf/json 0.995
  (0.984 to 1.004, within noise, n=7; a first n=3 run read 1.009).
- The full internal/arm64 suite passes; xo's s2 and s3 are identical.
- bench/perf/rcpart's rows stay flat (`rcpart check`: 20 rows, 0 slow);
  none of them has an enum only struct, which is why the rule cannot reach
  them: no leaf of such a struct is updated in place.
- A function that consumes such a struct on a rare path pays a retain
  there and its callers a release after the call, as decision 0149
  describes for enums.

## Alternatives

- **Every struct with a consumed leaf** (part-borrow, 2026-11-02): quadratic
  through Str and List leaves; parked.
- **Changing template** (dot passed as `V`, or returned by reference): fixes
  one std module, not the next struct of enums.
