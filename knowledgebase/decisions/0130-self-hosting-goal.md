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
- 2026-10-10: checker stage 1 of 8, names (decision 0141):
  `selfhost/check` resolves every identifier like internal/check on all
  347 accepted repository runs and gives the same diagnostics on the 56
  rejected only for names, three backends, plus mutations; found three
  compiler bugs, fixed (decision 0140 among them).
- 2026-10-10: checker stage 2 of 8, types and inference (decision
  0141): identical names and types sections on all 348 accepted
  repository runs, the same diagnostics on the 76 rejected only for names
  and types, three backends, plus name and type mutations; found a
  native backend bug (decision 0142) and a parser trap (decision 0143),
  fixed. The Xo checker (LLVM build) takes 1.2 times the Go checker's
  full check.
- 2026-10-10: checker stages 3 (generics) and 4 (effects and real-time)
  of 8 (decision 0141): the sigs section (type parameters, constraints,
  implicit parameters, declared and inferred effects) identical on all
  349 accepted repository runs, XO0402, comparability (XO0404), XO0701,
  and XO1401 to XO1405 the same on the 91 runs rejected only for stages
  1 to 4, three backends, plus generics and effects mutations; no
  compiler bug found. Effect ceilings of dependencies (XO1309) are not
  ported yet. The LLVM build takes 1.28 times the Go checker on the
  selfhost input and 0.97 times over all repository runs. Next: stage 5,
  value semantics (bench/results/selfhost-checker-2026-10-10.md).
- 2026-10-10: the parser's habits of other languages (decision 0087)
  with their recovery, pulled ahead of stage 8, and checker stages 5
  (value semantics: XO0501, XO0502, XO0503, XO0510, XO0520, XO0804,
  unused bindings) and 6 (exhaustiveness: XO0301, XO0303, XO0305) of 8
  (decision 0141): 580 of 608 repository runs compared and identical
  (350 accepted, 230 rejected with the same diagnostics), three
  backends, plus habit, value semantics, and pattern mutations; one
  self hosted parser bug fixed (diagnostic quoting). The LLVM build
  takes 1.9 times the Go checker on the selfhost input and 1.34 times
  over all repository runs. Next: stage 7, notes and fixes.
- 2026-10-10: checker stages 7 (notes and fixes, the full JSON section)
  and 8 (naming, time layouts, the checker's habits) of 8, and the Xo
  parser's resynchronization after hard errors (decisions 0141, 0127):
  587 of 609 repository runs compared whole and identical on the three
  backends. arm64 spill slot reuse (decision 0146) and LLVM struct
  parameters by reference (decision 0145) came out of it.
- 2026-10-10: module loading in the self hosted checker (decision 0141):
  xo.mod, xo.work, xo.lock, the module cache, vendor/, deny rules
  (XO1312), dependency rules (XO1301 to XO1307), and effect ceilings
  (XO1309); every repository run except the 16 `use go`/`use c` ones
  (610 of 626) is compared and identical on the three backends, plus
  module mutations. The checker is complete for programs without Go or
  C interop.

## Alternatives

- Stop at the parser (differential testing and gap finding only).
- Stay on the Go compiler indefinitely.
