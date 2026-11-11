# The playground compiler doubled when fault exits called zero_val.T

Status: Observed (2026-11-11, Xo Lang 2, from the owner's question on the
playground download; the second cause the same day)

## Symptom

compiler/wasm built for wasm (the playground's compiler) grew from 22.5 MB
(0.35.0) to 58.2 MB (main, 2026-11-11), 2.40 to 4.50 MB brotli, while the
compiler's and std's source grew about a third. The same 0.35.0 source built
by each release's xo: 21.5 MB of code with 0.35.0, 43.8 MB with 0.36.0, and
about the same since. Half of every function was leaf by leaf copies between
memory and locals: ssa.Builder.net_err_func, 30 lines, was 372 KB (30 KB
natively). Nothing watched the wasm build's size, so no gate saw it.

## Cause

2d51a8403 (2026-10-30, a native code size fix) made a fault exit's large
struct result one call of a shared `zero_val.T` instead of a zero built in
place. The wasm backend passes a large `var` parameter by its address (a
home, compiler/wasm/home.xo) and leaves its value there; a fault exit that
returns a zero it can write into the home keeps that, but a call's result is
not one, so the home fell back: the parameter loaded into locals at entry
and stored back at each return, in every `var` method of Builder, Checker
and Module.

## Fix

zero_val_name (compiler/ssa build.xo) gives no zero_val.T for a wasm build,
so wasm fault exits build the zero as before. Main's playground compiler:
58.0 to 26.2 MB, 4.45 to 3.15 MB brotli; user programs with such methods
shrink too (crypto_prims 15.9%). Native builds are unchanged.
internal/selfhost TestWasmPlaygroundStack fails when the build passes
playMaxBytes (32,000,000).

## A second cause: snapshots over a loop's back edge

After the fix, the largest functions (ssa.Builder.fe_replay 414 KB,
net_err_func 342 KB) still copied the home's fields into a frame: 781
snapshot reads in net_err_func, field 0 of Builder alone 619 leaves. A field
read of a home's value is snapshotted (home.xo plan_home, "gone") when it
can run after the value is given up (an update `b.m = ..`, a call taking b,
an edge passing b on). The test used plain reachability from the give up,
and in a loop every block of the body is reachable from the update through
the back edge, so every `b.m.funcs[i]` read at the loop's top read a
snapshot, although each iteration's b is the loop header's parameter (or
an update in the body), defined afresh before the read.

Fix: a read counts only when it is reachable from the give up without
entering the block that defines the value (home.xo reach_avoiding), for a
give up in a block or along an edge; not for the reads of an earlier call's
`var` result (partof), which read the value given up. The home's dataflow
still checks every read taken off the list, and falls back (hwhy) rather
than miscompile; none did across the playground's compiler. A nested update
(`b.m.funcs = ..`) also stays in the home when b.m's other uses are field
reads done before it (plan_nested reads_before). The playground compiler:
26.2 to 22.6 MB, 3.15 to 2.75 MB brotli.

## How to avoid

A change to what the SSA builder emits for every function (fault exits,
landings, zero values) is measured on the wasm build too, not only native
__text: the wasm backend's in-place homes depend on the exact shape of a
function's returns. Breakdown and method: bench/results/xoc-size-2026-11-11.md.
