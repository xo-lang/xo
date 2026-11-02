# A store moved past a load of another base reads stale memory

Status: Observed 2026-11-02.

**Symptom.** A native arm64 program computes a wrong Float from a list
element it just wrote, only when two indexes are equal at run time; LLVM,
Go and the same build with `XO_PASSES=-slp_f64` are right.

```
fn mix(var vs: List[V], i: Int, j: Int) {
  ...
  vs[j].z = c - vs[i].w
  vs[j].w = d - vs[i].z   // i == j: must read the new z
}
```

**Cause.** `slp_f64` (compiler/arm64/slp.xo) pairs the two stores to
`vs[j].z` and `vs[j].w` into one 128 bit store at the later one.
`slp_stores_movable` let the pair pass a Float load of any other base
register, but `vs[i]` and `vs[j]` are two base registers for one element
when `i == j`, so the load of `vs[i].z` ran before the store of z.
Introduced by eff7996a8 (2026-10-21); v0.25.0 to v0.35.0 have it.

**Fix.** A load through another base ends the move (it may alias), and a
load pack's member counts as passed only when its pack loads before the
first store. Golden: `slp_alias_store`. Any pass that moves a load or a
store past another must treat two different base registers as the same
memory unless it can prove otherwise; a test of such a pass runs each
call with equal and unequal indexes.

## Also observed: a group left scalar read a lane of a group made (2026-11-02)

After the fix above, progen.Alias seeds 54, 64 and 66 (Lang 3) stopped
miscompiling and failed to build instead: "m0 reads virtual register 49
before it is written". slp_commit skipped a lane's read by an
instruction that is itself packed, assuming it becomes the pack's; when
that reader's group was rejected (here because one of its own lanes was
read too early), the reader stayed scalar and read the other group's
lane before its pack. The check now records such reads with the
reader's group and leaves a group unmade, to a fixed point, when the
reading group is not made. Goldens slp_alias_54, 64 and 66.

## Also observed: a build read a packed lane in the other order (2026-11-02)

progen.Alias seed 387 (Lang 3) failed to build with "m2 reads virtual
register 21 before it is written". An fadd pack's second operand was
an fsub pack's two lanes in the other order (`b - 1.0` then `a - k`),
so slp built it with zip1 from the two scalars, which the fsub pack
had replaced. The read was skipped for the same reason as above (the
reading fadd is packed), but this time the reader was made and still
read the scalar, through the build. A lane a kept build reads is now
extracted after its pack, and the lane's group is not made when the
build's pack comes at or before the lane's. Golden slp_alias_387.
The general rule: a "packed reader reads through the pack" shortcut
holds only when the reader's operand is that pack, not a build of
its lanes.
