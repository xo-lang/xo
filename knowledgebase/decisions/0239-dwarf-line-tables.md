# 0239: DWARF line tables from the native backends

Status: Accepted (owner, 2026-10-16). Implemented 2026-10-16 (see "Implementation" below).

## Context

Native Xo binaries carry symbols and unwind tables (eh_frame, macOS
compact unwind) but no DWARF debug information, so every external tool
(lldb, Instruments, `sample`, perf, callgrind, crash reports) maps an
address to a function at best, never to a `.xo` file and line. Decision
0238's profiler has the same limit. A source level debugger (a Delve for
Xo, parked in the consider-later list) needs line tables first, then
types and variable locations.

## Decision

1. Both native backends emit DWARF 5 line tables (`.debug_line`, with
   `.debug_info` compile units and subprogram entries carrying low_pc
   and high_pc, and `.debug_abbrev`, `.debug_str`, `.debug_line_str`)
   mapping each instruction range to its `.xo` file, line and column:
   - LLVM: the emitter attaches DILocation metadata from the SSA
     positions (the positions the checker's diagnostics already use),
     plus DICompileUnit and DISubprogram; LLVM writes the sections.
   - arm64: the emitter records a (pc offset, file, line, column) row
     whenever the position changes, and writes `.debug_line` itself;
     selfhost/macho carries the sections into the Mach-O `__DWARF`
     segment, and cc links keep them (a dSYM with `dsymutil` when asked).
2. On by default (it costs file size, not run time): `xo build -s`
   strips them, and the release profile (decision 0099) strips unless
   `--debug-info` is given. The test binary cache and the backend cache
   key include the choice.
3. Inlined code reports the line of the inlined callee, with
   DW_TAG_inlined_subroutine entries on LLVM; arm64 starts with the
   caller's line and adds inline entries later.
4. Types, variables and locations (`.debug_info` beyond subprograms)
   are a later decision, with the debugger.

## Consequences

- lldb breaks and steps by `.xo` line; `sample`, Instruments, perf,
  callgrind and the 0238 profiler name lines; fault reports can print
  `file:line` for each frame.
- Binaries grow (estimate 10 to 25%, measured in the implementation's
  results file); release builds strip by default.
- Every codegen change must keep positions attached; a golden test
  checks `llvm-dwarfdump --debug-line` (or a Xo reader) for a known
  program on both backends, and a fault golden checks `file:line`.

## Alternatives

- A private line table section read only by Xo tools: no external tool
  understands it.
- Source maps from symbol names only: no line or column.
- Full DWARF (types and variables) at once: much larger; line tables
  deliver most of the value for profiling first.

## Implementation (2026-10-16)

- SSA: every value has a packed source line (ssa Value.line, line *
  4096 + column) from the statement or block value it was lowered from,
  and each function its file (Func.file); the arm64 inliner gives copied
  values the call's line. The rc cache (v2) keeps both.
- arm64: instructions inherit the line (Inst.line); emission places a
  label where the line changes and records the function's rows
  (function cache v2 keeps them, with the text's tokens); debug.xo
  writes per object a DWARF 5 .debug_line (a sequence per function,
  rows by DW_LNE_set_address with `.quad label`), .debug_abbrev, and
  .debug_info (a compile unit, producer "xo", with stmt_list, low_pc,
  and high_pc at fixed places, and a subprogram per function). Only
  absolute relocations; every length is counted by the emitter.
- selfhost/macho on darwin: the executable carries no DWARF (as ld64);
  the link writes PROG.dSYM/Contents/Resources/DWARF/PROG (MH_DSYM, the
  executable's LC_UUID, its segment and section headers without
  contents, and __DWARF with the objects' sections joined, addresses
  relocated, each xo unit's offsets moved, and a synthesized
  .debug_aranges, without which lldb does not map addresses to units).
  Only the arm64 emitter's objects: another compiler's Mach-O DWARF
  refers between sections by offsets in its own object.
- selfhost/macho on Linux (elflink): every input .debug_ section is kept,
  joined, relocated (a symbol in a debug section is its object's offset
  there), and written as an unloaded section, with .debug_aranges
  added; cc links (lld) keep them by themselves.
- LLVM: a DICompileUnit (line tables only), DIFiles, a DISubprogram per
  function with a file (and xo_entry, which main is inlined into), and
  an inline DILocation on every instruction of those functions.
- Flags: on for build, run, and test; off for `xo asm`, with `-s`, and
  for `--release` unless `--debug-info`.
- Not done: a dSYM for darwin cc links (`dsymutil PROG` by hand; the cc
  link does not work on the development Mac, 2026-10-16), arm64
  DW_TAG_inlined_subroutine (point 3's later part), and fault reports
  printing file:line.

