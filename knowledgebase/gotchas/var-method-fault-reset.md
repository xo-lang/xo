# A fault exit builds a zero copy of every large result, in every function

Status: Observed and fixed (2026-10-30)

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

## Fix

A struct result of eight or more fields now gets its zero from one
generated function per type, zero_val.T, which the fault exit calls
(compiler/ssa build.xo fault_zero). zero_val.T is made while a body is
built (finish_fn), never by add_landings: the cleanup cache (decision
0382) stands in for a function whose landings grew nothing, so a function
made in that pass could be missing from a later build that reads a cached
caller. xo's own native `__text` went from 71,005,440 to 62,590,056 bytes
(xo built from the same source by main's xo and by the fixed one).

Do not lower the threshold (FAULT_ZERO_FIELDS) below 8 yet. At 2 the
runtime's fault report types (dump, dwline, prof) share a zero too, and
internal/llvm TestGoldenSplit/fault_task_exit/none (split 3) fails on
linux/arm64, 3 of 3 runs (passes on macOS and as one module): the program
prints "faulter: deferred" and then exits 70 at the task instead of
unwinding to its scope. The cause is unknown as of 2026-10-30; 2 saved
another 137,408 bytes.

A shared zero is a call, and inlining can turn the callee's `return` of
it into a join argument in the caller; there a large struct call result
takes its own frame copy (byref_var's main, 105,104 bytes). The arm64
inliner inlines a zero_val call that feeds a join (inline.xo
find_zero_join), and a struct over 3,072 words (24 KB, the reach of the
arm64 outgoing area) keeps the inline zero (build.xo
FAULT_ZERO_MAX_WORDS), since the call result is a second copy in the
frame even at the function's own exit (Huge.bump, 69,504 bytes).

## How to avoid the next one

Before accepting a helper's size, disassemble it (`objdump -d
--disassemble-symbols=SYM`) and look at what follows
`bl _xo_x_fault_continue` and at the frame size. A codegen change's
effect on xo needs a stage 2 build (xo built by the new xo); the
checkout's bin/xo was built by the older one, so a compiler change never
shows in its own binary.
