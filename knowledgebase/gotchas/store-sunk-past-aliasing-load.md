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
