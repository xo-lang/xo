# 0220: Per-module parallel compilation with dependency summaries

Status: Proposed (stub, 2026-10-13); being written by the fast-builds
agent; the owner decides before anything is implemented.

Xo builds a program as one whole-program compile, mostly on one core
(the arm64 self build of xo: 337 s on the Mac, emit 179 s). Go compiles
each package against a summary of its dependencies, in parallel, and
caches per package. To be filled in: the design, its cost to
monomorphization, cross-module inlining, and the arena and escape
analyses, and the alternatives.
