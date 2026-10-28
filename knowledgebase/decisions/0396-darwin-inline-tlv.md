# 0396: Thread local reads on darwin/arm64 run dyld's fast path inline

Status: Proposed (Xo Lang 3, 2026-10-28). Implementation measured; the numbers and gates follow with the code.

## Context

Every runtime read of the thread's scheduler and heap records (`rt.tlocal_get`, `tlocal_addr`, `tlocal_set`; the
worker, the heap, the current fiber) goes through darwin's thread local variable descriptor: the generated code
loads the descriptor, loads its thunk (dyld's `_tlv_get_addr`) and calls it, with a stack spill of x0 and x30
around the call (arm64 calls.xo `tl_base`): about 17 instructions and a call and return. Linux reads the thread
pointer plus a constant (3 instructions). A profile of the channel ping pong had `tlv_get_addr` at 5.4% of its time
before decision 0394 and more of an iteration after it (a handoff is cheaper now, its thread local reads are not).

## Decision

`tl_base` emits dyld's fast path inline, 12 instructions and no call: read `_xo_tlv`, a word holding the
descriptor's key plus 1 and its offset (key + 1 in the low half), take this thread's block from the key's slot of
the table behind the read only thread register (`mrs TPIDRRO_EL0`, `and #-8`), add the offset. A zero word (not
published yet) or an empty slot (the thread's first read) takes the slow path: a call to `xo_tlv_slow`, which
calls dyld's own thunk (so the block is made as before), then publishes the word.

The descriptor's layout (a 32 bit key at +8, a 32 bit offset at +12) and the table behind TPIDRRO_EL0 are dyld's
private ABI, so the stub publishes only after checking that the descriptor's thunk is the one this fast path
copies: its first eight instruction words (`ldr w16, [x0, #8]; mrs x17, tpidrro_el0; and x17, x17, #-8;
ldr x17, [x17, x16, lsl #3]; cbz x17; ldr w16, [x0, #12]; add x0, x17, x16; ret`: 0xb9400810 0xd53bd071
0x927df231 0xf8707a31 0xb4000091 0xb9400c10 0x8b100220 0xd65f03c0, read from the macOS dyld in use on
2026-10-27). If any word differs (a future macOS), the stub marks the thunk unknown (`_xo_tlv` word 2) and every read
keeps calling it: slower, never wrong. The stub keeps the flags and every register but x0, x16 and x17.

The Mach-O encoder gains `mrs xN, tpidrro_el0` (0xd53bd060 | N; dyld's own word 0xd53bd071 for x17). The two
words (key plus 1 with the offset, and the unknown flag) are `_xo_tlv` in `__DATA,__data`.

Not changed: Linux, darwin/amd64 (its TLV call stays), the LLVM backend (clang's own thread local code), the
runtime's `xo_tlocal_get`, `xo_tlocal_set` and `xo_tlocal_addr` assembly functions (a call each, as before).

## Consequences

Measured (`xo bench --ab`, 20 rounds, baseline xo at the same base against this one): instructions mutex 0.924,
spawn 0.947, fanout 0.945, ping pong 0.925; wall mutex 0.926 (0.907 to 0.958), spawn 0.964, fanout 0.935
(0.917 to 0.947), ping pong 0.915 (0.908 to 0.93, a rerun on a quiet machine), pmap 1.000 (instructions) and 1.005
(wall). HTTP (CRUD, 32 and 128 connections): unchanged within noise (128: 150.4 to 153.1k requests a second before,
149.2 to 152.3k after). The `xo` binary grew from 113676041 to 113725753 bytes. These rows were taken with the first
form of the template; the final one reloads the offset from `_xo_tlv` (3 more instructions on the fast path), because
the emitter's spill scratch registers x16 and x17 can be the result register and the template may not write the result
before its last move (found by TestGoldenRunSpill with XO_ARM64_REGS=2). `_xo_tlv` is emitted after the immortal
placeholders (gen.xo end_data), so `_xo_empty_list` stays at page offset 0 (TestEmptyListAtPageStart). Gates: macho and
arm64 unit tests, 231 codegen patterns, TestGoldenAsm, TestGoldenRun, the spill goldens, the fixed point.

Risks: a macOS whose dyld changed the thunk or the descriptor (guarded by the word check: falls back to the call);
a program that spawns threads through a path that does not initialize the thread's slot (the slot is empty: the
slow path calls the thunk, as before). Code size: each read grows from 8 to 16 instructions (4 of them the cold
slow path).

## Alternatives

- Keep the call and trim the spill (stp/ldp of x0, x30): saves 2 of 17 instructions.
- Use a fixed thread register for the table (Linux's approach; macOS reserves x18 and has no free register
  without an ABI change): rejected.
- Cache the block in the fiber or worker record: the base may not be kept across a call (a task resumes on
  another thread), the reason `tl_base` re-reads it.
