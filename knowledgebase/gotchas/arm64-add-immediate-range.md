# arm64: `add` takes a 12 bit immediate, and a frame's $OUT slot can be past it

Status: Observed (2026-10-10, the arm64 build of compiler/ssa)

Symptom: `xo build --backend=arm64 compiler/ssa` failed in the assembler:
`error: expected compatible register, symbol or integer in range [0,
4095]` at `add x6, sp, #4808`, before a call of `xo_str_replace` in
`Builder.expr`.

Cause: a runtime call that returns a Str gets the address of the frame's
$OUT slot (the length out parameter) in a register, made with `add xN,
sp, #outOff` (internal/arm64 emit.go, a move of kind mFrame). The slot
lies above the stack arguments of the function's calls, and a call that
passes a large struct by value (the SSA builder's `Builder`, which holds
the whole checker, passed to a by value method) puts more than 4 KiB of
arguments there. `add` encodes an unsigned 12 bit immediate, optionally
shifted left by 12, so 4808 is not encodable. Loads and stores reach
further (`ldr x19, [sp, #4808]` is fine: a scaled 12 bit offset reaches
32760), which is why only this add failed.

Fix: `addSP` emits `add xN, sp, #hi, lsl #12` and `add xN, xN, #lo` for
an offset past 4095. `internal/arm64/testdata/run/far_out_slot` (a struct
of 600 words passed by value, then `replace`) runs on the arm64 and Go
backends, and `TestFarOutSlot` requires the shifted add. Any other
`add ..., sp, #` of a frame offset needs the same care.
