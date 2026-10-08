# Type checker over the v0.3 paper corpus (2026-10-07)

Checker: `xo check` (P1, `internal/check`), spec `spec/core.md` and
`spec/std.md` draft 0.3. Corpus: every module under `bench/paper/v0.3/`
(tasks 01 to 08, 13 modules, written blind from the spec by other agents).
Reproduce with:

```
go run ./cmd/xo check bench/paper/v0.3
go test ./internal/check -run TestCorpusV03 -v                 # same, as data
go test ./internal/check -run TestCorpusV03 -corpus-report     # rewrites the generated section below
```

The corpus test never fails on diagnostics; they are data.

## Summary

| Category | Diagnostics |
|---|---|
| (a) real violation of the spec in the solution | 1 (XO0601) |
| (b) checker bug, fixed | 30 in the first run (9 XO0601, 6 XO0401, 3 XO0701, 3 XO0404, 2 XO0702, 2 XO0407, 2 XO0302, 2 XO0203, 1 XO0204), plus 1 silent bug found by mutation testing |
| (c) spec gap or ambiguity | 0 diagnostics now; the gaps below are where the checker had to choose a reading, and every corpus `GUESS` it depends on is accepted |

Final run: 13 modules, 1 diagnostic. Every binding and expression in the
corpus is fully typed (no `<error>` type and no open inference variable
outside the one reported expression), so the low count is not the checker
skipping code; see "How the result was validated".

## Generated counts

<!-- generated:begin -->
Modules checked: 13. Diagnostics: 1.

| Code | Count | Meaning |
|---|---|---|
| XO0601 | 1 | name does not resolve (core 4.3) |

| File | Diagnostics by code |
|---|---|
| `bench/paper/v0.3/04-file-sync/sync_test.xo` | XO0601 x1 |

| Diagnostic |
|---|
| `bench/paper/v0.3/04-file-sync/sync_test.xo:84:19` XO0601: name `FsOp` not found |
<!-- generated:end -->

## Judgment per code (final run)

### XO0601 (name does not resolve): 1

- `bench/paper/v0.3/04-file-sync/sync_test.xo:84:19`: name `FsOp` not found
  (`os.fs.fail_next(FsOp.Rename, ...)`).

Judgment: (a) real violation, with a (c) caveat. `FsOp` is declared in
`std/testing` (std.md 8.3), and only `testing` itself is predeclared in tests
(core 9.2), so the name must be written `testing.FsOp`, which is what task 08
did (with a GUESS marker). std.md 8.3 lists `fail_next(op: FsOp, ...)` without
saying how a test names the enum, so the mistake is easy. Bare variants would
work with an expected type (`fail_next(Rename, ...)`), qualified ones need the
module. The diagnostic now carries a note and a fix: `qualify as
testing.FsOp`.

## First run: checker bugs found and fixed

The first full run reported 31 diagnostics. One was the XO0601 above; the
other 30 were checker bugs, all fixed (each fix has a regression test in
`internal/check/rules_test.go`):

| Code | Count | Example messages | Cause and fix |
|---|---|---|---|
| XO0601 | 9 | `type fx not found` (cache.xo:112); `module net not found` (itemsvc.xo:71, for `Cache[Int, Item, UpstreamErr, net.dial]`); `field or method of E display not found` (cache.xo:31) | Effect parameters on structs (`struct Cache[K, V, E, fx: Effects]`) and effect sets as type arguments were not supported; implicit receiver type parameters did not inherit the constraint of the type's parameter (`E: Display`). Added `EffArg` type arguments and constraint inheritance. |
| XO0401 | 6 | `expected Result[(), Never], found ()` (pool.xo:44); `expected Decision, found Result[Decision, Never]` (limiter.xo:83) | A generic call's result was unified with the expected type before its error variable was known; an enclosing closure's open result became `Result[R, E]` and `E` later defaulted to `Never`. See `knowledgebase/gotchas/checker-hint-binds-open-error.md`. |
| XO0701 | 3 | ``the expected function type allows only `fx`, but performs `fx` `` (cache.xo:98); `` `Cache.get` declares `uses clock, fx`, but performs `fx` `` (cache.xo:112) | Struct effect parameters (as above), and inferred effects of generic non pub methods were not instantiated per call. See `knowledgebase/gotchas/checker-generic-effects-per-call.md`. |
| XO0404 | 3 | `` `E` does not implement `Display`, required by `E` of `decide` `` (cache.xo:114) | Implicit receiver parameter without the type's constraint (as above). |
| XO0702 | 2 | ``unknown effect `fx` `` (cache.xo:112) | Implicit effect parameter of a receiver not created. |
| XO0407 | 2 | `` `testing.FsOp` is a type, not a value `` (store_test.xo:26); `` `fx` is an effect variable, not a type `` (cache.xo:96) | Predeclared `testing` was not resolved in type position; effect arguments as above. |
| XO0302 | 2 | `` `TimedOut` is a variant of `task.Timed`, but the value is `Never` `` (cache.xo:233) | Follow on of the early unification bug. |
| XO0203 | 2 | `the success value of Result[V, task.Timed[E]] ! _ is itself a Result` (cache.xo:228, cache_test.xo:87) | Same early unification bug. |
| XO0204 | 1 | `` `?` on an optional value in a function that returns a Result `` (queue.xo:142) | A closure with an explicit `-> Lease?` and no `!` was given the generic callee's open error variable; an explicit signature without `!` now means the closure cannot fail. |

