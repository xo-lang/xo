# arm64: pair_mem drops labels nothing in the text refers to

Status: Observed (2026-10-16, DWARF line tables, decision 0239).

Symptom: `xo: assembler: LpGL1jtk_f8_l4 is used but not defined` when
a label referenced only from outside the function's text (a debug
line row, referenced from .debug_line) sat between two loads or stores.

Cause: compiler/arm64 lsr.xo pair_mem joins two loads or stores into
ldp or stp across labels that neither the function's text nor its call
site table refers to, and drops those labels (block labels only fallen
into).

Fix: debug row labels end in `_dl<digits>` (is_row_label) and pair_mem
keeps them, before the pair, instead of refusing to pair (which would
make a build with debug information generate different code). A new
kind of label referenced from another section must be kept the same
way, or added to pair_mem's references.
