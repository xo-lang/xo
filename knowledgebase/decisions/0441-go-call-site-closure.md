# 0441: Go backend: a bound call runs its adapter at the call site, under the caller's //line

Status: Proposed (xo-lang-6, 2026-10-30; the lead's item after the zap caller survey)

## Context

Every `use go` call goes through one adapter function in std/rt's
generated binding file (`M_name`, or `Go_pkg_name`: the conversions, the
fx replay hook, and the `GoEnter(tk); defer GoExit(tk, name)` boundary
that turns a Go panic into an Xo fault and tracks the task's time in Go).
A Go library that records its caller (zap's AddCaller, logrus's
ReportCaller, slog's AddSource, runtime.Caller in general) therefore
names the adapter's line in the generated Go, not the Xo line (the owner's
Windows testing, 2026-10-30; documented with `zap.add_caller_skip(1)` in
1b6771a9c). Survey of the zap workflow's 32 bindings: 2141 adapters, 97%
with the fx hook, 43% returning Result[_, Go_Error], 2.3% argument
passthrough, every call through exactly one adapter frame (two for the
replayValue mode). A direct call is not possible: GoExit's recover must be
deferred by the frame that made the call and run right after it.

## Decision

(To fill in.) The generated call site becomes a closure on one line under
the caller's //line directive, whose body is the adapter's; the frame
stays, its position is the Xo line. gobind emits each adapter's body in a
pasteable form; compiler/gen pastes it. Measured before pushing on the zap
workflow and a larger use-go program: generated Go lines, go build wall
(`xo bench --ab`, n >= 10), binary size; a build wall over 10% slower is
reported first.

## Consequences

(To fill in.)

## Alternatives

(To fill in.)
