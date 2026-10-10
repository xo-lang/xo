# An or pattern's later alternatives bound nothing natively

Status: Observed (2026-10-09, while adding native list patterns)

Symptom: on the native backends (`--no-gc`, LLVM and arm64), `match t {
Leaf(v) | Node(left: v, ..) => ... }` printed the value `v` had in an
earlier loop iteration (or garbage) when the second alternative matched;
the Go backend printed the right value. The `patterns` golden of
internal/gen showed it (`v=5 v=5` instead of `v=5 v=1`) as soon as its
list pattern stopped being XO1201.

Cause: the checker gives each alternative of an or pattern bindings of
its own (`orPattern` in internal/check/pattern.go pushes a scope per
alternative), and the arm's body resolves `v` to the first alternative's
`VarObj`. The SSA builder wrote the second alternative's binding to a
different variable, so the join read the first one's definition along a
path where it was never written (the SSA renamer then found an older
definition through the loop's block parameters).

Fix: `pattern` in `internal/ssa/build_expr.go` copies each later
alternative's bindings into the first alternative's by name
(`patBindings`) before jumping to the arm. `check.Inspect` does not walk
patterns (children include patterns only where they hold expressions),
so `patBindings` walks them itself.
