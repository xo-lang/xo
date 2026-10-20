# Native windows/amd64 builds carry no debug information

Status: Anticipated (2026-10-20, Xo lang 4)

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
