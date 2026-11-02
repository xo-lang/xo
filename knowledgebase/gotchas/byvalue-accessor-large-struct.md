# A by value accessor on a large struct read out of a list costs a struct copy per call

Status: Observed (2026-11-02, compiler/check Obj Bool packing, decision 0448 case (c))

Symptom: packing Obj's 22 Bool fields into one `flags: Int` read through
`fn (o: Obj) done() -> Bool` methods (d749e20a8) made coll's warm build
use more memory, not less: RSS 46 to 58 MB, footprint 31 to 43 MB, heap at
check.setup 26 to 36 MB, 39k more allocations; Mac --ab wall 1.064, cpu
1.10, max RSS 1.237.

Cause: `c.objs[i].done()` passes Obj, about 61 words with
many lists and strings, by value: the element is copied out of the list
place and its reference fields retained for the call, where the field read
`c.objs[i].done` loaded one word. Compare
selfhost-by-value-receiver-marks-lists.md (go backend).

Fix: keep the bits in a one word struct field (`pub flags: ObjFlags`,
`fn (f: ObjFlags) done()`), so a read copies one word:
`c.objs[i].flags.done()` (80a8523a0): coll warm RSS back to 46 MB,
heap counter at check.setup 26 to 24 MB, Mac --ab wall 1.005 (within
noise), instructions 0.998. Rule: an accessor on a large struct read from
list elements takes the smallest field it needs, not the struct.

Tooling note: `xo refs FILE:LINE:COL` of a method found only the
declaring module's uses (compiler/check), missing compiler/ssa, gen and
cli; the rewrite fell back to the checker's XO0601 positions.
