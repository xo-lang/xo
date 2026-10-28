# x86_64 Mach-O atoms: an FDE pointer resolved into a dead exception table

Status: Observed (2026-10-28)

## Symptom

A darwin/amd64 build in atoms mode (XO_STD_ROOTS, std's store) failed in
the link, through the parts and through the store alike: `relocation into
__TEXT,__gcc_except_tab` (fib) or `__DWARF,__debug_line` (catch). The
objects themselves were sound: every relocation's target was where it
should be.

## Cause

An x86_64 object's __eh_frame points at its functions and LSDAs with pc
relative values and no relocation (compiler/macho link.xo
x86_eh_pointer). It found the section holding a target by scanning the
object's sections, after the strip had split them into atoms, and passed
that index to translate, which takes the object's own section ordinals
and maps them to atoms: atom 2 (a function at 228) was read as ordinal 2,
__gcc_except_tab, and mapped to a dead atom of it. arm64 never meets
this: its FDEs name their functions by relocation.

## Fix

x86_eh_pointer computes the output address from the atom it found; a
pointer into a dead atom (a dead function's FDE or its LSDA) becomes 0,
which __unwind_info never names and no live function's pc is.

A second trap behind it: with the link succeeding, a caught fault was
not caught on the Intel laptop (catch: a fatal traceback, then SIGSEGV).
The strip keeps a function's LSDA through what its FDE names
(strip.xo unwind_refs), read from relocations; x86_64 FDEs have none, so
every LSDA atom of __gcc_except_tab was dead, its pointer written as 0,
and no frame had a landing pad. unwind_refs now decodes an x86_64 FDE's
function and LSDA pointers (x86_eh_atom); the store's edges come from the
same table.
