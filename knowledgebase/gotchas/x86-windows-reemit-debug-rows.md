# windows/amd64's second emission kept the first's DWARF rows

Status: Observed (2026-10-22, Xo Lang 2)

Symptom: the first windows/amd64 build with DWARF failed in the
assembler: `.LpGL1jtk_f84_dl0 is used but not defined`. The function's
text had row labels from `_dl50`, its .debug_line named `_dl0` on.

Cause: compiler/arm64 x86emit.xo x86_emit emits a windows/amd64 function
a second time when the first emission borrowed a register
(borrow_short: frame words for the borrowed registers). It dropped the
first emission's text, stubs and call sites, but not its debug state:
the rows (dl_rows), the row counter (dl_n), the function's debug record
(dbgs) and the variables' state, so the second emission's record listed
the first's labels too.

Fix: x86_emit saves the debug state with the rest before the first
emission and restores it before the second. Any new per emission state
in G (state.xo begin_func) that x86_emit_once changes belongs in that
save and restore.
