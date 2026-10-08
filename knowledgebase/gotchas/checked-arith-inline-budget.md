# Checked Int arithmetic keeps small functions from inlining

Status: Observed (2026-10-08, bench/results/perf-mono-2026-10-08.md);
fixed 2026-10-08 (update section of the same file)

Symptom: `fn (r: Rect) area() -> Int { r.w * r.h }` was called, not
inlined, in a hot loop (`go build -gcflags=-m=2`: cost 87 exceeds budget
80), and a generic `total_area` loop ran 2.3x slower than Go even with
`--mono`. Without `--mono` every checked `+` on an Int was itself a call.

Cause: two layers. (1) `rt.AddS` and friends are Go generic functions;
Go compiles them per GC shape with a dictionary argument, and the shape
copies cost 84 (AddS) to 237 (DivS) nodes, over Go's 80 node budget.
(2) Even a typed helper with the fault moved out of line spends about 60
nodes on the call to the fault function (Go's inliner charges about 57 per
call), so a caller with one checked operation is near the budget and one
with two is over it.

Fix:

- Typed int64 helpers (`rt.AddI..RemI`, `List.AtM`, std/rt/mono.go) are
  emitted by default. Their fault is `panic(&arithFault{a, b, op, at})`:
  the inliner charges a panic about one node, so AddI costs 29 nodes and
  MulI (bits.Mul64, an intrinsic, behind a 32 bit fast path) 69. The
  message and the stack are built when the panic is recovered (see
  `gotchas/panic-valued-fault-stack.md`).
- A constant operand selects a one comparison form (`rt.AddIUp`,
  `rt.SubIDown`, `rt.MulIK` with precomputed bounds, plain `/` and `%` by a
  constant other than 0 and -1).
- The range analysis (internal/gen/ranges.go) emits plain Go for
  operations it proves cannot fault, and `List.AtU` for indexes it proves
  in range.

`Rect.area` now inlines (cost 72) and the iface workload runs at 0.80x of
Go. The generic `rt.AddS` family (`--no-mono`, sized integer types) still
calls an out of line fault and stays over the budget.
