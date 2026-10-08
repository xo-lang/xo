# 0026: ? on Option; leading dot continues a line

Status: Accepted (2026-10-07)

## Context
No early exit for `None` forced nested `if let` (03). The newline rule
prevented splitting method chains (05). (Paper test 2026-10-07 spec 0.2, [results](../../bench/results/paper-2026-10-07-v0.2.md))

## Decision
- In a function returning `U?`, `opt?` returns `None` early. In a `Result`
  function it is XO0204 with an `.or_err(...)` fix. `Option.and_then` added.
- A line whose first token is `.` continues the previous line.
