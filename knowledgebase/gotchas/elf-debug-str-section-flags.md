# clang's assembler requires .debug_str and .debug_line_str to be "MS",@progbits,1

Status: Observed (2026-10-22)

Symptom: a linux/arm64 program with `use c`, built by xo to assembly and
assembled by clang, failed with `error: changed section flags for
.debug_str, expected: 0x30` and `changed section entsize for .debug_str,
expected: 1` at every `.section .debug_str,"",@progbits` (and the same for
.debug_line_str). Programs that xo assembles and links itself were fine.

Cause: LLVM's ELF assembler creates .debug_str and .debug_line_str as
mergeable strings (SHF_MERGE | SHF_STRINGS, entry size 1) and rejects a
`.section` directive that names them with other flags. compiler/arm64
debug.xo has emitted .debug_str (the path slots) since a93ba88ff with
flags "", and .debug_line_str since b8c6fd647.

Fix: debug.xo writes `.section .debug_str,"MS",@progbits,1` (and the same
for .debug_line_str) on ELF. Xo's own assembler accepts the form (it
already did for .rodata.str1.1). Check a new ELF debug section's flags
against what clang gives it (`clang -g -S` on a C file shows them).
