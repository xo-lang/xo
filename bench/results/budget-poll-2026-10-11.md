# Budget poll cost (decision 0173), 2026-10-11

What the `tk.Tick()` polls of resource budgets cost on the Go backend,
outside any budget (the poll is a nil check of `tk.bud` there).

Program (scratch, not committed): `fib(35)` (call-bound recursion),
`sum(500000000)` (a `while` loop of integer arithmetic), and
`sieve(20000000)` (a `for` over a range with a nested `while`, indexing a
`List[Bool]`). Built with `xo build` from main at ab15d71 (old) and with
polls in every loop and every non-leaf function (new); 5 alternating
rounds through heavy.sh on the shared 12-core Mac (ten agents running, so
wall clock is noisy; the rounds agree to a few percent).

| benchmark | old | polls everywhere | change |
|---|---|---|---|
| fib(35) | 19.9 to 20.7 ms | 29.0 to 29.9 ms | about +45% |
| sum (while loop) | 1.411 to 1.422 s | 1.412 to 1.417 s | none measurable |
| sieve | 68.8 to 71.1 ms | 80.3 to 92.1 ms | about +18% |

`(*Task).Tick` inlines (`go build -gcflags=-m`); the cost is the load
and branch in small bodies and the call it adds on the slow path, which
changes register allocation in tiny recursive functions.

Decision: polls are emitted only when the program calls
`task.with_budget` somewhere (`gen.usesBudgets`, like the `xo_nohttp`
tag). A program without budgets generates the same Go as before (the
gosnap goldens are unchanged), so it pays nothing; a program that uses a
budget pays the figures above in all its code, budgeted or not.

Next experiments, ranked:

1. Poll only code reachable from a `with_budget` closure (a call graph
   from the closures passed to it, through function values
   conservatively): the rest of a budget-using program pays nothing.
2. Poll recursion only in functions on a call-graph cycle (SCC), not in
   every non-leaf function: fib would keep its poll, most non-leaf
   functions would lose theirs.
3. Hoist the poll of a counted `for` over a range to every 1024
   iterations (an inner counter in a register), which should remove most
   of the sieve's cost.
