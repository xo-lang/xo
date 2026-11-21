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

## A match arm that is an immediately called closure (Observed 2026-11-21, task #570)

Symptom: `go vet` on the generated Go reported `unreachable code` for
`return (match e { A(n) => n + 1, C => fn(x: Int) -> Int { x ^ (-1) }(p) })`.
The output was right on every backend. Found by progen
(TestArithProperty seeds 5690147, 5700612, 5700269) in the pre-tag hunt,
task #569.

Cause: the arm lowers to `return func(...) {...}(tk, v_p).V`, a return
that ends on a line beginning with `}`. ends_terminating
(compiler/gen/fn.xo) reads the last line at the block's indentation and
knew `}` alone (a closed block, parsed) and lines beginning with a jump
keyword, so `}(tk, v_p).V` read as not terminating and the arm got a
`break LABEL` after the return.

Fix: any last line beginning with `}` is parsed
(never_falls_through), which finds the return the line closes.
Regression: internal/gen TestMatchArmClosureCallVet.
