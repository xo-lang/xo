# Two LLVM builds of one program differ (darwin)

Status: Observed (2026-10-20), open: not fixed yet.

## Symptom

Two `xo build --backend=llvm` of the same hello program on the Mac, all
caches off (XO_BACKEND_CACHE=off), to the same output path, differ in
81 bytes: the LC_UUID load command (16 bytes), one byte of __LINKEDIT's
symbol table, and two 32 byte runs in the ad hoc code signature.

## Cause

The link is clang driving ld64 with the build's object files: their
debug map stabs (N_OSO, type 0x66) hold each object's modification
time, so the symbol table differs; ld64's UUID hashes the output, so it
differs too; and the signature's page hashes cover both pages. The
native backends link with compiler/macho, which writes neither, so the
arm64 identity tests do not see this.

## Fix (to try)

ZERO_AR_DATE=1 in the environment of the clang link (ld64 then writes 0
for N_OSO times, as for archive members), or no debug map stabs when the
DWARF is in a separate .dSYM; then confirm two builds are byte
identical, and add that check for the LLVM backend (release signing
needs reproducible binaries).
