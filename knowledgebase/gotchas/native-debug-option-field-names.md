# Native Debug text printed Option and Result with field names

Status: Observed (2026-10-09, decision 0083)

Symptom: the first native run of an `expect(m.get("b")) == Some(2)`
failure printed `right: Some(v: 2)`; the Go backend prints `Some(2)`.

Cause: the generated Debug functions (internal/ssa/build_show.go) print a
variant's named fields as `name: value`, which is right for user enums
(`Bad(msg: "x")`) but std/rt's option.go writes the prelude's `Some`,
`Ok`, and `Err` without the field name (`v`, `e`). No golden printed the
Debug text of an Option before tests ran natively.

Fix: build_show omits field names for prelude Option and Result. Expect
failure messages compare left and right as Debug text, so the native
test reports (cmd/xo TestNoGCTests) now cover Debug text against the Go
backend.
