# A reference counting change can cross an inliner limit by two instructions

Status: Observed (2026-11-03, Xo Lang 1, decision 0465's view_proj)

Symptom: with XO_PASSES=+view_proj (a Str view a projection: rt.str_borrow
plus a Retain where the view must be owned, instead of rt.str_view, which
counts itself), `xo bench --ab` on linux/arm64 put bench/perf/trimchars at
1.18 (trim) and 1.26 (trim_start) of the time without it, while a program of
views compared in place ran at 0.91. The SSA after insert_rc differed only
in `str_view` becoming `str_borrow` and `retain` side by side.

Cause: arm64's inliner counts a Retain as an instruction and a call by its
name. scan.trim_chars, called from one site, cost 149 instructions against
INLINE_ONE_PADDED_INSTRS (150); its two returned views each gained a Retain,
151, and `XO_INLINE_REPORT=trim` showed `no (pads)` where it had shown
`small`. Not inlined, the trims paid a call and lost the caller's constant
cutset. A first guess, an out of line retain thunk, was wrong: rt.str_view's
own count used the same thunk.

Fix: compiler/arm64 inlsum.xo fn_sum does not count a Retain right after its
rt.str_borrow (together they are what rt.str_view was), and rt.str_borrow is
in inline.xo's inline_intrinsic (instructions only). Two other traps of the
same change: the verifier builds its own Rc (verify.xo), so a flag on Rc is
invisible to it (rc.xo kind() treats rt.str_borrow as a projection with no
flag); and folding an owned view's Retain back into a counted rt.str_view
failed the inlined module's verification of xo itself (a counted view is
owned to the verifier, a projection to the pass).

A second pass keyed on the old shape: arm64 viewlen.xo (view_lens) turns a
view used only for its length into `hi - lo` and matched rt.str_view by
name; with view_proj an inlined trim that was only measured built the view
and called the retain again (Mac, normal builds: 37 ms against 42 for
trimchars trim_start). It now takes rt.str_borrow and drops its Retain. Any
pass that matches an intrinsic by name (grep the compiler for the name) is
part of a change to that intrinsic's shape.

How to check: for a change to what insert_rc emits, compare
`XO_INLINE_REPORT=NAME` of the hot functions with the change off and on
(both runs with some XO_PASSES entry, since any entry turns the caches off),
and `xo dev dump ssa --fn=NAME --stage=rc` with XO_PASSES set the same way.
