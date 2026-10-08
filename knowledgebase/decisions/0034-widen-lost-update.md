# 0034: XO0520 covers or_err copies and values mutated after storing

Status: Accepted (2026-10-07). Amends 0023.

## Context
The narrowed XO0520 missed a planted lost update (a local mutated after being
appended) and the checker found it also missed `.or_err(...)?` copies, which
the corpus uses. ([planted and harder](../../bench/results/planted-and-harder-2026-10-07.md), [checker corpus](../../bench/results/check-corpus-v0.3.md))

## Decision
XO0520 also applies to `var` copies initialized with `.or_err(...)?` and to a
local `var` mutated after being stored into a collection and not stored again.
