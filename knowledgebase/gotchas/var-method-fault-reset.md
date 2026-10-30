# A fault exit builds a zero copy of every large result, in every function

Status: Observed (2026-10-30)

## Symptom

A small `fn (var g: G)` helper in compiler/arm64 (range_value, then a
twelve line give_sub) added 7.2 to 8.8 KB to xo's `__text`: about 470
`adr` and 760 `str` after `bl _xo_x_fault_continue`, and a 3.7 KB stack
frame. Across xo (main, 2026-10-30, `__text` 70,970,408 bytes), 1,974
functions carry 925,923 `adr` and 2,018,714 `str`/`stp` after their
fault_continue: 11.8 MB of zeroing.

## Cause

compiler/ssa build.xo finish_fn gives the unwind exit zero results
(`b.zero` per result). A `var` parameter's out is a result, so the exit of
a method on the arm64 generator builds a zero G field by field, one empty
List or Map per field, inline. Any `var g: G` method with a fault path
has it, including one whose only fault is a call that may fault. The out
is written back to the caller's variable, so it must stay a real zero (a
placeholder could be dropped after a catch).

## Why it hid

Each push reported only its own `__text` delta, and a few KB per function
read as the cost of the change. Cold code never shows in callgrind,
XO_PROF or `xo bench --ab`, which measure executed paths. It was found
only when one helper's size was broken down by opcode.

## Fix or workaround

Until the compiler builds the zero once per type: keep code that may
fault, in a new helper, out of `var g` methods (a plain method returning
what it found, as isel's range_subs), and let a caller that already has
the path apply the result. Before accepting a helper's size, disassemble
it (`objdump -d --disassemble-symbols=SYM`) and look at what follows
`bl _xo_x_fault_continue` and at the frame size. A codegen change's
effect on xo needs a stage 2 build (xo built by the new xo); the
checkout's bin/xo was built by the older one.
