# A pass that needs another pass's cleanup must ask whether it is on

Status: Observed (2026-11-20, task #461)

Symptom: with `XO_PASSES=-fold_tuples` every program failed its build
under XO_INLINE_STRICT=1: "the inlined arm64 module does not verify (30
errors; the first: prof.trace_text_at: b72: edge to b68 holds {...},
another predecessor {... v1342.1:1})". TestGoldenPassesOff
pass_struct_spills/fold_tuples was red on main for a day; the "all" case
passed, since it turns the inliner off too.

Cause: af88679ee let a callee with `var` parameters inline at a call with
a landing pad (inline.xo pad_tuple_ok). Its copy returns through a tuple,
which verifies only after fold_tuple_fields has replaced the field reads
by the tuple's operands. The new rule was tested with every pass on.

Fix: pad_tuple_ok asks `pass_on(m, "fold_tuples")`. A change that leans
on a later pass to make its output valid checks that pass's switch, and
its test dir goes into internal/arm64 passhunt_test.go's passHuntTargets
with that pass (varlist_set_fault: fold_tuples). Run
`go test ./internal/arm64 -run 'TestGoldenPassesOff/^DIR$'` before
pushing an inliner or pass change.
