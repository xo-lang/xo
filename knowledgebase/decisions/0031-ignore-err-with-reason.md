# 0031: Discarding an error needs a reason

Status: Accepted (2026-10-07)

## Context
In the planted bug benchmark, 4 of 5 ignored error bugs escaped every Go tool,
and all 4 were explicit `_` discards. Spec 0.3 allowed the same silently with
`let _ =`, so Xo prevented 0 of 5 ([planted and harder](../../bench/results/planted-and-harder-2026-10-07.md)).

## Decision
`let _ = <Result>` is XO0206. The only discard is
`r.ignore_err("reason")`, which yields `T?` and logs the reason at debug level.

## Consequences
Discards become searchable and reviewable; an agent has to state why. This
does not stop a determined wrong discard, only makes it visible.
