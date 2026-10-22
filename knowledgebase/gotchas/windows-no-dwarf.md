# Native windows/amd64 builds carry DWARF, not CodeView

Status: Observed (2026-10-20, Xo lang 4); DWARF kept since 2026-10-22

Symptom: a windows/amd64 build of the native backend has no line tables
or variables for a debugger, where linux/amd64 and darwin/amd64 builds
have DWARF (since 2026-10-20).

Cause: Windows debuggers (WinDbg, Visual Studio) read CodeView records
and a PDB, not DWARF; Xo's COFF writer and PE linker (compiler/macho
coff.xo, pelink.xo) write neither, and were never fed .debug_ sections.
compiler/arm64 gen.xo new_gen keeps m.debug_info off for x86_64 on
windows, so nothing new reaches them.

Fix: none yet. A Windows fault report still names its frames (the
unwinder reads the PE's unwind data), without lines or locals. CodeView
and PDB output would be its own decision.

Since 2026-10-22 (Xo Lang 2): compiler/arm64 gen.xo emits DWARF for
windows/amd64 too, the COFF writer names a section past 8 bytes through
the string table and makes 4 byte references in a .debug_ section
SECREL, and pelink.xo keeps the .debug_ sections (discardable, after
.reloc, "/N" names, no base relocations for their addresses). The
program reads them itself (std/native/llrt dwline.xo pe_sects, from its
file): fault tracebacks (XO_TRACEBACK=1) show file:line and inlined
frames, and --race reports their lines. llvm-dwarfdump --verify passes on
the PE. Windows debuggers still want CodeView and a PDB; gdb and lldb
read the DWARF. Since decision 0344 (2026-10-22) Xo's PE linker writes
PROG.pdb from that DWARF (function names first, then lines). A debug build is larger (tsan_race.exe 0.9 to 6.8 MB,
mostly .debug_loclists); --release or --strip leaves it out.
