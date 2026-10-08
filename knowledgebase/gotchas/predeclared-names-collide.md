# Predeclared names take common variable names

Status: Observed (2026-10-07, paper test tasks 02 and 03). Resolved by decisions 0013 and 0017.

`err`, `ok`, `result` are predeclared and cannot be shadowed, so the Go habit
of naming a variable `err` or `result` fails. `use std/test` also binds the
keyword `test`. Proposed fix in bench/results/paper-2026-10-07.md (rename to
`Ok Err Some None`, make `result` contextual, rename `std/test`).
