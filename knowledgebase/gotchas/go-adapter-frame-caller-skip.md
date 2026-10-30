# A Go library that records its caller names Xo's binding adapter

Status: Superseded by decision 0441 (2026-10-30): the Go backend pastes
the adapter's body at the call under the Xo call's `//line`, so zap logs
`"caller":"main.xo:LINE"` with no caller skip; a call through an
interface value or of a generic function still runs the adapter in rt.
Kept for the history and the frame analysis. Observed 2026-10-30 (the
owner's Windows testing of zap on the Go backend).

## Symptom

zap's production logger (AddCaller on) logs
`"caller":"rt/xo_gobind_go_uber_org_zap.go:1687"` for a call made from
main.xo, not the Xo line.

## Cause

Every `use go` call runs through the binding's adapter (`M_info` and the
like, generated into the std/rt package: the argument and result
conversions and the panic boundary). zap reads `runtime.Caller(2)` from
`Logger.check`: frame 2 is Info's caller, which is the adapter. The Xo call
site is one frame further and its generated Go line carries `//line
main.xo:N` (so it is not a missing `//line`).

## Fix

Tell the library to skip one more frame: `cfg.build([zap.add_caller_skip(1)])`
logs `"caller":"main.xo:19"` (bench/usego-workflows/zap checks it; spec
core.md 4.4). The same holds for any library that walks the stack to its
caller (logrus ReportCaller, slog handlers that take a source: each has
its own skip option; check its frames before claiming the count).
