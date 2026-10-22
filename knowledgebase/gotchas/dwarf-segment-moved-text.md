# The __DWARF segment's command moved the code of debug builds

Status: Observed (2026-10-22)

Symptom: internal/arm64 TestDebugInfoSameCode (decision 0252's gate:
debug information does not change code generation) failed: the default
build and the `-s` build of every golden program had the same bytes of
code at different addresses, `__text` at 0x100000948 with debug
information and 0x100000680 without, so `otool -t` differed on every line.

Cause: decision 0346 stage 1 (ead761c21) added the `__DWARF` segment's
load command (72 bytes and 80 per section: 712 with the eight DWARF
sections) to debug builds only, and compiler/macho link.xo layout
started `__TEXT`'s sections right after the load commands, so the code
of a debug build sat 712 bytes later and every adrp and adr changed.

Fix: layout starts `__TEXT`'s sections after room for that command with
all eight sections, written or not; a build without debug information
leaves the room as zeros (712 bytes). A new load command that only some
builds write needs the same reserved room, or the code moves with it.
