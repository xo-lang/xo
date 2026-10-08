# Empty braces in expression position are a map, not a block

Status: Anticipated (2026-10-08, `internal/format`)

Symptom: a formatter or code generator that prints an empty block as `{}`
in a match arm body or as an expression statement changes the program:
`Done => {}` is an arm whose value is an empty Map.

Cause: core 1.6 makes `{}` the empty map; `braceIsMap` in the parser
also treats `{` newline `}` as a map. Only bodies (functions, closures,
`if`, loops, `defer`, `scope`) parse `{}` as an empty block.

Fix: write `()` for a unit arm body (`Done => ()`). `xo fmt` prints the
tree it parsed, so it never turns one into the other; tools that build
Xo source must do the same.
