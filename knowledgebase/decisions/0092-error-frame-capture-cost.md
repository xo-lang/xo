# 0092: Error frames: always on, captured lazily, locations only in release builds (Appendix B question 3)

Status: Accepted (2026-10-09, by the owner; proposed by the spec audit).
Implemented on the Go backend 2026-10-09 (see "Implementation" below);
the native backends record no frames yet and adopt the same design when
they do.

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
  builds. Appendix B question 3 is removed. (Done 2026-10-09; "by
  reference" became "by copy", see Implementation.)
- `std/rt`: `Trace` gains a parameter slice and a formatter; `FrameLabel`
  becomes a bounded formatter; `internal/gen` passes values instead of a
  formatted label. Lazy capture keeps parameter values alive as long as the
  error lives (memory retention, bounded by the trace length).
- Arena allocated values (decision 0066) captured in a frame must not
  outlive the request: an error escaping a handler would have to copy its
  captured values out of the arena, or the arena analysis must treat a
  parameter captured by a `?` frame as escaping. This needs a test before
  the change ships. (Done 2026-10-09: no arena value can be a captured
  parameter; golden `arena_frames`, see Implementation.)

## Implementation (Go backend, 2026-10-09)

- Release builds exist (`xo build --release`, decision 0097). Frames are
  recorded in every build; `xo build --release --error-args=false` keeps
  the function and location and prints `(...)` for the parameters
  (`--error-args=false` without `--release` is a usage error, exit 2).
- Runtime (`std/rt/frames.go`): `Trace{Prev, Fn, Loc, Names, Args}`.
  `Frame1` to `Frame6` are generic and keep the frame and the parameter
  values in one allocation (`frameN[A, B, ...]` embeds the `Trace`, whose
  `Args` points back at it); `FrameN` takes more parameters as `[]any`;
  `Frame0` has no values. `Trace.Lines` formats on demand. It is the only
  reader of frames: main's error report, crash bundles, test failures,
  and handler error logs all go through it. (`Fault.frames` of
  `task.isolate` lists a fault's Go stack frames, which never held
  parameter values, so nothing changed there.)
- Bounded formatting: `debugBounded` formats like `Debug` but stops at
  65 bytes. `debugTo` checks the builder's length on entry (one atomic
  load of the registry of bounded builders, `boundedActive`, when none is
  active) and panics with `debugStop`, recovered by `debugBounded`;
  strings, byte strings, builders, and Go error texts are quoted only as
  far as the limit needs (`writeQuoted`). Everything written is a prefix
  of the full `Debug` text, so the 64 byte cut (61 bytes and `...`) is
  byte identical to the eager one (`TestFrameDebugBounded` compares both
  on edge cases: exactly 64 and 65 bytes, multibyte and invalid UTF-8 at
  the cut, escapes, nesting, maps, secrets). A 1 MB parameter costs what
  a 90 byte one does.
- Generator (`internal/gen/expr.go` `frameCode`, `frameValue`): a `?`
  emits `rt.FrameK(tr, fn, loc, "a b", a, b)`; capabilities are left out
  as before. Every existing golden's error output is byte identical.

### Value semantics: why a lazy frame shows the value at capture time

A frame must print what an eager frame would have printed at the `?`.
The values are copied into the frame when the `?` runs, so later
rebinding of a parameter or of the caller's variable (including a `var`
parameter, whose Go form is a pointer: the frame copies `*p`) cannot
reach it. What remains is memory the copy shares with something that can
still be written in place:

- `Str`, `Bytes`, scalars, enums, structs of those: immutable once
  copied.
- `Map` and `Set`: a persistent append only log; a copy sees only the
  entries up to its own length, later writes append past it or fork.
- `List`: written in place while its block is not marked shared
  (`internal/gen/own.go`). A `var` List parameter and a List lent to a
  plain parameter (a borrow) both share the caller's block, so the frame
  takes the value through `ownCode`, which marks it shared: the caller's
  next write in place copies first. Without the mark, the golden
  `error_frames_borrow` printed `xs=[9, 2, 3]` instead of `[1, 2, 3]`,
  and `error_frames_capture` printed the caller's later `xs[1] = 99`.
- `Buf` (its `freeze` turns the array into an unshared List that is then
  written in place) and `Atomic` (its Debug text is its current value):
  formatted when captured, bounded (`rt.FrameTextOf`, `gen.frameEager`,
  also for structs, enums, and tuples holding them and for unspecialized
  type parameters). Without it the golden printed `b=Buf[55, 1]` and
  `count=Atomic(9)` instead of `Buf[5, 1]` and `Atomic(1)`.
- Secrets stay `<redacted>`; handles (Mutex, Chan, capabilities) print a
  constant text.

Remaining limit: a value reached only through an interface (a `Display`
parameter holding a struct with an `Atomic` field) is not visible to the
static check, so its Atomic would print the value at print time. No
golden or benchmark program does this.

Arenas (decision 0066): a frame captures only parameter bindings. A
confined handler's request is never a parameter of a frame (the handler
closure's frames list the enclosing function's parameters, and the
request may only be read through methods that return heap values), and
arena temporaries are never bound. The golden `arena_frames` keeps the
Results of 20 failed checks of request data in a Mutex while 60 requests
reuse the arena, then prints the first one's frames intact, with and
without `--no-arenas`.

### Measurements (darwin/arm64 M4 Pro, Go 1.26.1)

`go test ./std/rt -run '^$' -bench Frames -benchmem -count=3`
(`std/rt/audit_bench_test.go`; 5 frames, an `Int` and a `Str` parameter;
medians):

| Frames | short Str | 90 byte Str |
|---|---:|---:|
| before: eager (emitted until this change) | 781 ns, 58 allocs, 1386 B | 2587 ns, 83 allocs, 3506 B |
| after: as emitted now (`Frame2`) | 97 ns, 5 allocs, 480 B | 97 ns, 5 allocs, 480 B |
| no frames (`return Err(e)`) | 21 ns | 21 ns |
| function and location only | 85 ns, 5 allocs | 85 ns, 5 allocs |

The implementation beats the audit's prototype (`lazy`, 219 ns, 18
allocs) because the values share the frame's allocation instead of
being boxed one by one. The cost moved to printing: formatting the 5
frames takes 1.9 us (short), 4.5 us (90 bytes), and 4.6 us with a 1 MB
parameter (`BenchmarkFramesPrint`), against a full format per frame
before. The success path is unchanged (24 ns, no frame code runs).

End to end (`xo build`, 1,000,000 errors through 5 frames of
`level(d: Int, id: Int, name: Str) -> Int ! LoadErr`, 5 runs): before
920 to 943 ms (3 byte name) and 2770 to 2787 ms (90 byte name); after
95 to 97 ms for both (first run 118 ms); `--release --error-args=false`
82 to 84 ms. About 10 times faster with short parameters and 29 times
with long ones.

## Alternatives

- Eager (today): measured 3.5 to 11 times slower than lazy, unbounded in
  parameter size.
- Off in release: loses the production trail.
- Locations only by default: 3 times cheaper than lazy but loses the
  values that explain most errors; kept as an opt out.
