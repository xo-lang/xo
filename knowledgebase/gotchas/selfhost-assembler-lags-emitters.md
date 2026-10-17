# compiler/macho's assembler lags the emitters' new instructions

Status: Observed (2026-10-13, decision 0212)

Symptom: every `--as=macho` build (xo1 on darwin with compiler/macho,
and ELF on Linux) failed after the heap in Xo merged
(`unsupported instruction: prfm pldl1keep, [x12]`); http programs
failed before that (`sxtw x9, w0`, from rt.mem_cmp).

Cause: an emitter feature (rt.prefetch's `prfm`, mem_cmp's `sxtw`) was
checked with the platform assembler (internal/arm64's goldens, cc -c)
but not with compiler/macho, which accepts only the instructions and
directives it was taught, and fails the build on any other.

Fix: both instructions are in compiler/macho/enc.xo now, byte identical
to clang. After changing what an arm64 emitter prints, assemble a
program that uses it with both assemblers and compare the objects: build
compiler/macho (`xo build -o macho ./compiler/macho`), `xo asmdump
[--os=linux] DIR` (drop the first line), `clang -c -target
arm64-apple-macos11` (or `aarch64-linux-gnu`) and `macho asm [--elf]`,
then `cmp`. Two LLVM habits the ELF writer must copy: a branch to a
global symbol is relocated even inside its own section, and a TLS
relocation names its symbol (never the section) and makes it STT_TLS,
as LLVM types every symbol of a TLS section.
