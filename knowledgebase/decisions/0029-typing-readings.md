# 0029: Typing readings from the P1 checker

Status: Accepted (2026-10-07)

## Context
The checker had to choose readings where spec 0.3 was silent; the v0.3 corpus
depended on 11 of them ([checker corpus](../../bench/results/check-corpus-v0.3.md)).

## Decision
- An unused `Result` binding is XO0201 whatever its name; `let _ =` handling changed in 0031.
- A closure ending in a `Result` is failing; a closure declared `-> T` without `!` cannot fail.
- No implicit conversions in branch joins or `requires ... else`; `opt == v` wraps in `Some`.
- Mutating a captured `var` is XO0502. Creating a closure performs no effects.
- Capture by any closure moves a move only value; passing to a `var` parameter does not.
- Structs may take `fx: Effects`; effect sets are valid type arguments (extended in 0033).
- `Never` satisfies every interface.
- A task error is consumed only if its handle is bound with `let` and awaited.
- `Str` is not indexable by position; `m[k] = v` on a var map is `put`.
- `derive Json` does not need `use std/json` (Appendix A fixed).
- Changed from the checker's reading: `n * d` is allowed as well as `d * n`.
