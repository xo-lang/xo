# Keeping a `var` struct in memory: three SSA shapes that hide the value's real lifetime

Status: Observed (2026-10-12), selfhost/wasm stage 3 (decision 0179,
`selfhost/wasm/home.xo`).

**Symptom.** Passing `var` parameters of 16 leaves or more by reference
(the home of decision 0179's stage 3) first made the self hosted
checker's module bigger, 29.9 MB to 40.0 MB: most calls copied the
`Checker` into a temporary block and back, because the analysis decided
the value was still needed after the call. Then a version that did keep
it in memory printed wrong sums for a loop of `var` calls (t2 in the
results, `c.l` lost between iterations) and failed archive_formats.

**Cause.** Three shapes of the self hosted SSA (after InsertRC):

1. Live ranges widened over blocks in their order (plan_locals) are not
   liveness: blocks are not in dominance order, so a value used in a
   loop never "dies" at the call that gives it up. Use exact liveness
   (`dies_at`: no later use in the block, not live into a successor).
2. The SSA reads a call's `var` result once per use: `field 1, v19301`
   appeared 2,311 times in `Checker.resolve_type_obj`. Every such read
   is the same value; the analysis must treat them as one (`members`,
   `canon`), or a later read of one of them sees the memory already
   given to the next call.
3. A field the caller borrows for an argument (`c.set_file(c.cur_file)`)
   is retained before the call and released at the end of every path as
   `field 28, v3005`, a read of the value given up. One such read keeps
   the old `Checker` live through the whole function. Snapshot those
   field reads where the value is defined (`snaps` in plan_home) instead.

And the trap in the fix: a snapshot's locals must be live from where it
is taken, through every block between (the liveness walk must start
from the snapshot's block, not the read's), or another value reuses
them in a block placed later in the order (the t2 and t3 failures).

**Fix.** home.xo: the chain is the greatest set closed under its rules
(loops), checked by a forward dataflow over the blocks (every use must
see the home holding that value, else the home falls back to loading at
entry and storing at return); exact liveness per value class; field
reads after a give up read a snapshot. Small programs t2 and t3 (a
`var` struct in a loop with `Result` returns and landing pads) caught
both failures in seconds, before the goldens did.
