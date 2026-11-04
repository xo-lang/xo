# A gate passes while the inliner silently falls back

Status: Observed (2026-11-04)

**Symptom.** Decision 0487's gate (every arm64 golden under
XO_FREE_CHECK=1, the codegen patterns, the fixed point) passed, yet a
25-line program with two field writes after a List.get payload made "the
inlined arm64 module does not verify" (`v132 uses v44, which is not
alive`) and built without inlining.

**Cause.** A build whose inlined module fails the rc verifier only warns
and builds without inlining, and its output is still right. The goldens
run with XO_INLINE_STRICT=1 (internal/xotool sets it for every test), but
none of 0487's had the failing shape (the chain's projection used after
two writes, inside a caller the prelude was inlined into); the verifier
(verify.xo carried_alive) followed one setfield of the owner chain, not
two.

**Fix.** verify.xo follows the chain transitively, and a golden of the
shape inlined into its caller guards it. A hand run (`xo build`, `xo run`,
`xo asm` in a script) sets `XO_INLINE_STRICT=1` itself; an rc, borrow or
inliner change adds a golden whose shape is inlined into a caller, not
only one in its own function.
