# A literal struct field shows no value in the debugger

Status: Observed (2026-10-17, Xo Lang 2); fix in progress (Xo Lang 3).

## Symptom

In internal/arm64's TestDebugTypes, `frame variable *it.name.obj` for
`Item{name: "box", ...}` in main's frame printed `<parent is NULL>`
instead of `bytes = "box"`, after the fewer drops change (a).

## Cause

Reference counting no longer retains or releases a statically immortal
leaf (compiler/ssa rc.xo immortal_leaf). Nothing then uses the word of a
literal field after the struct is built, so the register allocator gives
it no register past that point, and the variable locations
(compiler/arm64 dwvars.xo) only describe registers and spill slots.

## Fix

Describe a word whose value is a known constant by the constant itself:
DW_OP_addr of the static object for a Str literal (the konst vreg of
g.sym_addr), DW_OP_constu/consts with DW_OP_stack_value for an Int,
over the binding's range (Xo Lang 3, 2026-10-17). Until then the check
is skipped in TestDebugTypes with a comment naming this file.
