# 0012: Full pattern syntax; unused rule relaxed

Status: Accepted (2026-10-07)

## Context
All 5 agents used `_` though the spec never defined it. Unused closure
parameters forced workarounds. Literal and or patterns were missing. (Paper test 2026-10-07, [results](../../bench/results/paper-2026-10-07.md))

## Decision
- Patterns: `_`, literals, inclusive ranges, `|`, tuples, variants (positional or named, `..`), structs, list shapes, nesting.
- XO0110 does not cover parameters, `_`, or names starting with `_`. `let _ = expr` always allowed.
- A match arm may be a single statement (assignment).

## Consequences
- More parser and exhaustiveness work in P1.
