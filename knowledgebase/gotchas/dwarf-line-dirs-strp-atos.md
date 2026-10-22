# A line table whose directories are DW_FORM_strp breaks atos and --race symbolization

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
