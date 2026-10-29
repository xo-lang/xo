# lldb misplaces functions read through a debug map from Xo's objects

Status: Observed (2026-10-29)

Symptom: with a debug map (N_OSO) naming Xo-assembled objects, `breakpoint set -n F`
resolves, but at another function's address (prelude.str_split at ints.append_val).

Cause: Xo's assembler (compiler/macho asm.xo pointer_reloc) writes a DWARF pointer as an
extern ARM64_RELOC_UNSIGNED against the atom's symbol with only the addend in place, so
DW_AT_low_pc reads 0 in the object. ld64, dsymutil and Xo's linker apply the relocation;
lldb reads a mapped object's DWARF as written and never applies relocations.

Fix: the debug map's copies (std's <store>.parts, the program's dbgobj/) go through
compiler/macho dbgobj.xo debug_map_object: each such pointer gets the symbol's object
address in place and a section relocation (r_extern 0), as clang writes it. The linked
objects themselves are unchanged.
