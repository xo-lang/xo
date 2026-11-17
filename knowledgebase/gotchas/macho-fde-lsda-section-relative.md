# Apple ld misreads an FDE's LSDA pointer to an `L` label

Status: Observed (2026-11-16, task #189). Last verified: 2026-11-16 (ld 27106.1, ld-prime).

## Symptom

On darwin/arm64, a std/sqlite test whose open fails (`os.fs.sub("/nodir")`)
died with `SIGBUS: bus error` at an odd, small address (image base plus
0xe3dd); the crash report itself faulted. Tests that opened a database
passed. `dwarfdump --eh-frame` on the linked binary showed FDE LSDA
addresses below the start of `__gcc_except_tab`.

## Cause

Since 813806413 (2026-10-25) the arm64 backend named each function's
call site table with an assembler-local `L` label (`Lxo_f12_lsda`). An
assembler turns a reloc against an `L` label into one against the
section's start plus an offset (an `ltmp` symbol or a section reloc).
Apple's ld-prime mishandles that for the pc relative LSDA pointer of an
FDE in DWARF mode (a frame compact unwind cannot describe, such as one
that saves w19 at CFA-88): it negates the offset in the FDE and drops it
from `__unwind_info`'s LSDA index. The personality routine then read
another function's call site table and jumped to a wrong landing pad.
Only frames in DWARF mode with a table not first in its section are hit,
so most programs never threw through one. clang avoids it because its
C++ output names tables `GCC_except_tableN`, real symbols.

Repro (no xo): `~/xo-lead/notes/xo-lang-1/t189/fde/a.s` (two DWARF FDEs
with `L` LSDA labels) linked with `cc` gives LSDA 0x...480 for a section
at 0x...490; the same file with `l` labels gives 0x...4a0.

## Fix

On Darwin the label is linker private (`l` prefix, `emit.xo`
lsda_label): it is a symbol in the object, so the reloc names it, and
the linker drops it from the binary (no symbol table cost, which was the
reason for the 2026-10-25 change). compiler/macho's assembler also
renames an `L` LSDA label it is given to an `lxo_exceptN` symbol
(cfi.xo). Tests: arm64 "the darwin call site table label is linker
private", macho "a DWARF FDE's LSDA reloc names its own symbol".

Check a binary: every `LSDA Address` from `dwarfdump --eh-frame` lies in
`__gcc_except_tab` (`otool -l`).
