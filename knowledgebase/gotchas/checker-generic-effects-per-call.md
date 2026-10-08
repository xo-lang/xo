# Inferred effects of generic methods are per call

Status: Observed (2026-10-07, building the P1 checker, corpus task 07)

Symptom: `pub fn (c: Cache[K, V, E, fx]) get(...) uses (clock, fx)` was
reported as performing `fx` outside its declared set, through a call to the
non pub method `c.run_flight(...)`.

Cause: implicit receiver type parameters (std encoding rule 5, and how the
corpus writes generic methods) are created per method, so `run_flight` has
its own `fx`. Its inferred effects mention that `fx`, which is a different
parameter from `get`'s `fx`. The reference to an inferred function's effects
was not instantiated at the call.

Fix: an effect item that refers to the inferred effects of a function
carries the substitution of the call (`EffItem.Sub`); flattening maps the
callee's effect parameters through it. Any new way to reference a function
(values, method values) must carry the substitution too.
