# Linux LLVM binaries fail llvm-dwarfdump --verify in .debug_str_offsets

Status: Observed (2026-10-17); cause found: a false positive of the
verifier on tail merged strings, not a defect of the DWARF

Symptom: `llvm-dwarfdump --verify` (Debian LLVM 19.1.7, in
scripts/docker-linux.sh's image) on a linux/arm64 program built with
`--backend=llvm` reports errors like `.debug_str_offsets: contribution
X: index X: invalid string offset *X == XC, is neither zero nor
immediately following a null character` (166 for a hello program
without LTO, 138 with). The same program from the arm64 backend
verifies; macOS LLVM builds verify (dsymutil rebuilds the string
tables). gdb and lldb read lines, frames, and variables.

Cause: the link merges the string section's suffixes. `.debug_str` is
SHF_MERGE|SHF_STRINGS, and GNU ld (the C compiler's linker, which Linux
LLVM builds link with) shares a string's tail with a longer string's
end, so a name's offset can point into the middle of a longer string.
That is valid DWARF; the verifier wrongly expects every string to
follow a NUL. Evidence (2026-10-17, the hello program's IR from its
build directory): clang's object verifies clean; ld.lld linking it alone
verifies clean; ld.bfd linking it alone gives the 166 errors, and so
does ld.lld -O2 (which also merges tails); every DW_AT_name of the
ld.bfd output equals the ld.lld output's (3,469 names). It does not
predate or depend on decision 0252's variables, the split build, or LTO.

Fix: none needed. To verify a Linux LLVM binary's DWARF, link the same
objects with ld.lld (no -O2, so no tail merging) and verify that, or
compare the names and locations llvm-dwarfdump prints against such a
link. Do not treat these errors as a regression. The DWARF reader of
`xo debug` (selfhost/debug) reads a string at any offset, as gdb and lldb
do.
