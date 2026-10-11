# 0149: Enum parameters that some path only reads are borrowed

Status: Accepted (2026-10-10). Implemented in `internal/ssa/rc.go`
(`inferBorrows`, `consumedOnEveryPath`). Tests: SSA golden
`internal/ssa/testdata/golden/borrow_enum` (`walk` borrows its node,
`keep` and the List parameter of `add` stay owned) and the same program
as LLVM golden `internal/llvm/testdata/golden/borrow_enum` (no leak).
Evidence: `bench/results/selfhost-checker-2026-10-10.md` (LLVM
experiments). Refines the borrow inference of decision 0046.

## Context

Borrow inference (decision 0046, rc.go) marked a heap parameter borrowed
only when no path of its function consumes it. A recursive walk over a
tree (the self hosted checker's `check`, `infer`, `check0` over `Expr`
and `Stmt`) consumes its node on a few paths (storing it in a table,
wrapping it in a result), so the node parameter of the whole recursive
group became owned: every level retained the sub node it projected out
of its own (`dup` of a 352 byte `Expr`, a switch over its variants) and
released its own node (`drop`) when done. After decisions 0147 and 0148
those were the largest entries of the LLVM build's profile.

## Decision

- An enum parameter is borrowed unless every path from the entry to a
  return consumes it (landing pads are not paths; a loop with no exit
  is none). The paths that consume a borrowed parameter retain it there
  (the RC pass already does this for borrowed projections).
- Every other parameter keeps the rule of decision 0046 (owned once any
  path consumes it). Lists and strings are updated in place only when
  their count is one; a borrowed one that a path consumes is retained
  and then copied at every update. The first version of this decision,
  for all types, made the checker 25 times slower: `xo.map.put`'s map
  parameter, consumed on all but one path, became borrowed, and every
  `put` copied the map's lists.
- Enums are never updated in place, and their payloads are projections
  that are retained when consumed whether the enum is owned or borrowed,
  so borrowing one copies nothing.

## Consequences

- The LLVM build of the self hosted checker is about 7% faster (20 runs
  of the selfhost input: 3,163 to 2,934 ms, interleaved), at parity with
  the Go checker; the arm64 build gains too (the SSA is shared); output
  unchanged on every repository run on the three backends.
- A function that consumes an enum on a rare path pays a retain there
  and its callers a release after the call (an owned parameter moved at
  its last use cost nothing); the walks that read most nodes gain more
  than that.
- A borrowed root makes the box contents and list elements under it
  stable memory for decision 0147, so more sub nodes are passed by
  address.

## Alternatives

- **Every type** (the first version): quadratic copies of lists and
  maps updated through a borrowed parameter.
- **A cost model per call site** (count the consuming and reading paths
  weighted by loops): more precise, but the simple rule captures the
  tree walks and cannot make an in place update copy.
- **Borrow per path** (a parameter borrowed on reading paths and owned
  on consuming ones, as Lean's borrow inference does with specialized
  copies of the function): duplicates code.
