# Generated test programs that store then mutate fail XO0520

Status: Observed (2026-10-08, internal/gen/list_model_test.go)

Symptom: random programs that copy lists around failed to check with
`error XO0520: mutation of `c` is never written back`, even with every
binding passed to a function at the end.

Cause: the lost update check (core 5.2) rejects a local `var` that is
stored into a collection (`m[k] = c`, `g.push(c)`) and mutated
afterwards without being stored into a collection again; passing it to a
plain function does not count, and a later `c = ...` resets the
tracking. Value semantics tests do exactly this on purpose.

Fix: store through a call, `m[k] = id(c)` with `fn id(xs: List[Int]) ->
List[Int] { xs }`; the value is still copied through the same own mark
(the callee returns its parameter). Direct stores are covered by the
golden program `list_value_semantics`, which stores again where needed.
