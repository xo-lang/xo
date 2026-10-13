# arm64 MIR: a plain instruction with a result is assumed to have no side effect

Status: Anticipated for the deletion, Observed for the spill registers
(2026-10-13, decision 0200; selfhost/arm64 and internal/arm64)

Symptom: an inline atomic whose result is unused (`ldaddal` of a fetch
add nobody reads, `swpal` of a swap) disappears from the output; a
`cas` sequence ending in `cset` is copied into a conditional branch and
runs twice; with every operand spilled, an early result and the first
reloaded operand both get x16.

Cause: dce (removable) and live_dce delete any K_PLAIN instruction
without a fault exit whose definitions are dead; thread_cset turns
`t = <text>; cset; r = t; jump J` into a branch whose flags come from
`<text>` (keeping the original for dce to delete). emit_plain reloads
spilled uses into x16, x17, x15 in template order and wrote every
spilled definition through x16, which is wrong when the definition is
early (written while the uses are still read).

Fix: atomics are K_PLAIN instructions with `keep` set (Inst.keep, inst
keep in Go): removable, live_dce, and thread_cset skip them; they are
also early, and emit_plain now gives a spilled early definition the next
scratch register no reloaded use took (and faults the compile for a kept
instruction when none is left rather than emit wrong code). Any future
instruction with a memory effect and a result needs `keep` too.
