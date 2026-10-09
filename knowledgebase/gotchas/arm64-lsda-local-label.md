# A call site table under an assembler local label points into code (Mach-O)

Status: Observed (2026-10-09, decision 0085)

Symptom: with fault unwinding on the arm64 backend, the landing pads of
the first function with a call site table worked, and those of every
later function never ran: the personality routine read the table of
`leaf` as `site f9419210f00000+...`, bytes of instructions.

Cause: each function's table was emitted under an assembler local label
(`Lf3_lsda`) in `__TEXT,__gcc_except_tab`. A local label is not a symbol,
so the whole section was one atom for ld64, which resolved the
`.cfi_lsda` reference of every function as the atom's start plus an
offset it got wrong; only the table at offset 0 survived.

Fix: on Mach-O the table is labelled with a symbol,
`GCC_except_table_xoN` (as clang names them), so each table is its own
atom (`internal/arm64/emit.go` lsdaLabel). ELF keeps `.L` labels. When a
new section of data is referenced from unwind information, give each
entry a symbol on Mach-O.
