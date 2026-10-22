# The landing check after a call can be another value's

Status: Observed (2026-10-21, decision 0322 follow-up, Rc.uniq Field-of-call).

Symptom: the post-inline verifier (or any pass after insert_rc) rejects
an update of a call's `var` out that insert_rc accepted:
"updates: b1: v16 updates v13.1, which is neither a reuse result nor
provably unique" (golden uniq_var_params, arm64, only with proved marks).

Cause: prune_landings drops a call's trivial landing check (its pad
becomes -1, the rt.landing and its branch go). A later value in the same
block (here `list.set ... lands v21`) keeps its own check, so "the first
rt.landing after the call" is that value's, and the call looked as if it
landed into it. Before insert_rc the call had its own (pad set) check.

Fix: a call's landing is Value.pad; when pad is -1, an rt.landing after
it counts only when no value's pad names it (uniqvar.xo past_landing).
