# arm64: a block that if-conversion skips still hands its memo down

Status: Anticipated (2026-10-17, compiler/arm64 isel.xo range_branch,
range_and, logic_value, select_branch); avoided by construction

Symptom (would be): `reads virtual register N before it is written`, or
a wrong value, in a block whose SSA dominator is a block that an
if-conversion jumps over.

Cause: instruction selection walks the SSA blocks in reverse postorder
and starts each block's memo (constants in registers, quotients, list
lengths, loaded words) from its immediate dominator's
(opt.xo inherit). The if-conversions in isel.xo make a branch go
straight to a later block (`c >= 48 && c <= 57` as one unsigned
compare, `if c { 1 } else { 2 }` as csinc), so the blocks in between
are unreachable in the MIR, but they are still selected (as dead code)
and their memos still exist. A block whose dominator is one of them
would inherit registers that the code that now runs never writes. A
constant of 65536 or more is the usual carrier (inherit keeps those in
registers across blocks).

How it is avoided:

- range_branch skips the block holding `c <= hi` whose true successor
  it dominates, so the bound must be below 65536 in absolute value: that
  block then puts no register in its memo that inherit keeps.
- range_and, logic_value, and select_branch skip blocks that dominate
  nothing (the join has two predecessors, so its dominator is the block
  that branches, whose memo is right).
- A side whose instructions select_branch selects into the branching
  block (spec_ok) is marked in G.spec and selected as a trap, not
  again: its values keep the registers of the branching block.

When adding another if-conversion: check what the skipped blocks
dominate; either they dominate nothing, or nothing in their memo is a
register the new path does not write.
