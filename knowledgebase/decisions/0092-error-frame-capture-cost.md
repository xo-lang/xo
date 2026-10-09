# 0092: Error frames: always on, captured lazily, locations only in release builds (Appendix B question 3)

Status: Accepted (2026-10-09, by the owner; proposed by the spec audit)

## Context

Spec Appendix B question 3 asks what error frame capture costs in release
builds. Core 11.1: each `?` that propagates an error appends a frame with
the function, the location, and the `Debug` text of every parameter
(truncated to 64 bytes); "release builds record parameters only with
`--error-args`". There is no release build mode yet (core 8, 11.1 are
marked **[later]** since the audit).

How the Go backend does it today (`internal/gen/expr.go` `propagate`,
`std/rt/core.go` `Trace.Frame`, `FrameLabel`): every `Result` carries a
`Tr *Trace` pointer. On the error path only, a `?` calls
`FrameLabel(fn, name1, v1, ...)`, which formats each parameter with
`Debug` in full and then cuts it to 64 bytes, and allocates a `Trace`
node. The success path does nothing. The native backends record no frames
(core 8, decision 0085).

## Measurement

Micro benchmark (`std/rt/audit_bench_test.go`, `BenchmarkFramesError`,
`BenchmarkFramesSuccess`; darwin/arm64 M4 Pro, Go 1.26.1, medians of 3):
an error propagated through 5 frames, each function with an `Int` and a
`Str` parameter.

| Frames | short Str ("ada") | 90 byte Str |
|---|---:|---:|
| none (`return Err(e)`, no frame) | 20 ns, 0 allocs | 20 ns, 0 allocs |
| eager, as today (function, location, Debug of parameters) | 754 ns, 58 allocs, 1386 B | 2466 ns, 83 allocs, 3506 B |
| lazy (keep the parameter values, format when printed) | 214 ns, 18 allocs, 750 B | 215 ns, 18 allocs, 750 B |
| location only (function and location) | 71 ns, 5 allocs, 240 B | 71 ns, 5 allocs, 240 B |
| success path, eager design | 23.7 ns | |
| success path, lazy design | 24.5 ns | |

End to end in Xo (Go backend, `xo build`, 1,000,000 errors through 5
frames, `level(d: Int, id: Int, name: Str) -> Int ! LoadErr`, 5 runs):
`?` at every level costs 842 to 874 ns per error with a 3 byte name and
2635 to 2648 ns with a 90 byte name; the same program propagating with
`match ... Err(e) => return Err(e)` (no frame) costs 24 ns. So a frame
costs about 165 ns, plus about 0.4 ns per byte of every parameter's full
`Debug` text, because truncation happens after formatting: a 1 MB `Bytes`
parameter is formatted in full at each frame.

Reading:

- The success path is free in every design (the `Tr` pointer rides in the
  `Result`), so frames never slow correct code.
- Eager formatting is the whole cost: 90 percent of an error's cost with
  short parameters, 99 percent with long ones, and unbounded with large
  values.
- Values are immutable (core 5.1), so formatting later prints exactly what
  formatting now would: lazy capture changes no output.
- Against a request (an HTTP round trip is tens of microseconds), even
  today's cost is small; it matters in loops that use errors for control
  flow (parsers that try alternatives, lookups that fail often).

## Options

1. Keep eager frames everywhere; add release builds that drop parameters.
2. **Lazy frames everywhere**: a frame keeps the function, location, and
   references to the parameter values; `Debug` text is produced (and
   truncated while it is produced) only when a trace is printed, logged,
   written to a crash bundle, or read through `Fault.frames`.
3. Frames off in release builds: errors from production lose their trail,
   which contradicts design goal 7 (every failure reproducible) and the
   crash bundle (core 11.2).
4. Locations only by default, parameters only with `--error-args`
   (the spec's current text).

## Recommendation

Option 2, with the spec's release rule kept as an option, not a default:

- Frames stay on in every build, debug and release.
- Capture lazily (value references, formatted on demand); 3.5 times
  cheaper than today for short parameters and independent of parameter
  size.
- Bound `Debug` formatting at 64 bytes while formatting (stop early), so
  printing a frame never formats a whole large value.
- Parameters marked `Secret[T]` stay `<redacted>` (unchanged).
- A release build (when it exists, core 8) may drop parameter capture with
  `--error-args=false` for the smallest overhead (71 ns per 5 frames);
  parameters stay on by default because the evidence shows they are
  cheap once lazy and they are what makes a production error debuggable.
- Native backends adopt the same lazy frames when they start recording
  frames (core 8 gap).

## Consequences

- Core 11.1 changes when accepted: drop "Release builds record parameters
  only with `--error-args`", state that frames are lazy and parameters are
  kept by reference, and that `--error-args=false` exists for release
  builds. Appendix B question 3 is removed.
- `std/rt`: `Trace` gains a parameter slice and a formatter; `FrameLabel`
  becomes a bounded formatter; `internal/gen` passes values instead of a
  formatted label. Lazy capture keeps parameter values alive as long as the
  error lives (memory retention, bounded by the trace length).
- Arena allocated values (decision 0066) captured in a frame must not
  outlive the request: an error escaping a handler would have to copy its
  captured values out of the arena, or the arena analysis must treat a
  parameter captured by a `?` frame as escaping. This needs a test before
  the change ships.

## Alternatives

- Eager (today): measured 3.5 to 11 times slower than lazy, unbounded in
  parameter size.
- Off in release: loses the production trail.
- Locations only by default: 3 times cheaper than lazy but loses the
  values that explain most errors; kept as an opt out.
