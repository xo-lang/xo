# 0030: `is` pattern test and small syntax

Status: Accepted (2026-10-07)

## Context
Agents needed a boolean pattern test for assertions (03), compound assignments
beyond `+=` (05), tuple field access (02), qualified types in `from` (03),
doc comments on fields (03). ([planted and harder](../../bench/results/planted-and-harder-2026-10-07.md))

## Decision
- `x is Pattern` (contextual, precedence of `==`), no bindings.
- `+= -= *= /= %=`; `t.0`; qualified `from` types; doc comments on fields and variants.
