# 0344: A PDB and CodeView writer for the PE linker

Status: Accepted for stages 1 to 4, stage 5 (lld-link) deferred (Xo Lang 4,
2026-10-22; the owner approved building the writer the same day, after
decision 0343's crash report). Stage 1
implemented 2026-10-22 (compiler/macho pdb.xo): llvm-pdbutil 19 dumps
every stream without an error and finds globals by name through the GSI
hash; llvm-symbolizer names functions from the PDB alone. Measured on
xo.exe (140 MB): a 1.7 MB PDB, 6,996 procedures and 14,638 publics, and
the link phase within its run to run noise (0.9 to 1.4 s with and
without).
Stage 2 implemented 2026-10-22: DEBUG_S_LINES per function and
DEBUG_S_FILECHKSMS, /names with its hash table, the DBI file info. A
function's rows are its own: code of a call inlined into it takes the
call's line (stage 3 describes the inside), and only the file most rows
name is kept, since LLVM's reader (SymbolCache::findLineTable), as
MSVC's tools, opens every file block at the function's start. Over 400
random addresses of a program, llvm-symbolizer from the PDB alone gives
DWARF's outermost frame's function and file:line at 266 of 268 (the two
others: code of another file with no DW_TAG_inlined_subroutine, where
the PDB keeps the function's own line).
Stage 3 implemented 2026-10-22: S_INLINESITE and S_INLINESITE_END
nested in each procedure, LF_FUNC_ID records in the IPI stream (with its
hash stream: JamCRC of each record modulo the buckets, as LLVM's
writer), DEBUG_S_INLINEELINES. Over the same 400 addresses llvm-symbolizer
from the PDB alone gives DWARF's whole inline chain (each frame's
function, file and line) at 265 of 268, and Wine's dbghelp (scripts/
pdbframes) prints a crash report's traceback, inline frame included,
exactly as the runtime does from the DWARF. Two traps: ChangeCodeOffset
is opcode 3 (2 is ChangeCodeOffsetBase, which LLVM's reader skips when
it looks for the site holding an address; llvm-pdbutil prints both as
"code"), and a site's first entry needs a line operation even when the
line does not change (LLVM's reader takes the site's line from it and
drops a frame without one, and every frame after it).
Stage 4 implemented 2026-10-22 (compiler/macho cvtypes.xo): the TPI
stream from the variables' DWARF types (base types as primitive indices,
a forward reference per structure name that pointers, arrays and
members name, then its LF_FIELDLIST and LF_STRUCTURE; the hash stream:
the name's hash_v1 for a full definition, JamCRC for the rest), and
S_LOCAL with S_DEFRANGE_REGISTER, S_DEFRANGE_SUBFIELD_REGISTER (a piece
of a value in several registers, as Str's pointer and length) and
S_DEFRANGE_REGISTER_REL from the DWARF location lists, for procedures and
inline sites. llvm-pdbutil resolves every forward reference to its
definition through the hash; Wine's dbghelp lists each frame's variables
with their parameter flags. Values in a debugger are cdb's `dv`, in the
next approved Windows run.
Measured on xo.exe: a 61 MB PDB (1.7 MB with stage 1's names only; the
location records are most of it) and a 1.9 s link phase. Each place's ranges are
now merged and written with CodeView gaps (a location list repeats a
stack slot in every entry where another piece moves): 20.7 MB, the
S_DEFRANGE_REGISTER_REL records from 2.1 million (42 MB) to 135,321,
the link phase 2.0 s. CI 39831734067
(stages 1 to 3 on real Windows): cdb loaded the PDB ("pdb symbols") and
named poke; Windows' own dbghelp, through scripts/pdbframes, found no
PDB. The debug directory named it with forward slashes (xo's paths),
and dbghelp finds a moved PDB by its base name; the record now uses
backslashes, as MSVC's and lld's linkers write it, and pdbframes loads
cdb's dbghelp when the runner has it.

## Context

Native windows/amd64 programs carry DWARF since 2026-10-22
(gotchas/windows-no-dwarf.md): Xo's PE linker (compiler/macho pelink.xo)
keeps the objects' .debug_ sections, and the program reads them itself
for its fault tracebacks and crash reports (decision 0343). gdb, lldb and
`xo dev symbolize` read them too. Windows' own tools do not:

- WinDbg, cdb and Visual Studio read CodeView records from a PDB (a
  multi-stream file next to the executable), found through the
  executable's debug directory (an IMAGE_DEBUG_DIRECTORY entry of type
  CODEVIEW whose RSDS record holds the PDB's GUID, age and path).
- Windows Error Reporting dumps and the minidumps of decision 0343's
  context are read with those tools. 0.25.1's xo.exe crash took lldb on
  the dump with hand found offsets because cdb named no Xo frame.
- Performance tools (WPA, ETW stack walks, VTune on Windows) symbolize
  from PDBs only.

The two link paths for windows/amd64 today:

1. The amd64 backend without `use c`: Xo's PE linker, no C runtime, no
   SDK (decision 0254 6b step 4). Writes a COFF symbol table
   (/debug:symtab's layout) and the DWARF, no PDB.
2. With `use c`, or the LLVM backend: clang and lld-link. lld-link would
   write a PDB with /debug, but its inputs carry DWARF, not CodeView
   (.debug$S, .debug$T), so the PDB would hold the publics only.

## Decision

Xo writes the PDB itself, converting the program's DWARF to CodeView
after the link, from the final executable's bytes. One converter serves
both link paths.

### Where

- compiler/macho pdb.xo: the MSF container and the streams.
- compiler/macho codeview.xo: the CodeView symbol and type records and
  the C13 debug subsections.
- The DWARF comes from compiler/debug (the readers of `xo debug`, `xo dev
  dwarf` and `xo dev symbolize`: sections, line tables, functions with
  their inlined calls, variables, types), read from the PE itself.
- compiler/xo: after the link, PROG.exe gets PROG.pdb next to it, and
  its debug directory names it. `--release` and `--strip` write none
  (they drop the DWARF the PDB is made from).

### The executable's side

The PE linker reserves, in .rdata, an IMAGE_DEBUG_DIRECTORY entry
(Type 2, CODEVIEW) and its RSDS record (signature "RSDS", a GUID, an age
of 1, the PDB's file name), and points data directory 6 at it. The GUID
is computed, not random: a hash of the image with the GUID field
zeroed. So a build is still reproducible (lld-link's /Brepro), and the
PDB and the executable agree by construction. For the lld-link path,
stage 5 checks whether lld-link reserves the record with `/debug:dwarf`
and `/pdb:`; if not, the converter adds it in a section of its own.

### The PDB

The MSF 7.0 container: 4096 byte blocks, the free block maps, and the
stream directory. Streams:

| Stream | Contents |
|---|---|
| 1, PDB info | version 20000404, the GUID and age, the named stream map (`/names`) |
| 2, TPI | type records (stage 4; a header only before) |
| 3, DBI | the module list (one module per DWARF compile unit), the section contributions, the section map, the file info, the optional debug header (the section header stream) |
| 4, IPI | LF_FUNC_ID and LF_STRING_ID records for the inlinees (stage 3) |
| modules | per module: its CodeView symbols (S_GPROC32, S_FRAMEPROC, S_END; S_INLINESITE; S_LOCAL and S_DEFRANGE_*) and its C13 subsections (DEBUG_S_LINES, DEBUG_S_FILECHKSMS, DEBUG_S_INLINEELINES) |
| publics, globals | S_PUB32 for every function and data symbol, with the GSI hash tables and the address map |
| /names | the string table of file names |
| section headers | a copy of the PE's section headers |

Names are Xo's as the traceback prints them (`poke`, `tasks.run_entry`,
the decoded `xo.run[Task[(), ()]]`), not the mangled symbol names, so a
debugger's `k` reads like the crash report.

### Stages

1. The container, PDB info, DBI with the module list and section map,
   publics and globals, the section headers, and the RSDS debug
   directory in Xo's PE linker: function names by address. Checked
   with `llvm-pdbutil dump --all` and `llvm-pdbutil pretty` (Docker,
   the xo-linux-test image) and with `llvm-symbolizer --obj=PROG.exe`
   on a copy whose DWARF is stripped, so only the PDB can answer.
2. Line tables: DEBUG_S_LINES per function from the DWARF line rows,
   DEBUG_S_FILECHKSMS (no checksums) and `/names`: file and line per
   address, breakpoints by line.
3. Inlined calls: S_INLINESITE with its binary annotations and the IPI's
   LF_FUNC_ID, from DW_TAG_inlined_subroutine: `k` shows inlined frames
   as the crash report does.
4. Locals and types: S_LOCAL with S_DEFRANGE_REGISTER,
   S_DEFRANGE_FRAMEPOINTER_REL and S_DEFRANGE_REGISTER_REL from the DWARF
   location lists, and TPI records (LF_STRUCTURE, LF_FIELDLIST,
   LF_POINTER, LF_PROCEDURE) for the types `xo debug` reads.
5. The lld-link path (`use c`, the LLVM backend): the same converter on
   lld-link's output. Deferred (owner, 2026-10-22): building and testing
   it needs a Windows SDK sysroot (xwin, accepting Microsoft's SDK
   license again, which the owner declined), Windows releases use the
   native PE linker, and the LLVM Windows recipe does not ship. Taken up
   when the LLVM Windows path is wanted.

Each stage is checked on Windows with cdb in one CI run on the
xo-windows runner (`k`, `ln`, `.lines`, `bp FILE:LINE`, `dv`), asked
for through the lead, as decision 0225 requires.

## Consequences

- WinDbg, cdb, Visual Studio and ETW tools name Xo frames, lines,
  inlined calls and (stage 4) locals; WER dumps can be read with cdb.
- A debug build of a windows/amd64 program writes one more file. Its
  size is measured in stage 1 against hello and xo.exe.
- The link does more work (reading the DWARF back and writing the PDB);
  measured in stage 1, and skipped when nothing reads it (--release).
- The PDB format has no standard: the references are LLVM's
  documentation (llvm.org/docs/PDB), its reader (llvm-pdbutil), and
  Microsoft's microsoft-pdb repository. A layout mistake shows up as a
  tool that silently finds nothing, so every stage is checked by a
  reader, not by inspection.

## Alternatives

- **lld-link /debug only**: needs lld-link and the SDK, which Xo's own
  PE linker exists to avoid, and gives publics only (the inputs carry
  DWARF).
- **Emit CodeView from the backend (.debug$S, .debug$T) and let a linker
  merge it**: a second debug info emitter beside the DWARF one, kept in
  step with it; and Xo's PE linker would still need the PDB writer.
- **DWARF only**: gdb and lldb read it on Windows, but WinDbg, Visual
  Studio, WER triage and ETW do not; this is what 0.25.1's crash cost.
- **A PDB with publics only (stage 1 alone)**: names without lines; a
  first step, not the goal.
