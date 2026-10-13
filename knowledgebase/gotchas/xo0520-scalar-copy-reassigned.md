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

Seen again (2026-10-13, std/native/llrt/digest_tables.xo, decision
0195): the generated rounds start sixteen `var`s from `t[i + k]`; `0 +
t[i + k]` passed the check but cost an `add #0` per load on arm64 (the
backend does not fold it), so the generator writes `let x = t[i + k]`
then `var v = x`, which costs nothing.

Seen again (2026-10-13, std/native/llrt/tz.xo, decision 0193 step 5): `var hi = w[H_NTX]` in a binary search and four `var`s of tzset started from `w[...]` and swapped; fixed with `0 + w[H_NTX]` and with `let`s chosen by `if` expressions.
