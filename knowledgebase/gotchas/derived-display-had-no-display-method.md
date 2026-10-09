# `e.display()` was rejected on derived and std error types

Status: Observed (2026-10-09, Codex r2 rerun, task 03)

Symptom: XO0601 "`http.ClientErr` has no field or method `display`",
although std.md says every std error type implements `Display`. The same
happened for every user type with `derive Display` and for builtins such
as `Int`.

Cause: `derive Display` made the type satisfy `Display` and `Error`
(interpolation and `?` conversion worked) but did not add a `display`
method that method lookup could find; only hand written `display`
methods (`DurationErr`, `go.Error`) were callable.

Fix: the checker types `x.display()` as `Str` for any value that
implements `Display` without a declared method, and the Go backend
emits the same text as `"${x}"`. Every std error type is covered by
`internal/gen/testdata/golden/std_error_display`. The SSA native backend
does not lower this call yet (like `min`, `max`, `clamp`).
