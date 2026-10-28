# A clang assembler flag does nothing for the native amd64 path

Observed 2026-10-28 (xo-lang-6). Status: Observed.

**Symptom.** sort1M on an i7 was assembled "with
`-mbranches-within-32B-boundaries`" to test Intel's JCC erratum, ran
no faster, and the erratum was ruled out
(bench/results/sort1m-x86-2026-10-27.md). The binary's hot loop still
had a fused `cmpq`/`jge` straddling a 32 byte boundary: nothing was
padded.

**Cause.** `xo build --backend=amd64` assembles its own text with Xo's
x86-64 assembler (compiler/macho asm_amd64.xo, decision 0254 stage 1)
and links with Xo's linkers; clang is not in that path (XO_CC and
XO_CLANG only matter for `use c` and the LLVM backend), so a clang or
LLVM assembler flag, a `-mllvm` option, or a cc wrapper changes
nothing in the native output. The run measured the same bytes twice.

**Fix.** A layout or encoding experiment on the native path changes the
assembler (asm_amd64.xo) or the emitter (x86emit.xo); check the bytes
(`otool -tV` / `objdump -d` of the hot function) before and after and
confirm the change is in them before timing. The erratum itself is
decision 0398 (the assembler pads every jump off 32 byte boundaries).
