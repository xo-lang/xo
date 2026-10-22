# 0329: ChaCha20's key stream four blocks at a time in vector registers

Status: Accepted (owner, 2026-10-21, via the lead: route (b), an rt
intrinsic, over an integer-lane SLP stage, which stays a candidate)

## Context

TLS 1.2 ChaCha20-Poly1305 bulk ran at 0.44 of Go on linux/arm64, 2.2
times its CPU per MB (bench/results/tls-h2-vs-go-2026-10-21.md, Xo Lang
3's profile): std/native/llrt chacha.xo's xor_stream computed one 64
byte block at a time in scalar 32 bit arithmetic in 64 bit registers
(an `& M32` per addition), and pushed each word of output (push_xor,
12% flat). Go computes four blocks at a time in NEON (chacha_arm64.s).
The arm64 SLP pass (decision 0323) pairs Float operations only.

## Decision

- cpu.chacha_blocks4(dst, src, st) in std/native/llrt cpu.xo, with a
  portable body (chacha_blocks4_soft), as cpu.aes_enc10 is: dst = src
  xor four blocks of key stream (256 bytes), from the 80 bytes at st (the
  16 state words, word 12 unused, then the four blocks' counters).
- compiler/ssa maps it to rt.chacha_blocks4 on arm64 only (cpu_call);
  amd64 (both backends) and wasm run the portable body.
- arm64 (compiler/arm64 calls.xo chacha_neon): the vertical layout, one V
  register per state word, a lane per block; the four quarter rounds of
  a half round step by step together; rotations by 16 as rev32 on
  halfwords, the others shl and sri into a free register that becomes
  the word's (no moves); the state added back, each group of four words
  transposed into the blocks (trn1 and trn2 on .4s then .2d), xored with
  src, stored. The instruction clobbers v0 to v7 and v16 to v31 (never
  v8 to v15, whose low halves a callee keeps). compiler/macho encodes
  shl and sri by an immediate, rev32 .8h, dup .4s from a lane, trn1 and
  trn2 (words checked against the system assembler).
- LLVM (compiler/llvm call.xo chacha4): the same in <4 x i32> vector IR,
  rotations with llvm.fshl, the transposes as shufflevector.
- chacha.xo's xor_stream writes four blocks at a time straight into the
  output string's room (no per word push), then the rest a block at a
  time as before.

## Consequences

- Bulk ChaCha20 (2,000 x 16 KB, Mac arm64 instructions): native 837 to
  197 M (-76%), LLVM 881 to 192 M (-78%); output identical. Poly1305
  stays scalar (a separate item).
- Golden chacha_blocks4: every length class around 256 (0 to 4101 bytes)
  at two counters, as a SHA-256 of the output, against the Go backend's
  output, which is Go's own ChaCha20.
- A new intrinsic the compiler's own code reaches (xo includes std's
  TLS) must not be mapped in a build whose backends cannot lower it:
  gotchas/intrinsic-mapped-before-lowered.md.

## Alternatives

- (a) An integer-lane SLP stage (4 x 32 bit add, eor, rotate) for
  ChaCha, SHA-256, and Poly1305 limbs: general, but multi-day; measure
  SHA-256 and Poly1305's gain first (the lead's condition).
- A source restructure (four blocks per iteration in scalar Xo): more
  instruction level parallelism only, no vectors.
