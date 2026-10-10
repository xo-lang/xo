# 0128: arm64 frames up to 28 KiB with stack probes; helper frame records past stp reach

Status: Accepted (2026-10-10); its 28 KiB limit superseded by 0146 (spill slot reuse, frames past 28 KiB through x28). Implemented in `internal/arm64/emit.go`
(`maxFrame`, `probeSP`, prologue), `rc.go` and `closures.go`
(`recordAt`). Tests: run programs `internal/arm64/testdata/run/big_frame`
and `wide_values` (TestRun), `TestBigFrameProbes`,
`TestWideHelperRecords` (`internal/arm64/frame_test.go`). Gotcha:
`gotchas/arm64-large-frame.md`.

## Context
The self hosted parser (decision 0127) did not build with
`--backend=arm64`: `Parser.parse_file` needed a 5120 byte frame (the
backend refused frames over 4000 bytes), and the reference counting
helpers of its syntax tree enums, values of 80 and more words, stored
their frame record with `stp x29, x30, [sp, #688]`, which the assembler
rejects (stp reaches 504 bytes). The LLVM backend built it.

## Decision
- Frames up to `maxFrame` = 28672 bytes. Every spill slot, saved
  register, and up to 4 KiB of incoming stack arguments stay within a
  scaled 12 bit `ldr`/`str` offset (32760). The frame pointer of a frame
  record at 4 KiB or more is set with `add x29, sp, #hi, lsl #12` and
  `add x29, x29, #lo`.
- A frame larger than one page is allocated a page (4 KiB, the smallest
  guard page: Linux, and one page below every fiber stack) at a time,
  each followed by `str xzr, [sp]`, so it touches the guard page before
  anything below it instead of stepping over it into another mapping.
- Helpers (`rcHelper`, the pointer variants, closure environment drops)
  store and load the frame record with two `str`/`ldr` when it is past
  504 bytes.

## Consequences
- Functions of a few hundred live values (large parsers, generated
  code) build natively. Past 28 KiB the build still fails with "frame
  too large"; spill slot reuse would shrink such frames and is not done.
- Probing costs one store per page in the prologue of only those
  functions.

## Alternatives
- **Split functions in the Xo source**: a workaround in every program,
  and the limit was arbitrary.
- **A loop of probes**: smaller code for huge frames, unneeded at 28 KiB
  (at most 7 probes).
- **Reuse spill slots across disjoint live ranges**: the real fix for
  frame size, a register allocator change; recorded as a next step.
