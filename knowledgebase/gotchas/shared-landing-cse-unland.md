# Dropping a landing with cleanup code can strand other landings

Status: Observed (2026-11-19, task #438)

**Symptom**: under `XO_INLINE_STRICT=1`, test/codegen builds failed with
"the inlined arm64 module does not verify ... bigmod.modulus: b5: v254
lands on v60, which is not a check" (consttime, fnaddr, intrinsics) and
"prof.trace_text_at: b267: v1306 lands on v754, which is not a check"
(winx18, windows/arm64) after cse began merging a checked operation
with its dominating twin and dropping its landing (`unland`).

**Cause**: two ways. A landing can serve more than one checked operation
(the inliner shares a call's landing), so dropping it for one left the
others pointing at a value that no longer ends its block with a branch.
And unlike prune_landings, which drops only trivial pads, unland cuts
off a pad with cleanup code: prune_func then removes the pad's blocks,
including a landing of their own (v754) that code still reachable
another way (a cold fault block, b267) landed on.

**Fix**: `unland` (compiler/ssa/passes.xo) keeps a landing any other
value lands on, and cse snapshots the function before its first unland,
checks every landing after the pruning (`landings_ok`), and restores
and reruns without unlanding when one is not a check. A pass that
removes blocks after dropping a landing checks every value's `pad_of`
afterwards, not only the ones it rewrote.
