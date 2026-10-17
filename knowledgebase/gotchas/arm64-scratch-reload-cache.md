# arm64: x16 and x17 keep a spilled value between instructions

Status: Observed (2026-10-17: the first version miscompiled the self build)

Symptom (expected): a wrong value with no error on the arm64 backend,
after a change to the emitter or a new way to write the stack; or, in a
debugger, a write to a spilled variable that the program does not see
right away.

Cause: since 2026-10-17 selfhost/arm64 lsr.xo reload_reuse, a pass over
each function's final text (before pair_mem), drops a reload `ldr
x16|x17, [sp, #k]` when that register already holds slot k since its
last reload of k or its store to k. What it holds is forgotten at a
call (bl, blr), at an instruction that may write the register (a
computation whose result it is, a load into it, a swap, an exclusive
store), at anything that moves sp or touches the stack in another
form, and for slot k at a store to k from another register. At a label,
it keeps what held at every branch to the label and on the fall
through, if every reference to the label came before it in the text;
a label referred to later (a loop head) or from ext (the call site
table: landing pads) starts empty. Only the emitter writes spill slots
(by reference memory is its own frame region, mem_off; rt.frame_addr
gives the frame record). A first version kept the cache in the emitter
(emit_plain) and decided a block's predecessors from the MIR; jump
forwarding (fwd), rotated loop tests, and inlined header tests branch
where the MIR does not say, and the self build's s2 miscompiled the
checker (s3 failed with 1061 wrong exhaustiveness errors): the text is
the only place every branch is known. An early template's definition
must also avoid a use's register the cache kept (the same fix there).
A debugger's write to a spilled variable (decision 0253 point 8) may
not be seen until the next call or join point: x16 or x17 can still
hold the old value.

Fix: a new instruction form that writes x16, x17, or the stack belongs in
lsr.xo writes_scratch (its unit tests: selfhost/arm64 lsr_test.xo); for a
debugger write, document the limit or have the debugger stop only where
the cache is empty (a call, a block that starts fresh). Checks: progen
across backends (internal/gen TestArithProperty, TestCompileProperty
with XO_COMPILE_PROPS), the fixed point, and the arm64 goldens.
