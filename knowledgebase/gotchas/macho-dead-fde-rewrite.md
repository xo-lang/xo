# Mach-O store links kept every dead function's FDE

Status: Observed (2026-10-28)

## Symptom

An HTTP server linked through std's store (or its parts) had an
__eh_frame 4 times a whole program's (243 KB against 59 KB): 4456 FDEs
for about 2700 functions, some 3300 of them with pc begin 0. Dropping
those FDEs then failed every link with `no FDE for the DWARF mode
function at 4`.

## Cause

compiler/macho strip.xo kept __eh_frame whole: its FDEs' pc fields are
SUBTRACTOR pairs whose addend is minus the field's own offset in the
section (cfi.xo), so a record cannot move without its addends being
written again. A dead function's FDE stayed, pointing at its dead
atom's address (0). Some dead functions' compact unwind entries also
survive live_unwind and name addresses near 0; their lookup had been
answered by those dead FDEs.

## Fix

strip.xo eh_live (arm64) writes __eh_frame again without the FDEs whose
function atom is dead: the following records move back, each moved
SUBTRACTOR field's addend becomes addend - shift, each FDE's CIE pointer
is recomputed; applied in dead_strip and in StoreLink.resolve, so the
store's link stays the parts' byte for byte. unwind.xo unwind_info
leaves out a compact entry whose function atom is dead: a store link folds
dead atoms into each object's empty __dead section inside __text, so
such an entry's address looks like code (`at 4295088376`, a dwline
function), and an address range test does not catch it.

A second trap in the same rewrite: an object's __eh_frame shorter by
some records was no longer a multiple of its alignment (8), the link
padded between objects' sections with zeros, and a reader takes a zero
length for the terminator: the merged section ended after 114 FDEs and
every later function had none. The last kept record now grows by
DW_CFA_nop bytes (0) to keep the size a multiple of the alignment.
