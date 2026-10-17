# A literal struct field shows no value in the debugger

Status: Observed (2026-10-17, Xo Lang 2); fixed 2026-10-17 (Xo Lang 3).

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
over the binding's range (Xo Lang 3, 2026-10-17). compiler/arm64 records
the constant registers (G.kints from imm, G.ksyms from sym_addr, with
debug information on). dwvars.xo dv_plan gives a word in one (no other
instruction defines it; its own definition may have been deleted as
unused) the constant's location over the binding's other words' range.
A string's address is DW_OP_addrx, an entry of the part's .debug_addr
(debug.xo). The driver turns string addresses on (ssa.Module
dw_data_addrs) for ELF links and compiler/macho's own Mach-O link (the
default on macOS), not for a cc link on macOS: dsymutil relocates an
address in a location list as the function's
(gotchas/dsymutil-loclist-address-function-offset.md), so that link
shows the length but not the bytes. A binding whose words are all
constants has no range to borrow and still shows no location.
