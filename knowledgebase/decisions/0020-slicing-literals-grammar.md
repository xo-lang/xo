# 0020: Slicing, literal and grammar clarifications

Status: Accepted (2026-10-07)

## Context
Smaller gaps from the paper test: no slicing, no empty Set literal, `{}`
ambiguity, no closure literal grammar, trailing commas, const struct literals,
`else` placement, per file or per module `use`. (Paper test 2026-10-07, [results](../../bench/results/paper-2026-10-07.md))

## Decision
- Range slicing `xs[a..b]`, `xs[1..]`, `xs[..n]` on List, Bytes, Str (byte offsets).
- `#{}` empty Set; `{}` is always an empty Map; an empty block value is `()`.
- Closure grammar with optional types; trailing commas everywhere.
- `const` accepts compile time struct, enum, and collection values.
- `use` is per file.
