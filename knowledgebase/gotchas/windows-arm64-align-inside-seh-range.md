# windows/arm64: an alignment directive inside a function's SEH range fails clang's assembler

Status: Observed (2026-11-20, task #505)

## Symptom

cmd/xo TestWindowsArm64Assembly failed on main from 553a8b9bd on: clang
-cc1as on the windows/arm64 assembly stopped with "fatal error: error
in backend: Failed to evaluate function length in SEH unwind info". The
push's gate did not run the cmd suites (the Std Proven push rules), so
main's own check found it.

## Cause

553a8b9bd loads a Float constant with one `ldr d` from constants
emitted at the function's end, each after a `.p2align 3`. On
windows/arm64 that end was before `.seh_endproc`. The unwind data holds
the function's length, which the assembler must know as a constant when
it writes the record; an alignment between `.seh_proc` and
`.seh_endproc` makes the length depend on the layout.

## Fix

compiler/arm64 emit.xo writes the constants after `.seh_endproc` on
windows/arm64 (they stay next to the function, in reach of `ldr`).
Nothing that aligns or switches section goes between `.seh_proc` and
`.seh_endproc`. Run `go test ./cmd/xo -run TestWindowsArm64` for a
change to what the arm64 backend emits at a function's end.
