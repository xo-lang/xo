# 0254: Xo's own x86_64 backend (selfhost/amd64)

Status: Proposed (claimed 2026-10-17 by xo-fa; the lead's milestone
"native x86_64 without clang", with Xo Lang 2)

## Context

On arm64 hosts Xo builds with its own backend, assembler and linkers (no
C toolchain; decisions 0182, 0243, 0248). On x86_64 the default backend
is LLVM, so every x86_64 build needs clang. The arm64 backend
(selfhost/arm64: SSA to a machine IR, isel, regalloc, frames, text
assembly) and selfhost/macho (a text assembler and the Mach-O, ELF and
COFF writers and linkers) are the shape to follow.

## Decision (proposed)

A selfhost/amd64 backend with the arm64 backend's structure, emitting
GNU assembler text in AT&T syntax (what cc and clang assemble), and an
x86-64 assembler in selfhost/macho that encodes that text into the
object writers that exist. Text in between keeps cc as a fallback and as
the oracle: the same text assembled by clang and by Xo must give the same
bytes.

Stages and owners:

1. Encoder (Xo Lang 2): selfhost/macho asm_amd64.xo: the instruction set
   the emitter uses (integer, SSE2 scalar float, the BMI2/ADX, AES-NI,
   SHA-NI and PCLMULQDQ the crypto intrinsics use), REX and VEX
   prefixes, ModRM and SIB, rip relative operands, the relocations of
   ELF (R_X86_64_PC32, PLT32, GOTPCREL(X), TPOFF32), Mach-O and COFF.
   Golden tests: each form against llvm-mc's encoding.
2. Machine IR, isel, regalloc, frames, calls (xo-fa):
   selfhost/amd64 mir, isel (SSA to x86 forms: two operand instructions,
   memory operands, flags), regalloc (the arm64 one's linear scan with
   x86's register classes and the fixed registers of div, shifts and
   calls), frames (rbp chains, the SysV and Windows x64 ABIs, CFI), the
   runtime's intrinsics (rt.* that the arm64 emitter lowers: atomics,
   stack switch, thread locals, unwind save and resume), the fault and
   check stubs.
3. Crypto intrinsics on x86_64 (Xo Lang 2): cpu.adc chains (adc,
   adcx/adox, mulx with a fallback), SHA-256 (SHA-NI), AES rounds
   (AES-NI), carry-less multiply (PCLMULQDQ), as the arm64 emitter's.
4. ELF first: elflink's x86_64 relocations and its static mode (decision
   0243) for linux/amd64; then Mach-O (darwin/amd64, the Intel Mac) and
   PE/COFF (windows/amd64).
5. The default: x86_64 hosts default to selfhost/amd64 once the gates
   below pass; LLVM stays as --backend=llvm.

Verification: the 180 goldens on linux/amd64 in Docker (emulated, about
3x slower), the amd64 backend's asm pattern tests, the encoder's llvm-mc
goldens, progen across backends; CI on x86_64 runners when CI is back.
The self build's fixed point on x86_64 is the last gate before the
default changes.

## Consequences

To be filled in as stages land.

## Alternatives

- Keep LLVM on x86_64: clang on every x86_64 machine, and no control of
  its code shape (the reason for the arm64 backend).
- Emit machine code directly from isel (no text): faster builds, but no
  cc oracle and a second path for every directive; the arm64 path's text
  has been cheap (its assembly is about a tenth of a build).
