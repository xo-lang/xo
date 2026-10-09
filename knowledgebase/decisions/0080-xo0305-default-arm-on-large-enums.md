# 0080: XO0305 does not report a `_` arm that is clearly a default

Status: Accepted (2026-10-08). Implemented in `internal/check/pattern.go`
(`warnWildcardOwnEnum`, `defaultArm`). Gap 5 of
`bench/results/selfhost-lexer-2026-10-08.md`. Refines decision 0038.

## Context
Warning `XO0305` (decision 0038, core 3.5) reports an unguarded `_` arm
that hides variants of an enum declared in the same module, so that adding
a variant is a compile error at every `match`. On the self hosted lexer's
token `Kind` enum (88 variants) every "these 8 kinds are special, the rest
are not" match warned and asked to list the other 79, all on one line. The
port worked around it with `is` or patterns and lookup lists, which are
longer and hide the same risk.

## Decision
- **Not reported when the arm is a default:** the `_` arm hides at least 8
  variants (`wildcardDefaultMin`) and more than half of the enum. Listing
  that many is the noise the warning is meant to prevent, and a reader
  sees a default; a new variant falling into it is what the author asked
  for.
- **Still reported otherwise**, including a `_` hiding most of a small
  enum (2 of 3, or 7 of 9): listing those is cheap, and the case the
  warning exists for (a `match` that handles nearly every variant and
  sweeps the last few into `_`) keeps its warning however large the enum.
- **The message names at most 8 hidden variants**, then "and N more".
- XO0305 keeps its meaning (a `_` arm hiding variants of a local enum); only
  when it is reported changes. No new code, no new syntax.

## Consequences
- `selfhost/lexer` writes `match k { Ident => ..., _ => ... }` on `Kind`.
- A default arm over a large local enum is silent: adding a variant that
  needs its own handling in such a `match` is not caught. That is the
  trade made here, and it is the same as for enums of other modules.

## Alternatives
- **Only cap the message**: still a warning on every default arm, so
  `xo check` is never clean on such code.
- **A per match opt out** (an attribute or a distinct default arm syntax):
  new syntax for one warning; the rule above needs no annotation.
- **A fraction only (more than half hidden)**: would also silence `_`
  hiding 2 of 3 variants, the most common small enum case the warning
  catches today.
- **An absolute count only (8 or more hidden)**: would silence a `_`
  that hides 8 of 100 variants, where the author handled 92 and very
  likely forgot some.
