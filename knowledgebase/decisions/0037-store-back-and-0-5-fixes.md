# 0037: Store back required for mutated copies; 0.5 fixes

Status: Accepted (2026-10-07). Amends 0034.

## Context
Stage 2 probes showed XO0520 missed a lost update when the stale copy was read
or returned after the change (the same bug as planted Go variant 01 X6). The
mutation probe found 5 real misses among 31 removed store backs. Subjects also
hit two checker bugs and two spec contradictions
(`bench/results/stage2-xo-check-2026-10-07.md`).

## Decision
- XO0520: a mutated copy out of a collection must be stored again after its
  last mutation (whole value passed to a call or assigned). Reads and returns
  do not count. Scratch copies go through a local. `with` derivations are not
  tracked, to avoid flagging views such as redacted copies.
- `is` combines with `&&`/`||`; tail `if`/`match` arms are in return position.
- Checker fixes: `testing` in `_test.xo` helper signatures; no XO0405 cascade
  for closures passed to a callee that already failed to type.

## Consequences
Mutation probe after the change: the read after case is caught; the remaining
copy out misses are 4 `with` derivations (by design). Results in
`bench/xo/v0.4/mutations.jsonl` (before: `mutations-checker-0.4.jsonl`).
