# Inlining a large padded callee at its one site does not pay

Status: Observed (2026-10-26), an experiment, not landed

Context: the native inliner takes a callee with landing pads only at one call
site and only up to INLINE_ONE_PADDED_INSTRs (150) instructions
(compiler/arm64/inline.xo, decision 0256's rule). num.layout, 990 instructions
and 225 blocks of bounds and overflow landing pads, stayed a call from
text's float path where LLVM inlines it, so it looked like free money for the
float_fmt row of bench/perf/text (LLVM ahead of native).

Tried: a rule in the pads branch of inlinable_set that also takes a padded
callee with exactly one call site when that site is not inside a loop of its
caller (a helper one_site_in_loop) and the callee updates no list in place,
regardless of INLINE_ONE_PADDED_INSTRs. XO_INLINE_REPORT changed from
"inline num.layout: instrs 990 blocks 225 sites 1 no (pads)" to "... small".

Result: no win. bench/perf/text float_fmt (1000 floats times 200 rounds,
arm64), seven alternating runs of the two binaries built by one xo at
dd9e4a954: 119,495,632 to 119,441,594 instructions retired (-54,038, -0.045%,
inside the run to run spread of about 0.1%), and __text 487,768 to 490,992
bytes (+3,224 B). Reverted.

Why: the call boundary is a few instructions per formatted float, while the
gap to LLVM on that row is about 25 instructions per float (LLVM 114,479,320,
native/LLVM 1.044 on dd9e4a954). It lives in the bodies, not in the calls:
num.layout's decimal digit loop, num.dbox and num.digits8, and the string
push path (cabi.obj_rs_push_words).

What to do instead: attribute the per-call instructions between native and
LLVM's code for one formatted float, then remove work (checks with a range
argument, a cheaper digit path); the l3 profile of xo's own llvm build gives
the shares: obj_rs_push_words 13.1%, digits8 11.0%, dbox 10.5%, layout 8.6%
flat and 55.0% cumulative.

Trap for the next agent: a size and site count alone does not tell you a call
costs anything. Measure instructions retired A/B on the row before writing the
rule.
