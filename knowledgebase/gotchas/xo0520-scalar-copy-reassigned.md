# XO0520 on a scalar copied out of a list and then reassigned

Status: Observed (2026-10-11, selfhost/ssa verify.xo)

Symptom: `var want = rs[0]` followed by `if rs.len() > 1 { want =
m.tuple(rs) }` and a read of `want` fails with `error XO0520: mutation
of `want` is never written back`, though `want` is an Int index and the
list is never meant to change.

Cause: the lost update check (core 5.2) treats any local `var` whose
initializer copies an element out of a collection (`xs[i]`, `m.get(k)`,
`first`, ...) as a copy that must be stored back after its last change,
and a whole reassignment counts as a change (internal/check flow.go
lostUpdates marks every assignment whose place root is the binding as a
mutation site): the checker does not tell a value computed from the
copy from an unrelated new one, whatever the type. Reading or
returning the binding never counts as storing.

Fix: do not start a `var` from a collection element when it only picks
between values: `let want = if rs.len() > 1 { m.tuple(rs) } else {
rs[0] }`, or copy through a `let` first as core 5.2 says for scratch
copies (`let first = rs[0]; var want = first`).
