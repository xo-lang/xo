# A new instruction in the WAT runtime needs the wasm assembler's opcode

Status: Observed (2026-10-13; b54614f2's `i64.rotr` in rt2.xo)

Symptom: every wasm program fails with `the assembler: wat: unknown
instruction \`i64.rotr\``; TestSelfhostWasm drops from 130 passing
goldens to 70 (below wasmMinPass), with no Go test failing.

Cause: selfhost/wasm assembles its own WAT (asm.xo's opcode table), and
the runtime text in rt*.xo is assembled into every module, so one
mnemonic the table lacks breaks every program. The table had the i32
rotates but not `i64.rotl` (0x89) and `i64.rotr` (0x8A).

Fix: add the opcode to selfhost/wasm/asm.xo with the runtime change and
run TestSelfhostWasm (`-run '^TestSelfhostWasm$'`, about a minute). A
check: every `i32.`/`i64.`/`f64.` mnemonic at the start of a line in
selfhost/wasm/rt*.xo must appear quoted in asm.xo.
