# A jump table loop's speed moves 15 percent with where its `br` lands

Status: Observed (2026-10-28, bench/perf/llvm/xo/interp on the M4 Pro;
the 0388 stage 3 oracle, notes xo-lang-6)

Symptom: interp ran 74 ms built normally and 63 ms with std's bodies
kept out of program code (XO_ORACLE_OPAQUE=inline), three times, so the
inliner looked 15 percent too eager. But `xo dev dump asm --ssa=inline
--fn=run` is byte for byte the same in both builds, and run's final code
is the same modulo placement under every oracle kind: only main changed
(171 to 76 instructions, println and the interpolation no longer
inlined), which moved run from 0x100000ca0 to 0x100000c1c.

Cause: run's loop dispatches a 16 op program through one jump table
`br` (decision 0051). An indirect branch is predicted from its address
and the branch history, so the one `br` has to learn a period 16
sequence, and how well it does depends on the bits of its address.
Nothing in the inliner's cost model, register pressure, borrow or
uniqueness facts was involved: the hot code was identical.

Fix: never read a speed change off one placement. `XO_ARM64_PAD=K`
(decision 0075, in the Xo emitter since 3114a7f7e) shifts every
function by 4K bytes; judge a change by its median over K = 0 to 15,
and a loop whose spread is far above the noise of identical binaries
by its shape, not its address. For a dispatch loop the shape fix is
threaded dispatch: the latch, header and dispatch copied into each arm
so each arm ends in its own `br` (compiler/arm64 threadsw.xo
thread_switch, measured in notes xo-lang-6).
