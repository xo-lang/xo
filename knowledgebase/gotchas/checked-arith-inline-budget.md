# Checked Int arithmetic keeps small functions from inlining

Status: Observed (2026-10-08, bench/results/perf-mono-2026-10-08.md)

Symptom: `fn (r: Rect) area() -> Int { r.w * r.h }` is called, not
inlined, in a hot loop (`go build -gcflags=-m=2`: cost 87 exceeds budget
80), and a generic `total_area` loop runs 2.3x slower than Go even with
`--mono`. Without `--mono` every checked `+` on an Int is itself a call.

Cause: two layers. (1) `rt.AddS` and friends are Go generic functions;
Go compiles them per GC shape with a dictionary argument, and the shape
copies cost 84 (AddS) to 237 (DivS) nodes, over Go's 80 node budget.
(2) Even a typed helper with the fault moved out of line spends about 60
nodes on the call to the fault function, so a caller with one checked
operation is near the budget and one with two is over it.

Fix: `--mono` emits `rt.AddI/SubI/MulI/DivI/RemI` (std/rt/mono.go, int64,
inlinable; MulI checks a 32 bit fast path) and `List.AtM`, fixing (1).
For (2), a `panic(&Struct{...})` fault path costs about 1 node plus the
literal (a two check probe costs 60 instead of 142 with a shared `goto`
fault call or 162 with one call per check); it needs the fault's stack to
be taken at the first recover from the frames below `runtime.gopanic`.
Not done yet; first next step in the results file.