Silent bug found by mutation testing (no diagnostic at all): variant
pattern fields of a non generic enum were typed `<error>`, so code under
patterns like `Again(err, hint) => ...` was unchecked. Removing a match arm
inside such a block was not reported. Fixed; see
`knowledgebase/gotchas/checker-error-type-hides-bugs.md`.

## How the result was validated

A clean run on a careful corpus is only meaningful if the checker actually
checks it. Three checks:

1. Every local binding and expression in the corpus has a type without
   `<error>` or open inference variables, except the one reported
   expression.
2. Mutation testing (`go test ./internal/check -run TestCorpusMutations
   -mutations -v`): each mutation is applied at one site in a copy of one
   task, and the run counts as a detection when the diagnostics differ from
   the unmodified task.

| Mutation | Sites | Detected | Not detected, and why |
|---|---|---|---|
| remove a postfix `?` | 220 | 217 | 3 legal: the value is then returned as is from a `-> T ! E` function (2), or discarded by `let _ =` (1). Before gap 2 below was closed, `let _n = <Result>` was a fourth, silent one. |
| delete one match arm | 158 | 146 | 12 legal: a catch all (`_`, `Err(e)`, `Storage(inner)`) remains, or the deleted arm had a guard |
| `var` to `let` | 75 | 75 | 0 |
| delete a numeric conversion `Float(`/`Int(` | 4 | 4 | 0 |
| delete a `derive` clause | 15 | 15 | 0 |
| typo in a member name (every third site) | 233 | 220 | 13, all in comments or doc text |
| delete a `uses` clause | 10 | 8 | 2 legal: non pub functions, whose effects are then inferred |

3. The older v0.2 corpus (written for `{x}` interpolation) gives 41
   diagnostics, all explained by the 0.2 to 0.3 changes: 32 XO0110 for
   variables only used inside `"{x}"`, which is literal text since 0.3, and 9
   XO0401 for `?` on `Fs.sub`, which became infallible (lazy) in 0.3.

## Spec gaps and ambiguities met while checking the corpus

These produced no diagnostic in the final run because the checker accepts
the corpus' reading; each is listed with the reading implemented. The full
list for the checker is in the final report of the work that added it.

1. Effect parameters on structs and effect sets as type arguments
   (`Cache[Int, Item, E, net.dial]`, task 07, GUESS). The grammar allows
   `fx: Effects` in any type parameter list (core 15) but core 6.2 shows only
   functions. Accepted: an effect argument is a label, an effect variable, or
   a parenthesized list.
2. `let _n = fallible()` (task 08 test shows the pattern with `?`). Core 2.6
   counts a bound Result as handled and core 3.1 exempts `_` names from
   XO0110, so together they let an error be ignored silently. Chosen: an
   unused binding of Result type is XO0201 whatever its name; only `let _ =`
   discards.
3. XO0520 copy forms (core 5.2, decision 0023) do not include
   `m.get(k).or_err(e)?`, which task 01 uses (and writes back correctly). A
   forgotten `put` there would not be reported. Proposal: add `.or_err(...)?`
   and `.or_fault(...)` to the list.
4. Generic method receivers (`fn (c: Cache[K, V, E, fx]) get`, task 07,
   GUESS): implicit receiver type parameters, inheriting the constraints of
   the type's parameters. Accepted.
5. A closure whose tail is a `Result` (task 07, GUESS): treated as a failing
   closure that returns the Result as is (core 2.6), so `s.spawn(fn() {
   fetch(id) })` spawns a failing task. If the expected success type is
   itself a Result, XO0203 applies to closures too.
6. Prefix omission for other modules' enums (01, 03; also listed in the 0.3
   paper results): bare variants are accepted whenever the expected type is
   known, for any module; qualified forms always work.
7. `Never` satisfies every constraint (`r.handle(...)` with a handler that
   cannot fail binds `E: Error` to `Never`; 01, 06 GUESS). Accepted.
8. A pub function that returns a closure (task 05, GUESS): creating a closure
   performs nothing; calling it performs the effects in its type.
9. `?` on an Option inside a closure declared `-> T?` (task 06, GUESS):
   accepted.
10. `Hash` implies `Eq` and there is no syntax for several constraints (task
    07, GUESS): constraints on `Map` keys are not checked.
11. `Err(e)` returned from a `! Error` function converts `e` to `Error`
    (task 04, GUESS): accepted as implicit conversion 2 (an interface
    expected in return position), which needs `e` to implement `Display`.
