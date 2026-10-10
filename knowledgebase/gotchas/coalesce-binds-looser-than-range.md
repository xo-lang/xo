# `??` binds looser than `..` inside a slice

Status: Observed (2026-10-09, std/native/time.xo)

Symptom: `digits[0..Int.parse(arg) ?? 9]` failed to check with three
errors: "`Str` cannot be indexed", "`??` needs an optional value on the
left, found `Range[Int]`", and "expected `Int`, found `Int?`".

Cause: `..` binds tighter than `??`, so the index parsed as
`(0..Int.parse(arg)) ?? 9`: a `??` on a range, and a Str indexed by
something that is not a range.

Fix: compute the bound first (`let k = Int.parse(arg) ?? 9` then
`digits[0..k]`), or parenthesize it: `digits[0..(Int.parse(arg) ?? 9)]`.
