# Value semantics: a mutated copy that is never written back

Status: Observed (2026-10-07, paper test task 05)

Symptom: updating a map entry under value semantics is get, modify, put:
`var b = m.get(k) ?? new_bucket(); b.take(now); m.put(k, b)`. Leaving out
`m.put` silently discards the update, and spec 0.1 has no check for it.

Fix (decision 0016): `XO0520` when a mutated local `var` is never read again,
and `Map.update(k, init, fn(var V) -> R)`. Limitation: if the stale copy is
read later (for example to compute a Retry-After value), XO0520 does not fire,
so prefer `update` over get, modify, put.

Update (decision 0023): XO0520 now only checks `var` bindings copied out of a
collection, which removes the false positives seen in the spec 0.2 run.

Update (2026-10-07, P1 checker): the implemented XO0520 follows 0023
literally, so it does not see a copy taken through any other call. The v0.3
corpus writes `var u = db.users.get(id).or_err(ApiErr.NotFound(id: id))?`
(task 01); with `db.users.put(id, u)` forgotten, nothing is reported. The
check is also source ordered, not path sensitive: a read in another branch
(`if c { b.take(now) } else { show(b) }`) counts as a later read. Prefer
`update`, and see `bench/results/check-corpus-v0.3.md` for the proposal to
add `.or_err(e)?` to the list of copy forms.

Update (spec 0.4, decision 0034): XO0520 now covers `.or_err(e)?` copies and
a local `var` mutated after being stored into a collection (`xs.push(v)`,
`m.put(k, v)`, `m[k] = v`) and not stored again, which is the planted bug
that 0023 missed (variant X6b in `internal/check/planted_test.go`). A store
later in an enclosing loop counts as storing again, so building one snapshot
per iteration is accepted. The stored rule ignores reads after the mutation.
