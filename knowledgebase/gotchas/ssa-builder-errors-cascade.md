# The SSA builder's unsupported errors cascade

Status: Observed (2026-10-09, decision 0083)

Symptom: the first `xo check --no-gc` of a `match` with a list pattern
reported two XO1201 errors at the same arm: "list patterns" and
"converting `()` to `Int`".

Cause: the builder keeps going after an unsupported construct (so it can
report every one), giving the construct a placeholder value of the unit
type. The code around it then fails to lower that placeholder (here the
arm's value converted to the match's type) and records a second error
that is only a consequence of the first.

Fix: `ssa.NoGCDiagnostics` drops an error whose span overlaps one
recorded before it (core 10.2: never cascade). When adding an
`unsupported` call, record it on the node that is unsupported, not on an
enclosing one, or it can hide an independent error inside the same span.
