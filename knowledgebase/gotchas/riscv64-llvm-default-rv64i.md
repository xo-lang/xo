# riscv64 under LLVM: the triple's default is RV64I

Status: Observed (Xo Lang 1, 2026-11-08, decision 0571 stage 1)

## Symptom

The first linux/riscv64 builds (`--backend=llvm`, host 2) failed three
ways, one after another:

1. `clang: error: unsupported argument 'generic' to option '-mcpu='`:
   the host's clang.cfg carries `-mcpu=generic`, an arm64 and x86 CPU
   name that clang refuses for a riscv64 target.
2. `instruction requires the following: 'D' (Double-Precision
   Floating-Point)` on the fiber switch's and preempt stub's `fsd` and
   `fld`: module asm and naked function asm are assembled without the
   command line's `-march=rv64gc`.
3. The link: `undefined symbol: __muldi3`, `__adddf3`, `__ltdf2`,
   `__atomic_load_8`, `__atomic_compare_exchange_8` and others, with
   "Hard-float 'd' ABI can't be used for a target that doesn't support
   the D instruction set extension (ignoring target-abi)". A nolibc
   build has no libgcc to supply them.

## Cause

`riscv64-linux-gnu` alone means RV64I: no multiply (M), atomics (A), or
floats (F, D). clang's `-march=rv64gc` reaches the C it compiles, but
the generated IR's functions take their features only from their
attribute groups. compiler/xo clang.xo `with_target_attrs` added clang's
target features to groups #1 and #4 only, which is enough on arm64 and
x86_64, whose defaults have every instruction the other groups need. On
riscv64 every other group compiled as RV64I, and LTO lowered each
multiply, float operation and atomic to a libgcc call.

## Fix

- clang.xo `llvm_cross_flags`: for riscv64, `-march=rv64gc -mabi=lp64d
  -mcpu=generic-rv64` (the last `-mcpu` wins over the config file's).
- `with_target_attrs(..., every)`: on riscv64, every attribute group
  gets the features (arm64 and x86_64 output stays byte for byte).
- The riscv64 asm blocks open with `.option push` and `.option arch, +f,
  +d`, and close with `.option pop`.

A new LLVM target whose triple's default lacks base extensions (RV32,
LoongArch, a soft float ARM) needs the same three checks.
