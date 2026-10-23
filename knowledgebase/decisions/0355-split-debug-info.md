# 0355: split debug information (`--debug-info=split`)

Status: Accepted (owner-approved item, lead, 2026-10-23; implemented 2026-10-23)

## Context

A release build carries no debug information; a debug build carries all
of it in the binary. The owner wants the release binary small and
stripped of DWARF with its symbols in a separate file that debuggers,
profilers, llvm-symbolizer, `xo dev symbolize` and the crash report reader
find by an id: `xo build --release --debug-info=split`. The flag spelling
follows Xo Lang 2's --release presets ADR (`--debug-info=on|off|split`).

## Decision

`--debug-info=split` (the 0354 setting, read through `debug_info_eff`)
builds with DWARF and writes it beside the program, not in it:

- darwin: Xo's Mach-O linker links with no `__DWARF` segment
  (`embed_dwarf: false`) and writes PROG.dSYM (its existing dSYM
  writer, `dsym: true`), matched by the program's LC_UUID. A cc link
  (`use c`, `--ld=cc`) and the LLVM backend keep dsymutil's bundle
  instead of embedding it (cc_dsym's split). Works from any host for
  Xo's linker.
- linux: Xo's ELF linker (compiler/macho elfsplit.xo, link_elf_split)
  writes the program without `.debug_*` sections, with a
  `.note.gnu.build-id` (20 bytes of SHA-256 over the loaded sections,
  the DWARF and the symbols: the same link gives the same id) and a
  `.gnu_debuglink` naming PROG.debug with its CRC-32. PROG.debug is an
  ELF with the loaded sections as SHT_NOBITS at their addresses, the
  same note, `.symtab`, `.strtab` and the DWARF (as `objcopy
  --only-keep-debug` writes). Both are non-loaded sections at the end of
  the file; the program keeps its `.symtab` (unless `-s`). A link
  through cc or clang on linux (`use c`, `--ld=cc`, the LLVM backend)
  is an error with split, as are wasip1 and `--backend=go`.
- windows: the PDB as today.
- Readers: the runtime's traceback and locals (std/native/llrt dwline.xo)
  read PROG.dSYM by UUID (as before) and PROG.debug through the
  debuglink in the executable's directory when its build id matches;
  `xo dev symbolize` does the same; the binary cache keeps and restores
  PROG.debug beside an entry as it does the dSYM's DWARF; a build
  without split removes a stale PROG.dSYM or PROG.debug.

## Consequences

Measured 2026-10-23 (a small linux/arm64 program, Xo's ELF linker): the
program 691,872 bytes plus PROG.debug 1,354,352, against 1,977,960 bytes
unsplit. Without split no output changes: six linux/arm64 and amd64
binaries byte-identical with the same std before and after the linker
change. Verified in Docker linux/arm64: the runtime's fault traceback
names `main.xo` lines through PROG.debug (names only once the file is
gone), and `llvm-symbolizer --obj=PROG` and `addr2line -e PROG` resolve
a function to its line through it; `llvm-dwarfdump --verify PROG.debug`
passes. On darwin: no `__DWARF`, the dSYM's UUID is the program's, the
traceback names lines, lldb `image lookup` resolves through the dSYM.
Tests: internal/arm64 TestDebugInfoSplitDarwin and
TestDebugInfoSplitLinux, compiler/macho's split link and crc32 tests.
Not done: the build id note in unsplit linux programs (core dumps and
debuginfod would use it; a later step), and a PT_NOTE for it.

## Alternatives

Split DWARF units (.dwo/.dwp: compile-time splitting, a different
mechanism); stripping after the link with external tools (needs binutils
or llvm-objcopy on the user's machine).
