# A new prelude interface method breaks every implementation of it

Observed 2026-11-09 (Xo Lang 2, decision 0581's Stdio.raw_input).

## Symptom

After adding `raw_input` to the prelude's `Stdio`, pushq's overlap check failed with `error XO0401: expected
`Stdio`, found `Prefix`` (and `Tee`, `PrefixStdio`) in internal/llvm goldens os_literal and user_void_caps and in
cmd/xo/testdata/nogc/userfs. The offload gate had passed: its LLVM step runs only the goldens a change touches.

## Cause

`Stdio` (like `Fs`, `Net`, `Clock`) is an interface a program may implement itself (a capability wrapper passed in
an `Os` literal). A new method makes every such implementation incomplete. The compiler has its own (compiler/xo
wholebuild.xo `Tee`), and so do test programs and user code.

## Fix

Before pushing a new method on a prelude capability interface, find every implementation:
`grep -rn "fn (.*) read_secret" --include=*.xo .` (any existing method of the interface works as the probe; `xo
refs` does not list implementations yet). Add the method to each, and run the goldens that hold one. User code
outside the repository breaks the same way: the change log entry says so.
