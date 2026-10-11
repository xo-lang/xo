# 0110: Every checked program builds: a compile property test

Status: Accepted (2026-10-09). Test: TestCompileProperty
(`internal/gen/compileprop_test.go`).

## Context

A `select` arm without a binding passed `xo check` and failed `go build`
as a code generator bug (gotchas/select-unbound-recv-unused-slot.md): no
golden used that optional part of the syntax. Goldens cover the forms
someone thought of; the combinations of optional parts grow faster than
anyone writes goldens.

## Decision

1. A generator writes programs from fixed seeds that use the optional
   parts of the syntax in random combinations and nested in one another:
   select arms with and without a binding, with `send` and `after`, in
   any order; closures with declared or inferred parameter, result, and
   error types; match arms with literal, range, tuple, variant
   (positional, named, `..`), struct, list, newtype, and or patterns that
   bind, with guards; `defer` (a block or a statement); scopes, `spawn`,
   `spawn_background`, groups, `await`; `?`, `??`, optional chaining,
   interpolation, Map and List literals, struct `with`; `while` with
   `break`, `continue`, labels, and as a statement ending a function.
   Xo has no `break` with a value; a `while` used as a value is `()`.
2. Each program must check clean (a diagnostic is a bug of the
   generator), build on the Go backend, and, when the native check
   accepts it, build with `--no-gc`; then both run and must exit alike
   with the same standard output and first line of standard error (the
   deadlock fault without its location, decision 0083). Since
   2026-10-11 all of standard error is compared, normalized only for
   what decision 0083 lets differ (fault and error frames, the
   deadlock's location and blocked task list, crash bundle paths;
   `normStderr` in internal/gen/compileprop_test.go).
3. `-short` runs 24 programs (seeds 1 to 24, in parallel, a few
   seconds); the full suite 150. `XO_COMPILE_PROPS=N` runs N,
   `XO_COMPILE_PROP_SEED=S` one, `XO_COMPILE_PROP_KEEP=dir` keeps them.
4. Every bug it finds gets a golden of its own.

## Consequences

- Found so far: a receive arm after an `after` arm lost its value
  natively (golden internal/llvm `select_arm_order`). Over 1,000 seeds
  built and ran alike after that fix.
- New syntax should be added to the generator with its optional parts.

## Alternatives

- **Grammar driven generation then filtering by the checker**: most
  programs would not check; type directed templates check by
  construction.
- **Build only**: the run comparison costs little more and found the
  only bug so far.
