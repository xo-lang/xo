# Linux LLVM binaries fail llvm-dwarfdump --verify in .debug_str_offsets

Status: Observed (2026-10-17, main 63aa7f9a and later; not yet fixed)

Symptom: `llvm-dwarfdump --verify` (Debian LLVM 19.1.7, in
scripts/docker-linux.sh's image) on a linux/arm64 program built with
`--backend=llvm` reports errors like `.debug_str_offsets: contribution
X: index X: invalid string offset *X == XC, is neither zero nor
immediately following a null character`, about a dozen for a small
program. The same program from the arm64 backend verifies; macOS LLVM
builds verify (dsymutil rebuilds the string tables). gdb and lldb still
read lines, frames, and (decision 0252) variables.

Cause (not yet known): the errors appear with both links (Xo's elflink
and `--ld=cc`), so they come from the objects, not from joining
sections; they predate decision 0252's variables (main 63aa7f9a has
them with line tables only). Candidates: the split build's parts each
writing their own string offsets table that the link joins without
moving the offsets (the DW_AT_str_offsets_base of each unit), or the
LTO object's tables.

Next: build one part's object and verify it alone (`llvm-dwarfdump
--verify part.o`); if it verifies, the join is wrong (look for
.debug_str_offsets relocations against .debug_str in the objects:
`llvm-readelf -r`); if not, check clang's flags for the split parts.
