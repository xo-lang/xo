# 0104: Error frames on the native backends

Status: Accepted (2026-10-09). Implements decision 0092 natively.
`internal/ssa/build_frames.go` (trace type, frames at `?`, printing),
`build.go` (`typ` of Result, `fn.add` filling new errors' traces,
`mainWrapper`), `build_call.go` (`try`, closures capturing the frame's
parameters), `build_conc.go` (`scopeFailure`), `build_map.go` (equality,
hashing, and order skip the trace), `build_show.go` (Debug of Atomic).
Test: TestErrorTracesNative (`internal/llvm/gen_native_test.go`) compares
the whole standard error of seven internal/gen goldens on Go, LLVM, and
arm64.

## Context

Decision 0092 made error frames lazy on the Go backend: each `?` that
propagates an error adds a frame (function, location, the parameters'
values), formatted only when printed, each value's Debug text cut to 64
bytes. The native backends recorded no frames: `main ! E` printed
`error: <text>` without the `  at ...` lines (spec 10.1, 11.1).

## Decision

1. **The trace rides in the error.** The Err variant of every lowered
   Result has a last, hidden field `xo.tr`: the struct `xo.trace
   {frames: List[(Str, Str, fn() -> Str)]}`, innermost frame first, as
   std/rt's `Result.Tr`. A new Err (every construction that is not a
   `?`) gets an empty list (`rt.rl_empty`, no allocation); `fn.add`
   fills it, so builder code that makes errors needs no change.
2. **A `?` adds a frame**: the function's name (`module.fn`,
   `module.Type.method`), the `file:line` of the `?`, and a closure over
   the values of the receiver and parameters at the `?` (capabilities
   left out) whose code formats `a=1, s="x"` with each Debug text cut to
   61 bytes and `...` past 64: lazy, as on the Go backend. A value whose
   Debug text changes in place (an `Atomic`) is formatted when captured,
   as std/rt's `FrameTextOf`. A `?` inside a closure records the
   enclosing function's frame; the closure captures that function's
   parameters for it. A scope failing because of a child adds the
   scope's frame. std and native implementation code records none.
3. **The trace is not part of the error's value**: `==` (through the
   builder's `xo.eqc` helpers, never a backend helper), hashing, order,
   and Debug skip it.
4. **Printing**: main's report writes `error: <text>` and then `  at
   <label padded to 34 runes> <location>` per frame, as std/rt's
   `Trace.Lines`.

## Consequences

- Every native program's main error report now matches the Go backend's
  frame for frame (the crash bundle line is still Go only).
- An Err payload is one word larger; a `?` on the error path allocates
  the frame's closure and pushes on the list (in place when unshared).
  The success path is unchanged. Hello world is unchanged.
- Not yet: frames in `xo test --no-gc` failure reports, in HTTP handler
  error logs, through `Timed.Failed` and `Isolated.Failed` (their error
  is rewrapped without the trace), `--release --error-args=false`, and
  fault frames (`Fault.frames`, crash bundles).

## Alternatives

- **A side channel in the task context** (the last trace set by a
  returning function): cheaper, but wrong when an error is kept in a
  variable while other calls fail, and when a handled error's trace is
  still there for the next one.
- **Eager formatting at `?`**: simpler (a Str per frame) but the cost
  decision 0092 removed.
- **A trace handle in the C runtime**: printing needs the formatting
  closures anyway, which only generated code can call.
