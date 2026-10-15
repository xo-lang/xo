# Predeclared names take common variable names

Status: Observed (2026-10-07, paper test tasks 02 and 03). Resolved by decisions 0013 and 0017.

`err`, `ok`, `result` are predeclared and cannot be shadowed, so the Go habit
of naming a variable `err` or `result` fails. `use std/test` also binds the
keyword `test`. Proposed fix in bench/results/paper-2026-10-07.md (rename to
`Ok Err Some None`, make `result` contextual, rename `std/test`).

Observed again 2026-10-15 (B31, runtime modules in std/native/llrt): the
type names `Str`, `Time`, `Err` and `Result` are predeclared too, so a
module's own `struct Str` (a DER window, cbyte.xo), `struct Time` or
`struct Err` fails with XO0603 ("is predeclared and cannot be redefined at
top level") followed by XO0605 and XO0601 errors at every use. Name such
structs for what they are (`Win`, `TimeRes`, `LevelErr`).
