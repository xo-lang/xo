# 0441: Go backend: a bound call runs its adapter at the call site, under the caller's //line

Status: Accepted (owner, 2026-10-30). Proposed (xo-lang-6, 2026-10-30; the lead's item after the zap caller survey)

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

The Go backend emits a bound call as the adapter's body applied at the
call, a function literal whose every line is under the Xo call's //line
directive, in place of the adapter call (shown on one line here):

```go
//line main.xo:19:3
func(h rt.Go_go_uber_org_zap_Logger, tk *rt.Task, a0 string, a1 rt.List[...]) { if rt.GoFxOn() { rt.GoFxRerun(tk, "zap.Logger.info", "handle") }; rt.GoEnter(tk); defer rt.GoExit(tk, "(*zap.Logger).Info"); h.P.Info(a0, ...) }(v_child, tk, "order placed", ...)
```

The frame stays (GoExit's recover must be deferred by the frame that
made the call), but its position is the Xo line: runtime.Caller from the
library sees `main.xo:19`, and a library that records its caller records
the Xo line without a caller skip.

- gobind (internal/gobind gen.go siteForm) writes one
  `//xo:site KEY<tab>IMPORTS<tab>LITERAL` record per non generic adapter
  at the end of the binding's Go file: KEY `fn` or `Type.fn` in Xo names,
  IMPORTS the `alias=path` pairs the literal uses, LITERAL the function
  literal with the receiver as its first parameter `h`, every package rt
  name under the placeholder `%rt.` (the backend writes its runtime
  qualifier there, "rt." or "" inside rt), and Go packages under aliases
  unique across bindings (`xgb_` plus the path; a top level standard
  package keeps its own name, the one gen.xo import_std uses). The
  literal is derived from the adapter's Go text with go/ast: the names
  declared in it (parameters, :=, var, range, nested literals' parameters)
  and Go's predeclared names stay, selectors and composite literal keys
  are not qualified, the rest is rt's. A value mode adapter (the
  GoFxValue wrapper and its goCall_ inner function) is flattened into one
  literal: the inner's statements replace the wrapper's final call and
  the call inside the replay closure (the inner is unexported in rt, so
  the program's package cannot name it: six go_* goldens failed with
  "undefined: rt.goCall_Go_time_sleep" before the closure got them).
  The literal is written over several lines, escaped in the record
  (`\n`, `\t`, `\\`): gofmt reflows a one-line literal, and then only
  its first line was under the Xo call's //line (zap recorded
  main.xo:25, the statement's gofmt line, not 19). A record that cannot
  be derived leaves an `//xo:site-error` line (TestSiteForms checks
  none on net/url and crypto/sha256). Version 33: older bindings are
  rebound.
- compiler/gen (gosite.xo) reads a binding's records once per module and
  pastes the literal at every call of a bound method
  (call.xo emit_call, std_call) and bound free function (std_call); a
  call through an interface value, a generic function, a field accessor
  and a type without a record keep the adapter call. The literal's
  imports are registered on the generated package.
- The zap workflow drops `zap.add_caller_skip(1)` and checks the exact
  line; gotcha go-adapter-frame-caller-skip.md and spec core 4.4's
  adapter frame bullet are superseded by this decision.
  Version 34 (the same day): no record for an adapter that names rt's
  unexported helpers (an implements constructor's xoNew_X: io.reader,
  http.handler) or a generic type's method (its receiver names the
  type's parameters), since the program's package cannot name either;
  33's records made the Go backend write `rt.xoNew_Go_io_Reader` and
  `rt.T`, which did not build (TestUseGoImplements,
  TestUseGoGenericTypes, 2026-10-30; gotcha
  go-site-form-names-rt-internals.md). Those calls keep the adapter.

Measured on the linux/arm64 offload host, the Go backend
(bench/results/go-site-forms-2026-10-30.md): the edit loop's rebuild
wall (caches off, GOCACHE warm, 10 alternating rounds) zap 232 to
230 ms (0.991), chi 238 to 238 ms (1.004), both within noise; the binary
the same size (6095010 and 7930018 bytes: the literals inline what the
adapters held, and the unused adapters were dead code either way); the
program's generated Go 200 to 389 lines (zap, 12 site literals) and 264
to 538 (chi, 14), the whole tree the backend hands `go build` 128419 to
130822 and 126336 to 128623 lines (about 2%); zap records main.xo:19
with no caller skip.

## Consequences

- A Go library's view of its caller is the Xo line (zap, logrus, slog,
  log's Lshortfile, testify's assertion sites); the caller skip of
  1b6771a9c is gone from the workflow.
- The generated main grows by one adapter body per bound call (the
  adapter itself stays in rt, for interface calls and generics); go
  build compiles each literal: the cost is the measurement above.
- Debuggers and panics show the literal as `main.F_main.func1` at the Xo
  line, then the Go library; a panic's Xo fault keeps the adapter's
  display name (GoExit's argument).
- Record and replay (decision 0093) is unchanged: the literal carries the
  same GoFx hook as the adapter.

## Alternatives

- A direct call, no boundary frame: impossible, GoExit's recover must be
  deferred in the calling frame, and GoEnter/GoExit track the task's
  time in Go.
- `runtime.Caller` skip from Xo (`zap.add_caller_skip(1)`, 1b6771a9c):
  per library, per call style, and wrong for libraries without a skip
  option (log.Lshortfile).
- Rewriting the adapter's frame position with a //line inside rt: one
  adapter serves every call, so no single line is right.
- Go's inliner: a function with defer is never inlined, and an inlined
  frame keeps its own position in runtime.Caller anyway.
