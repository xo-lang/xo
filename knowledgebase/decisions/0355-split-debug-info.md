# 0355: split debug information (`--debug-info=split`)

Status: Proposed (owner-approved item, lead, 2026-10-23)

## Context

A release build carries no debug information; a debug build carries all
of it in the binary. The owner wants the release binary small and
stripped of DWARF with its symbols in a separate file that debuggers,
profilers, llvm-symbolizer, `xo dev symbolize` and the crash report reader
find by an id: `xo build --release --debug-info=split`. The flag spelling
follows Xo Lang 2's --release presets ADR (`--debug-info=on|off|split`).

## Decision (to be filled in with the code)

- darwin: the binary without DWARF, PROG.dSYM beside it, matched by the
  Mach-O UUID (the dSYM writer exists, --dsym).
- linux: the binary without .debug_* sections, with a GNU build-id note
  (.note.gnu.build-id) and .gnu_debuglink naming PROG.debug, an ELF with
  the DWARF sections and the same build id (Xo's ELF linker).
- windows: the PDB as today (always split).
- `xo dev symbolize` and the crash report reader look for PROG.debug /
  PROG.dSYM by id.

## Consequences

To be measured: binary sizes with and without split, tests that a split
binary has no debug sections and that lldb and llvm-symbolizer resolve a
function through the separate file.

## Alternatives

Split DWARF units (.dwo/.dwp: compile-time splitting, a different
mechanism); stripping after the link with external tools (needs binutils
or llvm-objcopy on the user's machine).
