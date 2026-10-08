# An elided effect argument in a result type cannot be a caller's variable

Status: Observed (2026-10-07, implementing core 6.2 elision in the P1 checker)

Symptom: with the literal reading of core 6.2 ("in a function signature it
becomes an implicit effect variable of that function"),
`pub fn build(client: http.Client) -> Svc { Svc{cache: new_cache(fn(id) {
fetch(client, id) })} }` was XO0701: "the expected function type allows only
`Svc.Cache.fx`, but performs `net.dial`". Even `fn wrap(c: Cache[Int, Str])
-> Svc { Svc{cache: c} }` would fail, because the parameter and the result
get two different variables.

Cause: an effect variable in the result type is universally quantified: the
caller picks it, so the body must work for every choice. A body that builds
the value has one concrete effect set.

Fix (P1 reading, listed in `bench/results/check-corpus-v0.4-spec.md`):
omitted effect arguments in parameter and receiver types are implicit effect
variables of the function, which it may perform without naming them; in the
result and error types they are effect inference variables bound by the
body. Call sites see the bound set through `EffSet.subst`, which follows
bound inference variables so a passed through parameter variable is
instantiated per call. Limitation: the inference variable belongs to the
declaration, so a caller checked before the body with an explicit effect
annotation can bind it first; P1 does not report that.
