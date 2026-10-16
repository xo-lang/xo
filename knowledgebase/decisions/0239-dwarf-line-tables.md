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
- darwin cc links (the LLVM backend's, arm64 `--ld=cc`; 2026-10-16):
  the driver runs `dsymutil PROG -o PROG.dSYM` after the link (link.xo
  cc_dsym; the Command Line Tools have dsymutil), with ld64's
  `-object_path_lto` keeping an LTO link's object in the build
  directory so its DWARF is there to gather. No dsymutil, or a failing
  one, leaves the program without a dSYM. 0.15 s against the 49 s link
  of the LLVM http_stream golden. The binary cache keeps a dSYM's DWARF
  beside its entry (ENTRY.dsym) and writes it back on a hit, for macho's
  dSYM too, and removes a dSYM left from another link of the same path.
  internal/arm64 TestDebugLineDarwinCC checks LLVM, --ld=cc and macho,
  first build and cache hit: dwarfdump's line table and --verify, and
  lldb resolving `main.xo:9`.
- Not done: arm64 DW_TAG_inlined_subroutine (point 3's later part:
  Xo lang 5 writes it in selfhost/arm64, Xo Lang 3 carries it through
  selfhost/macho dsym.xo and tests it).
- LLVM binaries are larger than the estimate (TLS client +39% on Linux;
  bench/results/dwarf-line-tables-2026-10-16.md); release builds strip.

## Readers (2026-10-16, second step)

- std/native/llrt/dwline.xo reads the line tables back (v4 and v5,
  every opcode): Linux's .debug_line of /proc/self/exe, darwin's
  PROG.dSYM when its LC_UUID is the image's.
- The profiler (decision 0238, prof.xo) fills pprof's Line.line and
  Function.filename (`go tool pprof -list`, `-lines`) and the text
  summary's hottest line per function; a leaf pc is recorded plus 1, as
  Go's profiles do, since the tools read every location as a return
  address (a leaf at a function's first instruction was charged to the
  function before it).
- Fault reports (lead's choice, 2026-10-16): opt in with XO_TRACEBACK
  (spec core 8): `traceback:` and `  FUNCTION (FILE:LINE)` per frame
  down to main, recorded when the fault is raised by a walk of the
  unwind tables (unwind.xo trace; Linux LLVM code has no frame
  records); the one line report and the goldens are unchanged. `xo
  test` sets it for its tests. The arm64 fault stubs have their own
  rows (the faulting instruction's line).

## arm64 inlined frames (2026-10-16, third step)

- The arm64 inliner records each call it copies a callee in at (ssa
  Func.inl_sites: callee, its file, the call's line, the enclosing
  site) and gives copied values the callee's own line and that site
  (Value.inl), instead of the call's line.
- debug.xo: rows carry their site's file (DW_LNS_set_file per row);
  .debug_info has an abstract DW_TAG_subprogram per inlined callee
  (name, DW_AT_inline) and, under each function's subprogram (now with
  children), a DW_TAG_inlined_subroutine per run of an outermost site's
  code (abstract_origin, low_pc and high_pc labels, call_file,
  call_line, call_column). v1 is one level: nested sites belong to their
  outermost call's entry. lldb shows `scale at main.xo:2:3 [inlined]`
  above `main at main.xo:9:5`. Addresses stay `.quad label` and the
  references unit relative, so selfhost/macho dsym.xo carries them
  unchanged (Xo Lang 3 writes the tests).
- The function cache's key (fcache.xo fn_key) now holds every value's
  line and site, the function's file, and its sites when debug
  information is on; before this, a change of lines alone reused an
  entry with the old rows (gotchas/fcache-key-without-lines.md).
- Not yet: the profiler's and tracebacks' frames in inlined code still
  name the enclosing function (dwline.xo reads only the line table),
  with the callee's line.

