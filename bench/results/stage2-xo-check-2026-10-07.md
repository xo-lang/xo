# Stage 2: Xo with the checker in the loop (2026-10-07)

Eight fresh agents wrote tasks 01 to 08 in Xo (spec 0.4) and iterated against
the frozen checker `.tools/bin/xo-0.4` until 0 errors. Solutions, build logs,
and notes are in `bench/xo/v0.4/<task>/`. The compiler cannot run Xo yet, so
"clean" means type checks, not tests pass.

## Runs until a clean check

| Task | Xo runs to clean | First run errors | Plain Go runs to green | Xo lines | Go lines (plain) |
|---|---|---|---|---|---|
| 01 CRUD API | 1 | 0 | 1 | 321 | 654 |
| 02 Worker pool | 1 | 0 | 1 | 241 | 639 |
| 03 Retry client | 1 | 0 | 1 | 325 | 710 |
| 04 File sync | 1 | 0 | 1 | 383 | 862 |
| 05 Rate limiter | 1 | 0 | 1 | 285 | 586 |
| 06 Job queue | 1 | 0 | 2 | 786 | 1270 |
| 07 Cache | 1 | 0 | 1 | 616 | 1200 |
| 08 Notes service | 2 | 4 (all checker bugs) | 1 | 1149 | 1782 |

Agents also ran deliberate probes (planting a type error in a test block,
removing a `uses` clause); the checker caught every probe. Extra runs for
probes or `--json` confirmation are not counted above.

Not comparable yet: Go "green" includes running tests; Xo "clean" does not.
Line counts include tests, and Go solutions wrote more tests.

## Checker bugs found by the subjects

1. `testing` is not predeclared in helper function signatures in `_test.xo`
   files (core 9.2 says it is). Workaround: `use std/testing`.
2. That error cascaded into XO0405 (core 10.2 forbids cascades).

## Checker strictness: mutation probe

`bench/xo/mutate.py` applied 381 single line mutations to the stage 2 code and
reran `xo check`. Raw data: `bench/xo/v0.4/mutations.jsonl`.

| Mutation | Models | Caught | Misses that are legal code |
|---|---|---|---|
| remove `?` | ignored error | 58/58 (100%) | none |
| `var` to `let` | mutation rules | 48/48 (100%) | none |
| delete a match arm | missing case | 159/164 (97%) | all 5: a `_` arm still covers it |
| `None` to `0` | optional misuse | 48/53 (91%) | all 5: `0` is accepted where `Int?` is expected |
| delete a `uses` clause | effects | 10/13 (77%) | all 3: non pub functions infer effects |
| delete a `put`/`push`/`set` | lost update | 17/31 (55%) | 9 are plain logic or test edits; **5 are real copy out lost updates missed** |
| `<` to `<=` | logic (control) | 0/14 | expected: not a checker target |

The real gap is the lost update rule: XO0520 misses a copy out of a
collection that is then changed (directly, or derived with `with`) and read
afterwards but never stored (`01-crud-api/store.xo:121`, four in
`06-job-queue/queue.xo`). This is the same hole as planted Go bug 01 X6,
which the earlier estimate wrongly counted as caught. See the gotcha
`xo0520-misses-read-after-mutation.md`.

## Spec issues raised (for 0.5)

1. **XO0520 rule** should require the store back regardless of later reads,
   and cover values derived with `with` from a copied out element.
2. **`is` with `&&`:** the grammar line allows `is` only as a whole binary
   expression, but the prose example `resp is Ok(_) && retries < 3` combines it.
3. **Tail `match` arms mixing `T` and `Err(e)`** are accepted (each arm is in
   return position); the spec should say so.
4. **Wildcard arms hide new variants**: consider a `vet` warning for `_` on an
   enum from the same module.
5. Std gaps: no range to `List`, no path helpers (`path.dir`, `path.join`), no
   `Patch.apply_to`, no `Time.from_unix_millis`, no hex/base64 helpers, no
   one shot broadcast (`task.Once` or multi await handles), no deadline
   variant of `map_concurrent` that keeps finished results, no `while let`
   over channels, no `Atomic` max.
6. Behavior gaps that need the runtime: does a just spawned task run before
   `FakeClock.advance` fires timers; working directory under `xo test`; signal
   delivery in tests.

## Friction reported in this round

- Keep and fail `Mutex.with` boilerplate repeated per method (06).
- Cleanup on cancellation still needs a flag read in a shielded `defer` (04, 06).
- Two nested closures to update a map entry under a lock (05).
- Explicit Float/Int/Duration conversions in rate math (05).
- Effect and constraint plumbing in generic code (07).

## Next experiments

1. Fix the two checker bugs and the XO0520 rule (spec 0.5), rerun the mutation
   probe and require 5/5 on the real copy out cases.
2. When code generation lands: run every stage 2 solution's tests (`xo test`),
   which turns "type checks" into "works" and makes the Go comparison fair.
3. Plant the 30 safety bugs directly in the stage 2 Xo solutions and measure.

## Update: checker fixes (decision 0037, spec 0.5)

Done directly, no agents. Both checker bugs fixed with regression tests
(`internal/check/rules05_test.go`). XO0520 now requires a store back after the
last change. Mutation probe rerun: removed store backs caught 18/31 (was 17);
the read after case (`01-crud-api/store.xo:121`) is now caught. The remaining
copy out misses are 4 `with` derivations in task 06, untracked by design. All
8 stage 2 solutions still check clean, and the v0.3 corpus diagnostics are
unchanged (3 XO0206, 1 XO0601).
