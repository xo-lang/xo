# 0623: Shared release tails for nested error exits

Status: Proposed (Xo Lang 3, 2026-11-12; internal codegen, no semantics
change; the lead asked for the prototype and the numbers first).

## Context

aws-43's gcp models (decision 0013 there) grew bigquery's sample from
4.44 to 5.43 MB of __text. With every derived decoder kept out of the
inliner (XO_INLINE_DENY='xo.json.dec*') the growth stays (-28 KB of
996 KB), so inlining is not the cause. Each field of a derived Json
decoder has two error returns (a duplicate key, the member itself), and
the reference counting pass puts, on each, a release of every field
value decoded before it: O(n^2) releases. A 60 field struct of Str
fields has a 65 KB decoder; the releases between fields grow 0, 0, 1,
1, ..., 14, 14 in its disassembly. Notes:
~/xo-lead/notes/lang3/jsondec-size-2026-11-12.md.

Fixes in the JSON generator alone do not work: a struct accumulator is
flattened (every error releases all n slots); an error ladder of
blocks loses its carried values (unused block parameters are removed
before the RC pass, so the releases stay on the edges).

## Decision (proposed)

A pass after the RC pass (compiler/ssa/reltail.xo) finds exits that
release nested sets of values (each exit's set is the previous exit's
set plus some values) and rewrites them as a ladder: an exit jumps to a
shared tail that releases its own values and falls into the tail of the
smaller set. Each value is released in one place. The values dominate
their tails (a tail is reached only from exits after the value's
definition), so the result is valid SSA.

## Results

To be filled: __text of the gcp calendar and bigquery samples, an aws
sample and xo; callgrind and one --ab on jsontoken decode and on xo's
self build; the fixed point.

## Consequences

To be filled.

## Alternatives

- Generator-only fixes (above): no effect.
- A heap scratch object for the decoded values: one allocation per
  decode.
- A runtime helper that releases a list of slots: still O(n^2) data.
