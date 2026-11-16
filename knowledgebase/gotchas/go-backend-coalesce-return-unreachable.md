# go vet: unreachable code after `?? return` in generated Go

Status: Observed (2026-11-16, main check alert #1119)

**Symptom.** `go vet ./std/rt` failed with `std/rt/std_gen_metrics.go:5949:3:
unreachable code` after std/metrics' `parse_proc_stat` gained
`let at = text.rfind(")") ?? return Err(...)`. Builds and runs were fine.

**Cause.** The Go backend lowered the right side of `??` as a value: a
`var x_v Never` block holding the `return`, then `x_c = Absurd(x_v)` after
it. The assignment follows a terminating block, which vet reports.

**Fix.** compiler/gen `coalesce` lowers a diverging block (`return`,
`break`, `continue`, a block or match of type Never) with a discard
destination and writes no assignment after it.
`internal/gen` TestCoalesceDivergingVet builds such a program with
`--go-tree` and runs go vet on it. A new lowering that puts a value after
a block of type Never has the same trap: vet the tree it writes.
