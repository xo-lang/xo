# A struct field default cannot name a constant natively, and a match arm cannot be one

Status: Observed (2026-10-29, the html/template port, decision 0408)

## Symptom

`struct Context { state: Int = STATE_TEXT, ... }` with `const STATE_TEXT
= 0` checks, but the native backends reject it: `XO1201: not available on
the native backends (arm64, amd64, LLVM) yet: the name STATE_TEXT here
(support is planned)`, once per field. And `match st { STATE_TEXT =>
..., STATE_TAG => ... }` over an Int is not a comparison: an upper case
identifier in a pattern binds the value like any name, so the first arm
always matches (the checker reports nothing; the port's tests would have).

## Cause

A field default is evaluated where the struct literal is built, and the
native lowering of defaults handles literals only (the Go backend
evaluates the expression). Patterns name variants, literals, bindings and
`_`; a constant is a binding.

## Fix

Write the literal in the default and name the constants in a comment
(`state: Int = 0 // STATE_TEXT`), or build the value with a function.
Compare constants with an if chain (`if st == STATE_TEXT {...} else if
...`) or match on literals (`match c { 38 => ... }`, which works).
