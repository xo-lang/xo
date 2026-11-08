# Empty braces in expression position are a map, not a block

Status: Observed 2026-11-08 (Anticipated 2026-10-08, `internal/format`).
Hit by hand in compiler/cli/modules sumserve.xo: a `match` ending a
closure passed to `Mutex.with`, with `Ok(_) => {}` and an `Err(e) =>
eprintln(...)` arm, failed XO0401 "expected `Map[_, _]`, found `()`" at
the `Err` arm, since the closure's last expression is its value and the
first arm made it a Map. In statement position (not a body's last
expression) the same arm is harmless, which is why the pattern is common
in the code. Written as `if let Err(e) = ... { ... }` instead.

Symptom: a formatter or code generator that prints an empty block as `{}`
in a match arm body or as an expression statement changes the program:
`Done => {}` is an arm whose value is an empty Map.

Cause: core 1.6 makes `{}` the empty map; `braceIsMap` in the parser
also treats `{` newline `}` as a map. Only bodies (functions, closures,
`if`, loops, `defer`, `scope`) parse `{}` as an empty block.

Fix: write `()` for a unit arm body (`Done => ()`). `xo fmt` prints the
tree it parsed, so it never turns one into the other; tools that build
Xo source must do the same.
