# 0503: Duration.seconds is Go's Duration.Seconds, to the bit

Status: Proposed (Xo Lang 2, 2026-11-04, at the lead's request; held for
the owner: it changes results in the last bit)

## Context

`d.seconds()` is the nanoseconds over 1e9 as one division on every
backend: compiler/ssa call.xo lowers `Duration.seconds` to
`IntToFloat(d) / 1e9`, and the Go backend's std/rt/time.go
`Duration_seconds` is `float64(d) / 1e9`. Go's `time.Duration.Seconds`
is `float64(d / Second) + float64(d % Second) / 1e9`: whole seconds plus
the rest. The two round differently in the last bit for some durations
past one second (a Go program over a million random durations under
1000 h: 2 differ, e.g. 24330575993982 ns is 24330.575993981998 in Go and
24330.575993982 as one division).

Std follows Go's standard library and ports Go's code (decision 0276;
CLAUDE.md "port Go's code ... where it fits"), and its ports are checked
against Go's output to the bit. A port that calls `seconds()` where Go
calls `Seconds()` can then differ from Go in the last bit. It happened:
the 2026-11-04 random differential of std/rate against
golang.org/x/time/rate found tokens off in the last bit (3 of about
2,000 token reads), fixed in 24f2d90ae by a private `go_seconds` in
std/native/rate.xo. std/rate's `every` (`1.0 / interval.seconds()`, Go's
`1 / Limit(interval.Seconds())`) and std/metrics' timing
(`clock.since(t0).seconds()`, client_golang's `time.Since(t).Seconds()`)
have the same exposure.

## Decision

`Duration.seconds()` returns Go's value: `Float(d / 1s) + Float(d % 1s) /
1e9` with Int division and remainder, on every backend.

- compiler/ssa call.xo: the `Duration.seconds` lowering becomes the
  quotient, the remainder, two IntToFloat, a division by 1e9 and an add
  (two integer ops and a floating add more than today; no call).
- std/rt/time.go `Duration_seconds`: `time.Duration(d).Seconds()`.
- WASM lowers it the same way (decision rule: a new rt intrinsic or a
  changed lowering also lands in wasm rt2.xo, with its goldens).
- spec/std.md's `seconds()` line says it is Go's `Duration.Seconds`.
- std/native/rate.xo's `go_seconds` is removed for `seconds()`.

## Consequences

- Results change in the last bit for a few durations over one second: a
  program printing `seconds()` with full precision, or comparing it
  exactly, can see a different last digit. CHANGELOG marks it as a
  changed result (not Breaking: no program that compares floats with a
  tolerance changes), std/changes/prelude.md records it.
- Every port that calls `seconds()` agrees with Go's code to the bit
  without a private helper.
- A few more instructions per call (`seconds()` is not on hot paths in
  std: rate's tokens, metrics' observe, timing prints).
- Test: a golden printing `seconds()` of the durations that differ
  (24330575993982 ns and Go's other cases from a sweep) equal to Go's
  text, on arm64, amd64, LLVM, wasm and the Go backend.

## Alternatives

- Keep one division and fix each port privately (rate's `go_seconds`):
  every future port has to know the trap; the random differential found
  it only because it compared to the bit.
- A second method (`seconds_go()`): two answers for one question.
