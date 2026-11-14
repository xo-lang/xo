# Mach-O arm64 relocations reach only 8 MiB past a section symbol

Status: Observed (Xo Lang 1, 2026-11-14).

## Symptom

A darwin/arm64 native build of a program with more than about 8 MB of constant data in one
section (2200 string literals of 5000 bytes) failed in xo's assembler:

    xo: assembler: line 72369: addend 8391792 does not fit ARM64_RELOC_ADDEND

A large generated table in an SDK hits it. The same program built with the shared back-end cache
passed: with that cache's separately compiled std the build took another path, so a check of the
fix has to build with a back-end cache of its own (`XO_BACKEND_CACHE_DIR`).

## Cause

A reference to a temporary label (`Ltmp`, `.L...`) is relocated against its section's symbol
with the label's offset as the addend. Mach-O arm64 carries an addend in an
`ARM64_RELOC_ADDEND` entry, whose 24 bit field holds -8 MiB to 8 MiB - 1, so a label further
into its section has no encoding.

## Fix

compiler/macho asm.xo reloc_to: when the addend does not fit, the relocation names the label
itself with addend 0, as Apple's toolchain names a local symbol. The label is written as a local
symbol marked `N_ALT_ENTRY` (n_desc 0x200), so the linker keeps it inside the atom it falls in,
and strip.xo's atomize skips alt-entry symbols, so it does not split the section there.
internal/arm64 TestConstDataPast8MiB builds and runs an 11 MB program (it fails without the fix).

Other formats: ELF's RELA addend is 64 bits. COFF arm64 already had the same limit at 1 MiB
(ADRP's in-place 21 bit field) and handles it as LLVM does (coffa64.xo coff_a64_labels: a
`$L<section>_N` label at every 1 MiB); the 11 MB program cross builds for windows/arm64 and
linux/arm64.
