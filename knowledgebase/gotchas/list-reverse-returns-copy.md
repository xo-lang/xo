# List.reverse returns a reversed copy; a discarded result is silent

Status: Observed (Xo Lang 2, 2026-10-17, compiler/arm64 fieldlicm.xo).

## Symptom

A compiler pass built a list backward and called `chain.reverse()` to put
it in order. The list stayed backward, and the strict self build failed
far from the cause: `advance: b54: v186 uses v186, which does not
dominate it`. The register promotion had read the versions of a list in
reverse, so a field read was replaced by the value computed from it.

## Cause

`fn (xs: List[T]) reverse() -> List[T]` (spec/std.md) returns a new
list; it does not reverse in place. The checker does not warn about a
discarded result of a method like this, so `xs.reverse()` as a statement
compiles and does nothing.

## Fix

Write `xs = xs.reverse()`. Since decision 0258 (2026-10-17) the checker
reports the bare statement as error XO0209, with that fix.
