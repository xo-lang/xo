# An `&&` in a loop condition hides its second half from the bounds proof

Observed 2026-10-26.

## Symptom

A loop whose cursors are already bounded by a guard still pays a bounds
check on the second cursor:

```xo
fn merge(xs: List[Int], lo: Int, hi: Int, mid: Int) {
  if hi > xs.len() {
    rt.fault_raise("merge: the end is past the list", "std/native/prelude.xo:0")
  }
  var i = 0
  var j = mid
  while i < (mid - lo) && j < hi {
    let a = xs[j] // checked, though hi <= xs.len()
    ...
```

The proof has the fact `hi <= xs.len()`, and `j < hi` is in the source, yet
the check stays. The fact holds at the use only when the guard's true edge
cannot reach it: `return` leaves the function and a fault ends the program,
so both work, and the std faults because a guard that returned would leave a
broken caller with a silently unsorted tail.

## Cause

The front end lowers a short circuit into nested branches whose result is a
block parameter: `i < left` becomes a branch, `j < hi` becomes a value, and
the loop body's branch tests a *phi* of that value and `false`. The arm64
bounds proof (`compiler/arm64/fieldlicm.xo`, `new_proof`) reads comparisons
at branch terminators only (`c = f.values[branch.args[0]]`), so a comparison
carried through a phi is invisible: no `j < hi` fact is recorded, and the
check cannot be dropped.

## Fix

Write the two tests as a loop and a break guard, which both reach a branch
terminator:

```xo
  while i < left {
    if j >= hi {
      break
    }
    let a = xs[j] // no check: j < hi <= xs.len()
    ...
```

A block parameter whose every incoming value is proven still works (the
index of a loop counter is a parameter), and a clamp
(`if h > xs.len() { h = xs.len() }`) carries its fact through the join's phi,
so the rewrite only matters for a short circuit in the condition itself.

## Alternatives

Teaching the proof to read a phi-carried condition (which edge makes the phi
true, and which comparison that edge tested) would remove the need for the
rewrite. It is not done: the phi's `false` arm and the loop back edge make
the implication graph larger, and the rewrite is a few lines in one std
function.
