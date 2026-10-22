# ld64 leaves no room for another load command

Status: Observed (2026-10-22)

Symptom: adding a segment to an executable ld64 linked (decision 0346
stage 3: the `__DWARF` segment after the link) has nowhere to put its
load command: hello's commands ended at byte 1720 and `__text` began at
1752, 32 bytes free, while a segment command with 8 to 11 sections takes
712 to 952 bytes.

Cause: ld64 places `__text` right after the load commands plus a small
pad unless told otherwise; Xo's own Mach-O linker leaves a page.

Fix: the cc links that embed their DWARF pass `-Wl,-headerpad,0x1000`
(compiler/xo link.xo DWARF_HEADERPAD), which is also in the binary
cache's key; macho.embed_dwarf checks the room and says to link with
-headerpad when it is short, and the dSYM is then kept instead.
