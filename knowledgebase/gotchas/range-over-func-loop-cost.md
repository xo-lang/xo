# `for i in a..b` through an iterator costs a call per iteration

Status: Observed (2026-10-08, bench/perf `iter`, bench/xo/v0.4/03-retry-client)

Symptom: a loop over a range literal ran its body through
`rt.Range.All()`, a Go range over func iterator: the body becomes a closure
(`F_backoff-range1`) called once per iteration, with state checks, and the
closure shows up as an extra Go frame in fault stacks.

Cause: Go compiles `for x := range seq` over an `iter.Seq` into a closure
for the body and a call of the iterator; the iterator's loop is not always
inlined into the caller.

Fix (2026-10-08, internal/gen/fn.go `forStmt`): a range literal
(`for i in a..b`, `a..=b`) is a counted Go loop over its bounds, evaluated
once; `a..=b` stops after `b` without computing `b + 1` into the binding.
Range values that are not literals still use `.All()`. The iter workload
went from 9.8 ms to 5.4 ms; fault stacks of code in such loops no longer
list the `-range1` frame (03-retry-client's contract fuzz failure prints
one Go frame fewer; messages and locations are unchanged).
