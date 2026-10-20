# 0274: the native backends emit machine code, not assembly text

Status: Proposed (owner approval 2026-10-20, through the lead; Xo Lang 2).
Stub claiming the number; the measurement and the plan come first, no
code before them.

## Context

The native backends build assembly text for each function, and Xo's
assemblers (compiler/macho for Mach-O, ELF, and COFF) parse that text
back into machine code. A cold build spends CPU formatting text and then
parsing it.

## Decision (to be written)

- Measure first: the CPU of building, joining, and parsing the text on a
  cold `xo build` (phase timings), and which encodings the assemblers
  already have.
- Per instruction encoders shared by the emitter and the assemblers; the
  text path kept for debugging and `xo asm`; the gate is byte identity of
  the object files against the text path.
- Coordinated with the emitter's owners (Xo lang 4, Xo Lang 3).
