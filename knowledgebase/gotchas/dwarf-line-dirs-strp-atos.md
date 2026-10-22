# A line table path that is not DW_FORM_line_strp breaks atos and --race symbolization

Status: Observed (2026-10-22)

Symptom: after a93ba88ff (the back-end cache's path slots), a darwin
arm64 `--race` report's stacks showed no file and line for Xo frames, and
`atos -o PROG.dSYM` printed "Attempt to set cursor outside bounds" for
every address (TestRaceStacksArm64: the report lacked `xo_f_touch
main.xo:`). lldb and Xo's own crash reader (std/native/llrt dwline.xo)
still resolved the same addresses.

Cause: the DWARF 5 line table header named its directories with
DW_FORM_strp (0x0e) into `__debug_str`, where the path slots live.
CoreSymbolication, which atos and the ThreadSanitizer runtime use on
macOS, appears to resolve a line table's strings only from
`__debug_line_str` (DW_FORM_line_strp, 0x1f, what clang emits) and
fails the unit on a strp offset (inferred from the bisect below, not
from its source). Zeroing `__debug_line` alone in the dSYM removed the
error; the parent of a93ba88ff (DW_FORM_string directories) resolved
main.xo:21.

Fix: compiler/arm64 debug.xo writes the directories as
DW_FORM_line_strp into a `.debug_line_str` (`__DWARF,__debug_line_str`)
section holding a second copy of the two path slots; the compile unit's
comp_dir stays DW_FORM_strp into `.debug_str`. `dw_slots_obj` fills both
copies; compiler/macho dsym.xo joins `__debug_line_str` and rebases the
directory offsets (line header bytes 34 and 38) by its chunk. Check a
change to the line table header with atos on a dSYM, not only lldb.

## File names: DW_FORM_string crashes macOS 26's atos

Observed 2026-10-22, on macOS 26.5.2 (the owner's Intel laptop) while
checking `--race` on darwin/amd64: atos exited with SIGSEGV on every Xo
executable or dSYM with DWARF (arm64 and amd64 alike; `-s` builds and C
programs were fine), so ThreadSanitizer's reports printed `<null>` for
every frame and "atos failed to symbolize". macOS 27's atos read the
same files. The crash report (/Library/Logs/DiagnosticReports/atos-*.ips,
not the user's folder) named CoreSymbolication's
`parse_dwarf5_path_list` → `TDwarfStream::FORM_string()`, a read at
address 8. Renaming one debug section at a time in the dSYM showed it
needed both `__debug_info` and `__debug_line`; renaming any one DIE
attribute or the inlined subroutine tag changed nothing.

Cause: the line table's file names were DW_FORM_string after
DW_FORM_line_strp directories; macOS 26's path list parser appears to
have no stream for inline strings in that position.

Fix: the file names are DW_FORM_line_strp too, after the two slots in
`.debug_line_str` (as clang writes them); dsym.xo rebases each name's
offset with the directories'. internal/arm64 TestDebugLineDarwin
(checkLinePaths) fails on any inline path in a line table. Test atos
changes on the oldest supported macOS too: a newer atos hides this.
