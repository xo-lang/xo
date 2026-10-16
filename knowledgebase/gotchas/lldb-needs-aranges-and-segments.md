# lldb maps addresses to DWARF units only with aranges and the segments

Status: Observed (2026-10-16, the dSYM of selfhost/macho, decision 0239).

Symptom: `dwarfdump --lookup` finds the line, `b main.xo:9` sets a
breakpoint, but lldb prints `unable to resolve a line table file
address ... back to a compile unit` and shows disassembly, not source.

Cause: lldb finds a compile unit for an address through .debug_aranges
(or the unit's own ranges, which it did not use here) and resolves file
addresses through the dSYM's section headers; a dSYM with only __DWARF
and no aranges gives it neither.

Fix: selfhost/macho dsym.xo copies the executable's segment and section
headers (no contents) into the dSYM, as dsymutil does, and synthesizes
a .debug_aranges set per xo unit from its low_pc and high_pc.
