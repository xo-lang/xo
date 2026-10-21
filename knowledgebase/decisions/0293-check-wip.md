# 0293: `xo check --wip` for code still being written

Status: Accepted (lead, from the agents' poll with the owner, 2026-10-20)

## Context

An unused local binding or import is an error (`XO0110`, core 3.1), and
so is discarding the value of a call that only computes it (`XO0209`,
core 2.6, decision 0258). Both catch a forgotten step in finished code,
but an agent checking a function halfway through writing it (a `let`
whose use comes in the next edit, an import added before its first call)
gets errors that say nothing about whether what it has so far is right.
Real errors are harder to see among them, and an agent that silences
them with `_` names or deletes the binding loses work. Xo lang 4 asked for
a way to check while iterating; `xo explain XO0110` already points at
`xo fix`, which removes the binding instead of keeping it.

## Decision

`xo check --wip` reports `XO0110` and `XO0209` as warnings. Each gets the
note "an error outside `xo check --wip`", and the check passes (exit 0)
when nothing else is wrong. Every other diagnostic keeps its severity,
so a type error or an unresolved name still fails it. The set is
cli/core diag.xo `wip_soft`.

A plain `xo check`, `xo build`, `xo run`, `xo test`, and `xo vet` stay
strict: the codes mean what they meant (spec 10 never changes a code's
meaning), and nothing builds with them.

## Consequences

- An agent can check after each step of a larger edit and see only the
  errors that matter for the step. Before it reports done it checks
  without `--wip`.
- `--wip` composes with `--json` (the severity field reads "warning") and
  `--fix` (fixes apply first, as without it).
- The set can grow by this ADR's addendum when another code turns out to
  be mid edit only. A code that can hide a wrong program (an unused
  Result, `XO0201`; a mutation never written back, `XO0520`) stays out.

## Alternatives

- A warning by default with an error at build time: two answers for one
  program, and CI checks would pass code that does not build.
- Including `XO0201` (a Result never used): a dropped error is a bug, not
  an unfinished step.
- An environment variable: invisible in a command line and in logs.
