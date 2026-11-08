# hollow_scan read a long assembly line once per byte (ELF)

Status: Observed (2026-11-07, Xo Lang 4)

Symptom: `xo build --targets=linux/arm64` of a program with a 20 MB
string constant ran for over 10 minutes at one CPU, its last phase line
`fn_store`, with `arm64.hollow_scan` and `ident_byte` on top of a
sample. The same program builds in 3 seconds now.

Cause: compiler/arm64 hollow.xo `hollow_scan` looks for the symbols each
line of a part's assembly names. On ELF it also tries the program's own
non `xo_` symbols (`elf_extra_at`), which reads to the end of the run of
name bytes at that position, and it tested `!ident_byte(b[k - 1])` (a
name starts here) only after. A string constant's `.ascii` line is one
long run of name bytes (letters and digits), so every byte read the rest
of the run: quadratic in the line's length.

Fix: test that a name can start at k first, so a run is read once.
compiler/arm64 hollow_test.xo "on ELF a long string constant's line is
scanned once" (4 MB on one line) hangs without it. Watch for the same
shape in other per-line scanners of the assembly text: a check that reads
ahead must come after the cheap one that rules the position out.
