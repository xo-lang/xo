# A proof that closes a cycle of block parameters must not cross an add

Status: Observed (2026-11-20, task #457, caught before a push)

Symptom: the first `small` (fieldlicm.xo: "this value is at most a length
plus little, so adding a constant cannot wrap") dropped the overflow
check of `k += 4` in a loop with no bound on k at all.

Cause: the walk over a loop header's parameter treats "the parameter met
again" as proven, which is right for a cycle of plain copies (the value
from outside decides). The rule for `x + k` then recursed into x, met the
header's parameter and called the whole counter small: each turn adds k,
so the value is not the one the walk started with.

Fix: the walk marks that it crossed an add (-1 in its seen list), and a
parameter met again then proves nothing. The latch's value has to be
bounded by a branch fact itself (`m + 4 <= lim`, or a sibling sum of the
same value that a branch bounded). test/codegen/matchbounds `count_up`
keeps its check. Any new inductive rule in Proof (nonneg, at_most,
below_len, small) needs a case like it: a loop whose counter grows with
nothing above it.

## The same work: facts copied per join made the compiler quadratic

Status: Observed (2026-11-20, task #457, caught before a push)

Symptom: xo did not finish compiling itself in twenty minutes with the
first `join_facts` (one core at 100% in `xo build`; the lead's spin and
hang alerts 86707 and 86708 on a cgab run). `--phase-times` never
printed the inline phase.

Cause: to give the taken side of an `a && b` join the facts of b's
block, it copied each such fact under the new block: every join walked
every fact, with a walk up the dominator tree for each. One function of
1500 `a && b` conditions took 92.7 s (5.35 s with main's compiler).

Fix: nothing is copied. The facts have their own dominator tree
(Proof.fidom), in which the taken side hangs under the block its value
came from; 5.43 s for that function, and xo's own inline phase 3.30 s
against 3.08 s. internal/arm64 TestJoinFactsBudget holds the budget.
Two rules from it: a new fact source must cost per branch, never per
branch times facts; and a WIP compiler that is slow poisons
`scripts/xobuild`, which bootstraps from the newest earlier build in
`<user cache>/xo/checkout` (delete that build's directory, found by its
`commit` file, before building again).
