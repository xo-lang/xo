# 0587: Integer bounds as type constants: `Int.max`, `Int.min`, `UInt8.max`, ...

Status: Accepted (owner, 2026-11-10, as written, relayed by the lead;
proposed by Xo Lang 1 from owner tester feedback). The first case of `Int.min`
gave three errors; std.md 1 called type constants (`Float.PI`, `Float.INF`)
"a later step", so this is a language change.

## Context

A program that needs the largest Int writes `9223372036854775807` (std
has 72 such uses outside tests, plus `-9223372036854775807 - 1` for the
smallest), or reaches for `Int.max` / `Int.min` as in Go's
`math.MaxInt64`, Rust's `i64::MAX`, Java's `Long.MAX_VALUE`. Today
`Int.min` is not a value: the checker tries the Ord method `min` and the
errors cascade (one prints the unknown type as `_`).

## Decision (proposed)

1. Every fixed-width integer type has two type constants, written without
   parentheses: `Int.max`, `Int.min`, `Int8.max` ... `Int64.min`,
   `UInt8.max` ... `UInt64.max`, and `.min` (0) for the unsigned types.
   Each has its own type (`UInt8.max` is a `UInt8`) and is a constant
   expression (usable in `const`, patterns, ranges).
2. They are declared in the prelude as type constants, a new prelude
   declaration form `const (static: Int) max: Int = 9223372036854775807`
   (spec/std.md 1, `xo doc Int` lists them), resolved by the checker on a
   selector of a type name; no user syntax for type constants in this
   decision (a later ADR may open it, with `Float.PI` / `Float.INF`).
3. `x.min(y)` and `x.max(y)` (the Ord methods) are unchanged; `Int.min(a,
   b)` (calling the method through the type) is XO0601 with the note
   naming `a.min(b)` and `Int.min` the constant.
4. std replaces its literal bounds with the constants (std/native rate,
   big, protojson, amqp, ... 72 uses), with no behavior change; a std
   change log entry per module touched (0276).

## Consequences

- `Int.min` and `Int.max` read as in the other languages; `-Int.min`
  still faults (overflow), and the XO-F text names it.
- A new prelude declaration form the checker, xo doc and the SSA
  constant folder learn; the backends see plain constants.

## Alternatives

- **Static functions `Int.max()`**: no new form, but `Int.max` without
  parentheses (what the tester wrote) stays an error.
- **Module constants `math.MAX_INT`**: Go's style, but a second place to
  look and no per-type form.
- **Only a habit note** naming the literal: leaves 72 magic numbers in std.

## Implementation (Xo Lang 1, 2026-11-10)

Point 2's form is a doc marker, not new syntax: the prelude declares
`/// The largest Int. @static Int.max` above `const INT_MAX: Int = ...`
(std/prelude.xo, Numbers), and the checker attaches a std constant so
documented to the type's statics and leaves its own name out of scope
(compiler/check/decl.xo attach_static_const; core 4.6 lists the marker).
The parser, the formatter, the AST caches and every backend are
unchanged: a reference is an ordinary std constant to SSA and the Go
backend (std/rt/intbounds.go). `Int.max(a, b)` is XO0407 (a constant,
not a function) with the note naming `a.max(b)`, where point 3 said
XO0601: XO0407 is what the existing type constants (`Time.UNIX_EPOCH`)
give. Golden int_bounds; errorcheck int_max_call.

Point 4 (std's literal bounds) follows the first seed whose checker
knows `@static`: xo builds itself with the previous release's xo, whose
checker reads `Int.max` in std/native/llrt or a std module the compiler
imports as an unknown name (gotchas/seed-older-than-intrinsics.md). The
prelude's new constants are harmless to an older checker (ordinary
prelude constants to it), which the gate's seedbuild step checks.
