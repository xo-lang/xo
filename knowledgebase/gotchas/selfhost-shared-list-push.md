# Xo: pushing to a copy of a list another copy already extended copies the whole list

Status: Observed (2026-10-11, the go build of compiler/ssa)

Symptom: a CPU profile of the go build of the self hosted SSA builder
(`XO_CPUPROFILE`, `--bench=20 compiler/ssa`) showed 9% of the run in
`rt.List_copyOut` under `List_push`. Counting the copies (a temporary
counter in std/rt list.go, not committed) gave 177,674 pushes that
copied a list whose capacity was not exhausted, 1.7 million elements in
all, almost all from `Builder.comparable`.

Cause: `comparable(t, seen)` did `var s2 = seen; s2.push(t)` and passed
s2 to the recursive calls for each field. Every sibling call pushes onto
its own copy of the same prefix; the first push claims the slot after
the prefix, so every later sibling's push finds the slot taken and
copies the whole list (std/rt List_push: in place only when the header
ends at the block's claimed length). The path copy also revisited every
shared field type, where the Go builder's `seen` map visits each type
once.

Fix: one `seen` list threaded through the walk as a `var` parameter
(expr.xo comparable), as the Go builder shares its map: identical
output, the copies gone. A recursive walk that extends a list per level
should pass one list and pop (or share it as Go does), not copy and
push per call.
