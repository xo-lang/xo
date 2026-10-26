# Merging two generated functions does not remove the body they both held

Status: Observed (2026-10-25, decision 0372's stage 0).

Symptom: folding `xo.maph.listput[T]` into `xo.maph.putrest[T]` with one
mode arm per path (mode 0 the lists put, mode 1 the replay) measured
216 B a Map type and 1,768 B on CRUD, about a third of the 0.9 to 1.1 KB
a type the estimate expected from removing a copy of `put_body`.

Cause: both arms still held a full `put_body`. Merging the thunks deduped
only the entry, prologue and epilogue, not the body, so the text that
mattered stayed duplicated inside one function. A body is shared only
when a single copy is reached from both paths.

Fix: compute the input the two paths disagree on inside the arm, then
fall into one body:

```
let sv = b.local()
let (yes, no) = b.branch(b.bop(Op.Eq, mode, b.cint(1)))
... yes: b.write_var(sv, b.cbool(false))        // the replay marks nothing
... no:  b.write_var(sv, b.maph_put_pro(t, ...)) // the lists put's flag
... seal/join, then one put_body(t, mp, k, v, b.read(sv), rebuild, hash)
```

The replay's path is unchanged for the old `-1` ("no mark"), since
`maph_marked` only branches when the flag is a value. The same change
then measured -4,304 B on CRUD and -6,856 B on an 8 Map type synthetic
(819 to 1,057 B a type).

Check: measure such a dedup on two shapes (4 and 8 Map types) and read
the family's per-function rows in sizebreak, not only `__text`; a mode
arm that looks shared in the source is the shape that hides this.
