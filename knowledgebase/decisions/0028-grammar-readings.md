# 0028: Grammar readings from the P1 parser

Status: Accepted (2026-10-07)

## Context
Building `internal/parser` exposed undefined nonterminals and places the
spec 0.3 grammar did not settle (parser agent report, `internal/parser/spec_test.go`).

## Decision
Written into core 1.3, 1.6, and 15:
- Defined `ImportPath`, `FnSig`, `Invariant`, `Derive`, `SelectArm`, `Literal`, `TypeArg`.
- Clauses, `requires ... else`, and the body `{` may start on following lines.
- `T?` after any type; qualified type names; `var` in function type parameters; `..` anywhere once in list patterns; `(p)` and `()` patterns.
- Type arguments vs. index heuristic; `{` map vs. block rule; return position function types take their own `uses`.
- Durations are integer literals; escapes listed; integer literals never become `Float`.
- Local `const` allowed (fixes the 3.1 example); section 7.5 moved after 7.4.

## Consequences
The parser is the reference implementation of the grammar; spec examples are parsed in CI.
