# 0138: `for b in bytes` yields UInt8

Status: Accepted, implemented (2026-10-10). Spec: core 3.5, std 2.2.
Code: `internal/check/stmt.go` (iterElem), `internal/gen/fn.go`
(forStmt: a counted loop over the Go string), `internal/ssa/build_stmt.go`
(forStmt: `Str.len` and an unchecked `bytes.index`, for LLVM and arm64),
`internal/check/habits.go` (Go's `for _, b := range data` rewrites to
`for b in data`). Tests: `internal/llvm/testdata/golden/bytes_for` (Go,
LLVM, and arm64 agree, no leaks), `internal/check/bytesfor_test.go`,
`internal/fix/testdata/habits/go-range-bytes`.

## Context

Codex r4 (bench/results/codex-xo-r4-2026-10-10.md) wrote
`for c in tag.bytes() { if c >= 'a' && c <= 'z' ... }` twice to validate
ASCII input and got `XO0406 cannot iterate over Bytes`. `b[i]` already
yields a `UInt8` (decision 0081), and Go, Rust (`for b in bytes`,
`.bytes()`), and Python (`for b in data`) all iterate bytes, so the
error taught a detour (`for i in 0..b.len() { let c = b[i] }`) rather
than a rule. The owner decided on 2026-10-10 to make `Bytes` iterable.

## Decision

- `for x in data` with `data: Bytes` binds each byte in order, typed
  `UInt8`. `break`, `continue`, labels, slices (`data[2..]`), and
  `bound N` work as for a List.
- No index pairs: `for` over a List does not yield `(i, x)` either (that
  is `xs.indexed()`), so `for (i, b) in data` stays an error (`XO0302`,
  the pattern does not fit `UInt8`). The spec points at
  `for i in 0..data.len()`.
- `Str` stays not iterable: a string has two reasonable element types
  (runes and bytes), and `XO0406` asks for `.runes()` or `.bytes()`.
- Backends: the Go backend emits a counted loop over the Go string that
  backs `Bytes`; the SSA reads the length with `Str.len` and each byte
  with `bytes.index` without a position (the index is in range by
  construction, so no bounds check).

## Consequences

- The habit of iterating bytes from Go, Rust, and Python is now valid
  Xo; ASCII checks against rune literals (`c >= 'a'`) already type
  check, since a rune literal adapts to `UInt8`.
- Go's `for _, b := range data` gets the fix `for b in data` (XO0177).

## Alternatives

- A note and fix on `XO0406` rewriting to an indexed loop: keeps the
  language smaller, but the rewrite is longer and the habit is common.
- `Bytes.indexed()` for `(Int, UInt8)` pairs: not asked for by any run;
  can be added later without changing this decision.
- Iterating `Str` by rune: rejected above; ambiguous between runes and
  bytes.
