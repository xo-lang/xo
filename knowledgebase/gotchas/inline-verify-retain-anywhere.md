# The inlined module's verifier counted a retain anywhere in a function

Status: Observed 2026-11-18 (task #380).

**Symptom.** `XO_INLINE_STRICT=1 xo build --backend=arm64 bench/perf/json`
failed with "main: b246: v1031 updates v47, which is neither a reuse
result nor provably unique"; a plain build printed the warning and
built the whole program without inlining (slower, and quietly).

**Cause.** The loop in main pushes to `us` in place. After the loop,
`enc_small(us)` is inlined, and its body retains the list for each
`json.encode_str(us)`. Before inlining that retain was of the callee's
parameter; after it, the retain is of main's loop value. The
uniqueness analysis (ssa/rc.xo `unique`) records retains as one set
for the whole function, so the check at each push saw "retained" and
failed, though no path from the retain reaches the push.

**Fix.** verify.xo `uniq_here`: when the check fails, it is retried
counting only the retains in blocks that can reach the update's block
(rc.xo `unique_in`). A retain that cannot reach the update has not run
when the update runs. The rc pass itself still uses the whole function
set, so its code is unchanged. Golden `inline_retain_after_loop`
fails on the old verifier.

**Avoid.** A function-wide fact that the inliner can move into a
caller (a retain, a borrow) is checked where it can act, not wherever
it sits.
