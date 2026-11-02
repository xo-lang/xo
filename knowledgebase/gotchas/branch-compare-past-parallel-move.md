# A compare moved past a parallel move reads the new values

Status: Observed 2026-11-01.

**Symptom.** A native program (arm64 and amd64) leaves a loop after one turn, with no
error; LLVM gives the right answer.

```
var c = true
while c {
  let n = s.len()
  s.add(x)
  c = s.len() != n
}
```

**Cause.** `thread_cset` (compiler/arm64/opt.xo) turns a block ending
`t = cmp ...; cset; moves; jump J`, where J only tests the Bool, into a
branch on the compare. The compare's text then runs inside the branch,
after the block's parallel move. `n` is the loop header's parameter for
the length, and the move writes the new length into it, so the branch
compared the new length with itself. It takes values held in a register
across the back edge, so a small change around the loop hides it: a later
use of the value, or a compiler that does not promote the field.

**Fix.** The pass keeps the `cset` when the move or the chain's moves write
a register the compare reads. Any pass that moves a read below a
`K_PMOVE` needs the same check.

**How it was found.** Xo code in a compiler pass behaved differently after
a refactor; `xo dev bisect-pass --ref=off` named the pass. Neither progen
nor the goldens had a loop whose `var` Bool comes from a compare with a
value read at the loop's top: progen's `while` tests a counter against a
constant.

Golden: `internal/llvm/testdata/golden/loop_compare_moved`.
