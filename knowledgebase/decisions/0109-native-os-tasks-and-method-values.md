# 0109: Native os.tasks, method values, and map_concurrent_until

Status: Accepted (2026-10-09). Implemented in `internal/ssa/build_tasks.go`
(`tasksMain`, `rootScope`), `build_testmode.go` (a scope around each
test), `std/llrt/xo_rt.c` (`xo_prog_group_set`, `xo_prog_group`,
`xo_prog_group_clear`), `internal/check/methodvalue.go`
(`Info.MethodValueLit`), `internal/ssa/build_expr.go`, and
`std/native/task.xo`. Goldens: internal/gen `os_tasks`, `arena_escape`,
`error_frames_closures` (TestGenGoldensNative, TestErrorTracesNative),
internal/llvm `method_values`, `map_until`; cmd/xo
`testdata/nogc/same/fakes_test.xo`, `testdata/nogc/native`.

## Context

Three XO1201 rows the audit listed as planned: `os.tasks` (the program
lifetime task group), method values (`let f = x.method`), and
`task.map_concurrent_until`.

## Decision

1. **`os.tasks` is the group of a scope around main**, as std/rt's
   RunMain: when a program reads `tasks` of an `Os` (a scan of the
   program's modules), `main` becomes a wrapper that opens a scope,
   makes it the program's group in the C runtime (`os.tasks` reads it,
   retained), calls the program's main, and closes the scope: background
   tasks are canceled, the others awaited. A group task that failed
   (XO0801 allows that only for spawns in main's body, where the group's
   origin is visible) is the program's error, `error: <text>` with its
   frames and status 1, before main's own error. Unwinding out of main
   closes the scope as any scope's unwind block does. A test binary
   opens such a scope around the test it runs and closes it canceling
   every task (std/rt's runInWorld: TestOs.tasks lives for the test); a
   failed group task fails the test.
2. **A method value is a closure literal.** The checker gives a code
   generator `fn(p0, ...) { r.method(p0, ...) }` for `x.method`, its
   parts recorded as if checked (so the call lowers as any method call:
   user, std, prelude, or interface), and `r`, a binding the generator
   sets to the receiver's value where the value is made. The receiver is
   evaluated once, then, as on the Go backend.
3. **`map_concurrent_until`** is Xo over `with_timeout` and
   `map_concurrent`: each item's result is stored by index under a
   Mutex as it finishes, an item started after the deadline does not run
   f, and an item without a result is `Err(TimedOut)`. The scope that
   `with_timeout` opens joins every task before it returns, so the list
   is complete then.

## Consequences

- Programs that never mention `os.tasks` keep their main as it was.
- A `?` inside a closure now records `<function>.closure` (once per
  closure level) natively, as the Go backend's frames do; a closure given
  to `Mutex.with` records a frame too (both were found while comparing
  os_tasks's error report).

## Alternatives

- **`os.tasks` as a field of the lowered Os**: Os is a zero size
  capability natively; a field would change every capability's layout
  and every call that passes one.
- **Method values lowered in the SSA builder from the receiver's value
  and the method object**: needs a second lowering of every kind of
  method call (prelude methods are lowered from their call syntax); the
  synthetic literal reuses the existing one.
