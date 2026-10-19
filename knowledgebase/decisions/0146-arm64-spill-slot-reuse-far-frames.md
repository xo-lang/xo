# 0146: arm64 spill slots shared by disjoint live ranges; frames past 28 KiB through x28

Status: Accepted (2026-10-10). Supersedes the 28 KiB limit of decision
0128 (its probes and helper records stay). Implemented in
`internal/arm64/regalloc.go` (slot first fit, the second allocation
with x28 kept free), `internal/arm64/emit.go` (`frameBound`, the far
frame layout, `farSlots`, `memScale`, the probe loop of `probeSP`).
Tests: run program `internal/arm64/testdata/run/huge_frame` (4,500
values live across calls, about 36 KiB of slots, two stack parameters,
on the main stack and a fiber; TestRun), `TestFarFrame`
(`internal/arm64/frame_test.go`). Gotcha: `gotchas/arm64-large-frame.md`.

## Context

Each spilled value had a stack slot of its own for the whole function,
so a frame grew with the length of a function rather than with what is
live at once. The self hosted checker hit the 28 KiB limit of decision
0128 three times (`habit_call`, then `resolve_named_type` after one
field was added to `Checker`); the workarounds were splitting a
function into one function per `match` arm and moving a field back
into another struct. Past 28 KiB, slots and incoming stack arguments
leave the 32,760 byte reach of a scaled `ldr`/`str` offset from sp.

## Decision

- **Slot reuse.** A spilled value takes the first slot whose values'
  live ranges (the same ranges the register allocator uses, holes
  included) do not overlap its own, as a register would. The parallel
  moves already order memory to memory moves by location, so two
  values sharing a slot across one move are handled like registers.
- **Far frames.** When the frame bound after allocation (stack
  arguments, the `$OUT` slot, slots, saved registers, record) passes
  `maxFrame`, the allocation runs again with x28 kept free for the
  whole function and saved by the prologue. The frame is then laid out
  with the saved registers and the frame record below the slots (within
  reach of sp), and after emission every sp relative load or store past
  its instruction's reach (spill slots, incoming stack arguments) is
  rewritten as `add x28, sp, #hi, lsl #12` and an access at `[x28,
  #lo]` (`add x28, x28, #lo` first when lo is not encodable). The
  bound is 16 MiB (an `add` immediate), past any stack.
- **Probes** of a frame of more than 8 pages are a loop counting in x16
  (free in a prologue) instead of one `sub`/`str` pair per page.

## Consequences

- The self hosted checker's largest frame went from 28,176 bytes
  (`resolve_named_type`) to 6,448 (`check0`); the sum of its 809 frames
  from 2.28 MB to 1.10 MB; the arm64 build of the checker is about 4%
  faster on the selfhost input (5,547 to 5,349 ms for 20 runs).
- No function fails to build for its frame size below 16 MiB; a far
  frame costs one callee saved register and one `add` per far access,
  only in such functions.
- Go mode (goasm.go) is unchanged: Go's assembler splits large offsets
  itself, and its save slots stay one per value.
- Lowering a 4,500 statement function takes about 10 seconds, mostly
  in `ssa.dominators` (map lookups); not frame related, recorded as the
  next speedup for huge functions.

- Addendum (2026-10-19, pass arg_slots, compiler/arm64 regalloc.xo): a
  spilled value copied to a stack word (a call's stack argument or
  result past the registers, or this function's own stack parameter or
  result) takes that word as its spill slot when no value live at the
  same time holds it and no call in its range gives the word to its
  callee (each call's words are clobbered at its position: a callee may
  write the stack words it takes, its arguments as well as its results,
  as AAPCS64 allows). The copy then goes, src and destination equal.
  This function's own words are free once the entry has read them. Not
  for x86_64, Float values, or far frames. xo's `__text` 32.24 to 31.52
  MB (-2.21%), the HTTP server's assembly 3.7% fewer lines; golden
  arg_slots (in the spill and poison runs); `XO_PASSES=-arg_slots`.

## Alternatives

- **A second base register set once** (x28 = sp + 28 KiB): doubles the
  reach but keeps a limit.
- **Picking a free scratch register per access**: the emitter's
  scratch registers can all be in use around a template or a parallel
  move, so it is not always possible; reserving x28 only in far frames
  is simple and always correct.
- **Splitting functions in Xo sources**: the workaround this replaces.
