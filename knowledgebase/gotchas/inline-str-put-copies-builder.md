# --inline-str: BytesBuilder.put copied the whole builder each call

Status: Observed (2026-11-04)

**Symptom.** With `--inline-str` (decision 0370), bench/perf jpeg_decode ran
5.8x slower and xo's self build 40x slower; 80% of jpeg's time was
`libcstr.copy_words` under `obj.move_at`, called from `put_be`. Output was
identical, so no golden failed.

**Cause.** compiler/ssa call.xo made the builder an object before
`put_le`/`put_be` (an inline Str has no bytes to store into) as
`str.concat_obj("", cur)`. The empty literal is the destination, so
`reserve` allocated a new object and copied the whole builder into it on
every put: quadratic in the builder's size. The linkers patch relocations
with `put_le`, hence the self build.

**Fix.** `str.concat_obj(cur, "")`, the shape of the `write` path: an append
of nothing keeps a unique builder's object in place and makes only an
inline Str an object. Nothing ran `--inline-str` on a regular gate, so the
regression sat unseen from 0399 (put) until the 2026-11-03 survey; a gate
row now runs it.

**Avoid.** To make a value an object, append to it (`x + ""`); never
prepend to an empty literal.
