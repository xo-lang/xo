# A new instruction in the WAT runtime needs the wasm assembler's opcode

Status: Observed (2026-10-13; b54614f2's `i64.rotr` in rt2.xo)

Symptom: every wasm program fails with `the assembler: wat: unknown
instruction \`i64.rotr\``; TestSelfhostWasm drops from 130 passing
goldens to 70 (below wasmMinPass), with no Go test failing.

Cause: compiler/wasm assembles its own WAT (asm.xo's opcode table), and
the runtime text in rt*.xo is assembled into every module, so one
mnemonic the table lacks breaks every program. The table had the i32
rotates but not `i64.rotl` (0x89) and `i64.rotr` (0x8A).

Fix: add the opcode to compiler/wasm/asm.xo with the runtime change and
run TestSelfhostWasm (`-run '^TestSelfhostWasm$'`, about a minute). A
check: every `i32.`/`i64.`/`f64.` mnemonic at the start of a line in
compiler/wasm/rt*.xo must appear quoted in asm.xo.

## Again, hidden as unsupported (Observed 2026-10-29)

rt.xo's Float32 of a UInt64 (4d7692a97, 2026-10-23) emitted
`f32.convert_i64_u` (0xB5), which the table lacked, so the u64_to_float32
and float32 goldens failed to build on wasm in 0.33.x and 0.34.0. Unlike
i64.rotr it was not in every module, and compiler/wasm main.xo reports any
generate error, the assembler's too, as `== unsupported ...`, which
TestSelfhostWasm counted as unsupported, not failed. The test now fails a
`== unsupported` line naming `the assembler:`. Check: every
`i32.|i64.|f32.|f64.` mnemonic in compiler/wasm's .xo files against asm.xo's
table (`comm -23` of the two sorted lists; only `f64.const` is parsed apart).
