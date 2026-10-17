# XO0520 on reassigning a `var` that was initialized from a collection element

Status: Observed (2026-10-12), not fixed

Symptom: `xo check` reports `error XO0520: mutation of `j` is never
written back` with the note "`j` is a copy of a collection element taken
here" for an Int that is only reassigned, not mutated:

```xo
var j = bs[p].insts[0].targets[0]
while steps < 3 && bs[j].insts.len() == 1 {
  j = bs[j].insts[0].targets[0]   // XO0520 here
  steps += 1
}
```

Hit while writing `thread_const` in compiler/arm64/opt.xo (decision
0184). Reproduced with a 20 line program (the code above with two
structs) by the Go checker of 2026-10-12.

Cause: the element copy rule of core 5.2 treats any assignment to a
`var` that was initialized from an element read as a mutation of the
copy, even when the whole variable is replaced and its type is a scalar.

Fix (workaround): initialize through a `let` first, `let j0 =
bs[p].insts[0].targets[0]; var j = j0`, or move the loop into a helper
that takes the start as a parameter. Next step for a real fix: in the
checker's XO0520 analysis (internal/check, and its mirror in
compiler/check), exempt plain reassignment `x = e` of the whole
variable, and add the program above as a checker golden that must stay
clean.
