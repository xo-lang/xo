# 0130: Self-hosting is a goal

Status: Accepted (2026-10-10, by the owner)

## Context

The lexer is written in Xo (decision 0076) and a parser in Xo is under
way (decisions 0127 to 0129). Writing the compiler in Xo finds language
gaps (0078 to 0081 came from the lexer), cross-checks the Go compiler,
and gives a large realistic benchmark; carried to the end it removes the
Go toolchain from Xo's own build and lets the compiler ship as one native
binary.

## Decision

Self-hosting is a project goal: after the parser, the type checker, then
the remaining compiler stages are rewritten in Xo, each one checked
against the Go implementation with identical output on every program in
the repo before the next starts. The Go implementation stays the
reference until the Xo compiler can build itself (a fixed point) and
passes the full suite on the backends it targets.

## Consequences

- Language gaps found by the rewrite are fixed in the language, not
  worked around, unless the fix is large (then recorded and scheduled).
- Two implementations exist for a long time; differential tests keep
  them identical, and a change to one is mirrored in the other in the
  same change once a stage is self-hosted.
- The order of the remaining stages and the switch-over point get their
  own decisions when reached.

## Progress

- 2026-10-08: lexer (decision 0076), identical token dumps.
- 2026-10-10: parser (decision 0127), identical tree dumps on every
  accepted file, three backends.
- 2026-10-10: checker stage 1 of 8, names (decision 0139):
  `selfhost/check` resolves every identifier like internal/check on all
  347 accepted repository runs and gives the same diagnostics on the 56
  rejected only for names, three backends, plus mutations; found three
  compiler bugs, fixed (decision 0140 among them). Next: stage 2, types
  and inference (bench/results/selfhost-checker-2026-10-10.md).

## Alternatives

- Stop at the parser (differential testing and gap finding only).
- Stay on the Go compiler indefinitely.
