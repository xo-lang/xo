# The self hosted ELF assembler rejected the linux emitter's `.file`

Status: Observed (2026-10-12, scripts/release.sh run 1 of the switch
point B flip; bench/results/retire-go-2026-10-12.md "Switch point B: the
flip").

## Symptom

`xo1 build --backend=arm64 --target=linux/arm64 --ld=macho selfhost/xo`
(the linux/arm64 release binary) stopped after 1,000 s with
`xo1: assembler: line 2: .file in __text`. Decision 0182's stage 3
results had assembled the same program's linux assembly byte identical
to clang a few hours earlier.

## Cause

Commit 15b3a46 (Linux fixes) made both arm64 emitters write
`.file "prog.s"` as the second line of linux assembly
(gotchas/linux-arm64-file-symbol.md: a fixed STT_FILE name keeps GNU ld
links reproducible). It landed after stage 3 was measured, and the only
path that assembles linux assembly in Xo (`--as=macho` / `--ld=macho`)
was not in any merge-tier test, so nothing saw the new directive reach
selfhost/macho, whose ELF mode had no `.file`.

## Fix

selfhost/macho's ELF mode takes `.file "NAME"`: an STB_LOCAL STT_FILE
symbol in SHN_ABS, first in the symbol table after the null symbol, as
LLVM writes it (commit 9d8fac8; the macho Xo test "a .file directive is
an STT_FILE symbol first in the symbol table"; the hello program's object
byte identical to `clang --target=aarch64-linux-gnu -c` by
scripts/elf-compare.py). When an emitter gains a directive, assemble its
output with `macho asm --elf` (scripts/elf-compare.py) in the same
change.
