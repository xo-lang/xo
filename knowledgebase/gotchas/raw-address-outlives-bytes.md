# A raw address into a Bytes outlives the Bytes

Status: Observed (2026-10-16)

## Symptom

On macOS with the LLVM backend, `XO_TRACEBACK=1` printed the program's
main frame with another file's line (`main (prof.xo:2327)`, or no line
at all before decision 0239's inline reader), and missed inlined frames
that lldb showed. The result changed from build to build of the same
program: some builds were right. Linux and the arm64 backend happened to
be right.

## Cause

std/native/llrt/dwline.xo reads the debug sections through raw
addresses (`raw.bytes_addr(data)` plus offsets) into the Bytes the file
was read into. Nothing used `data` after the addresses were taken, so it
was released at its last use, before the line program and .debug_info
were parsed; the parsers' own allocations (lists, maps) then reused that
memory, and the reader parsed overwritten bytes: rows with file numbers
past the file table, which it dropped, and ranges that were not the
program's. A faithful Python parse of the same dSYM (scratch, not kept)
read the whole table, which pointed at the reader's input, not its logic.

## Fix

Hold the Bytes for as long as the addresses are used: `Sects.data` keeps
it, and the parsers take the Sects. Any code that keeps `raw.bytes_addr`
(or `rt.obj_addr`) results past the object's last named use must keep a
reference the same way.

The audit of 2026-10-16 (bench/results/raw-address-audit-2026-10-16.md)
lists every raw address user in std/native and selfhost and fixed 15:
call `raw.alive(b)` (or `raw.alive_str(s)`) after the last raw access,
which keeps the value live to that point. Releases go after the last
use even inside one call's arguments: `f(raw.data(s), s.len())` releases
an owned `s` before `f` runs.
