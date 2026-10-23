# A block parameter may alias its argument: releasing the argument on the edge frees it early

Status: Observed 2026-10-23 (compiler/arm64 viewlen.xo, first version)

Symptom: a pass that rewrote a Str block parameter into its length,
releasing each incoming string on its edge instead of releasing the
parameter where it was, failed the inlined module's verification in the
mail_basic golden: `mail.ipv6_ok: b21: v80 uses v61, which is not
alive` (XO_INLINE_STRICT=1 fails the build; without it the inliner
silently falls back to the module before inlining, so the pass seems to
do nothing).

Cause: after insert_rc an edge argument is not always a transfer of a
reference. The verifier, like insert_rc, counts references by leaf: a
parameter can receive a value with no retain and stand for the same
leaf, while the argument is still read after the jump (here v61 was
passed to b19 and read again further down). Its one release is the
parameter's; releasing the argument on the edge ended the leaf before
those reads.

Fix: keep the parameter's Str and every release where it was; add a
parameter for what the pass wants beside it (view_lens: an Int length,
with the immortal empty string passed for the Str where a view came in,
whose release does nothing). Any pass that moves releases across a
jump must check the leaf's later reads, not the value's.
