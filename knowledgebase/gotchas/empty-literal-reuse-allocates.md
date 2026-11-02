# A loop-entry copy of an empty default list allocates

Status: Observed (2026-11-02, Xo Lang 2's coll RSS, found by Xo Lang 1)

## Symptom

A build that decodes many records into a struct with list fields
(`check.ExR.obj` making an Obj) held about 620 bytes per list field per
record that were never pushed to: +12 MB in use at the check phase on
coll, allocated at the `for` lines of the decoder, not at the pushes.
The same decoder written with the Obj's flags in a nested struct
(80a8523a0) allocated none.

## Cause

A struct literal's omitted list field is `rt.rl_empty`, the immortal
empty list, which the uniqueness analysis (compiler/ssa rc.xo `uniq`)
never counts as unique (an explicit `[]` local is `list.new`, which it
does: a small repro with `fields: []` never showed it). When a loop
updates the field on every iteration and the value entering the loop is
not provably unique, rc.xo's loop-entry pass puts a `reuse` of it in the
entry block, and copy-on-write's slow path (rl_copy) answers the
immortal empty list with a new list of min_cap elements, before the
loop, even when the loop then runs zero times. Whether the pass fires
depends on the value's provenance: 22 inlined flag setters made the Obj
a block parameter (not provably unique), a nested flags struct did not.

## Fix

The loop-entry pass leaves out the reuse when the entering value is the
empty literal's list and every update of it under the loop is a push
(2026-11-02): each push keeps its own check, and a push into the empty
literal's list already makes a new list (rl_grow_n). Leaving the reuse
out is always sound (the parameter is then not proved unique). Golden
rc_empty_entry: the empty list also held by another local, another
field and a closure, read and passed between pushes, and in-place writes
after a loop that ran zero times. A runtime-only fix (rl_copy of the
empty list giving capacity 0) cut coll's heap at the check phase from
64.96 to 50.46 MB but added 8.5% allocations (a header, then a grow,
for every such list later pushed), so it was dropped.
