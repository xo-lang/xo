# Bounds checks come back after a call that takes a `var` List

Status: Observed (2026-10-13, decision 0208, Argon2id's argon2_g)

Symptom: a function checks `if 127 >= t.len() { fault }` once at entry
so that `t[0]` to `t[127]` need no bounds checks, but the reads of t
after a loop of calls such as `blamka_row(t, i)` (t a `var List[Int]`
parameter of the callee) are each `mov`, `cmp`, `b.hs` again: 384
instructions a block in argon2_g.

Cause: the callee returns the list, and the backends know nothing of
its length after the call (it could have pushed or popped).

Fix: check the length once more after the calls in the source (in the
generated digest_tables.xo of commit a4168f52, after the column loop:
two instructions instead of 384); or keep the reads before the calls.
Measuring that change through a scratch xo binary first showed nothing:
rebuild the binary after editing std (xo-binary-embeds-std-native.md). A callee that only
sets elements could tell its callers (an interprocedural length fact),
which no backend has yet.
