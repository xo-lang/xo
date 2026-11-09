# coff-compare.sh compared padded objects with llvm-mc

Observed 2026-11-09 (Xo Lang 2).

## Symptom

`scripts/coff-compare.sh` (Xo's windows/amd64 COFF objects against llvm-mc's on the goldens) reported every golden
as different, where it had reported 199 of 199 the same on 2026-10-19. Nobody ran it in between, so nobody saw it.

## Cause

Decision 0398 added jump padding to Xo's amd64 assembler (compiler/macho `assemble_coff_amd64`). llvm-mc does no such
padding, so the .text bytes, and every offset after them, differ. The script ran `macho asm` with the padding on.

## Fix

`macho asm --no-jump-pad` selects the raw assemblers (assemble_coff_amd64_raw, and the Mach-O and ELF ones), and the
script passes it: 644 of 644 the same with llvm-mc 23. Any byte compare against another assembler assembles without
Xo-only layout passes; a compare script that has not run since a layout change needs a rerun before its old result is
quoted.
