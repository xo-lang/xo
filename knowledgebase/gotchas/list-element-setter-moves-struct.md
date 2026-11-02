# A var method or field write on a large list element moves the whole element

Status: Observed (2026-11-02, compiler/check Obj Bool packing, decision 0448 case (c); diagnosed by Xo Lang 1)

Symptom: packing Obj's 22 Bool fields into one `flags: Int`, read through
`fn (o: Obj) done() -> Bool` and written through
`fn (var o: Obj) set_done(v: Bool)` (d749e20a8), made coll's warm build
use more memory, not less: RSS 46 to 58 MB, footprint 31 to 43 MB, heap at
check.setup 26 to 36 MB, 39k more allocations; Mac --ab wall 1.064, cpu
1.10, max RSS 1.237.

Cause: the getters were free: `c.objs[i].done()` inlines to a load and a
bit test, with no copy. The setters were not: `c.objs[v].set_used(true)`,
a `var` receiver on a list element, lowers to list.take, setfield and
list.set, which loads all 57 words of the element, writes the hole into
its reference fields, sets one bit, and stores all 57 words back
(Checker.mark_used grew to 661 instructions). A plain field write
`xs[i].f = v` on a large element lowers the same way (take and set, plus
an index to read the old value), so any one-field update of a large list
element moves the whole element. The memory rise was a second effect
(found by Xo Lang 1 from check.ExR.obj's SSA, 2026-11-02): with the Bools
behind setters, compiler/ssa rc.xo's loop-entry reuse fired at each of
ExR.obj's six loops that push to an Obj list field, and the field there
was still the default empty list, which is never unique, so rl_copy
allocated a 4-element buffer before the loop, kept in the Obj even when
the loop never ran (FieldInfo: 4 x 152 + 16 = 624 bytes per field per
Obj). With ObjFlags no reuse is placed. Fixed in a3d2916dc: the reuse
skips an empty literal's list before a loop that only pushes to it.

Fix: keep the bits in a one word struct field (`pub flags: ObjFlags`,
with `fn (var f: ObjFlags) set_done(v: Bool)`), so a write moves one word:
`c.objs[i].flags.set_done(true)` (80a8523a0): coll warm RSS back to 46 MB,
heap counter at check.setup 26 to 24 MB, Mac --ab wall 1.005 (within
noise), instructions 0.998. Rule, until the backend updates the changed
fields in place (Xo Lang 1 is drafting it): write one field of a large
list element through a small field struct, not a method on the element.

Tooling note: `xo refs FILE:LINE:COL` of a method found only the
declaring module's uses then (fixed in 27c2467fa).

## Fixed for scalar fields (2026-11-02, Xo Lang 1)

Pass take_in_place (compiler/arm64 takeplace.xo) stores a scalar field
of a list element in its slot (`list.setf K`) when the taken element
only has fields read and scalar fields set on its way back to the same
slot: the 14-word repro's setter is ldr, orr, str. A field holding a
reference (Str, List, Box) is stored in place too when its old value is
read and released before the set, which is what the lowering of a field
write emits (stage 2, the same day): load the old value, store the new
one, release the old. An element passed whole, or set back through a
block with more than one predecessor, still moves. Golden pass_take_in_place covers equal indexes, a
read of another element and of a `var` parameter between, a call that
grows or shrinks the list, and a shared list; pass_take_in_place_ref
the same for Str and List fields (a swap of two elements' names, equal
indexes, the old value kept past its replacement, a call that grows the
list between the read of the new value and its store).
