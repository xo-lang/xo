# Spilling a `var` receiver loses the mutation

Status: Observed (2026-10-07, bench/xo/v0.4/06-job-queue)

Symptom: `st.jobs.put(id, released with {state: Dead})` inside a `var`
method did nothing: 9 of 11 job queue tests failed (leases never expired).

Cause: to keep left to right evaluation, the generator spills operands that
come before an operand that needs statements (here the `with` expression)
into temporaries. The receiver of a `var` method is a place, and
`x_a := (*v_st).F_jobs; x_a.M_put(...)` mutates the copy.

Fix: operands that are places (a `var` receiver, a `&place` argument) are
never spilled (`operandsKeep` in internal/gen). A place that is not
addressable (through a `Box` or a collection element) is copied, passed,
and written back after the call. Golden test: `var_receiver_order`.
