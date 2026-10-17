# `uses` is a keyword

Status: Observed (2026-10-13, decision 0201).

**Symptom.** A local named `uses` in compiler/arm64 (mirroring the Go
emitter's `uses` slice) gave `XO0160 expected a variable name, found
keyword uses`, then `XO0110 d is never used` for the line before.

**Cause.** `uses` introduces effect clauses (`fn f() uses fx`), so it is
reserved like `when`.

**Fix.** Name it `us` (as compiler/arm64's `plain` does).
