# The playground compiler doubled when fault exits called zero_val.T

Status: Observed (2026-11-11, Xo Lang 2, from the owner's question on the
playground download)

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
58.2 to 26.2 MB, 4.50 to 3.15 MB brotli. Native builds are unchanged.
internal/selfhost TestWasmPlaygroundStack fails when the build passes
playMaxBytes (32,000,000).

## How to avoid

A change to what the SSA builder emits for every function (fault exits,
landings, zero values) is measured on the wasm build too, not only native
__text: the wasm backend's in-place homes depend on the exact shape of a
function's returns. Breakdown and method: bench/results/xoc-size-2026-11-11.md.
