# A wasm runtime function not at a line start is stripped

Status: Observed (2026-11-02, Xo Lang 4, decision 0447 stage 3)

Symptom: `xo build --targets=wasip1/wasm` failed with "the assembler: wat:
call of unknown function $wasi_stdin_read" although rt7.xo's text for
that function was appended to the runtime text, and a program that
called it lowered the call.

Cause: compiler/wasm gen.xo's `strip_runtime` reads the runtime text a
line at a time and starts a function at a line whose trimmed text begins
`(func $`. The text appended before it did not end with a newline, so
`(func $wasi_stdin_read` sat on the previous function's last line and
became part of that function's chunk; that function was unreachable, so
both were dropped.

Fix: start an appended runtime text with "\n" (gen.xo does for
rt7_p1_text). When a runtime function goes missing, check that its
`(func $` begins a line in the concatenated text.
