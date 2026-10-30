# wasm: a `var` home's temporary copy was made after the call's resume point

Status: Observed 2026-10-30.

## Symptom

The wasm build of xo (`xo.wasm` under node, WASI) built a different program
than native xo, then trapped in `rc_dec` (use of a freed block, the
`Module.funcs` list dropped twice by `build_native`). With `XO_DBG`-style
probes: the module was right inside `arm64.inline_cached` (177562 values)
and in `inline_or_defer`, and stale (the module from before inlining,
69433 values) in the caller `build_native`. Programs whose `var`
parameter is a large struct (HOME_MIN = 16 leaves or more) and whose callee
can suspend (a task `await`, a `scope`, I/O) were affected.

## Cause

`home_call` (compiler/wasm/call.xo) passes a `var` parameter of a function
with homes as the address of a temporary block holding a copy of the
argument, read back after the call. The copy (`temp_copy`: alloc and copy)
was emitted after `resume_point()`. A callee that suspends makes the caller
re-enter at the resume label and call it again: that re-ran the alloc and
copy, so the resumed call passed a fresh copy of the argument's OLD value,
the callee's updates stayed in the first block, and the caller read the
stale copy back. Ownership then followed the stale module (a leaked new
list, an extra release of the old one).

## Fix

`home_call` makes the temporary copies (`temp_make`) before the resume
point for a callee that can suspend (`g.susp`), as `sret_block` already did
for wide results; after the label it only pushes the same block's address.
Golden: internal/llvm/testdata/golden/wide_var_suspend (a 160 leaf `var`
parameter whose callee awaits two spawned tasks).

## Avoid

Anything that allocates or copies for a call must be emitted before the
call's `resume_point()`; the code after the label runs again on every
resume.
