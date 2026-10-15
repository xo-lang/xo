# A lowered intrinsic's Xo body runs nowhere the goldens look

Status: Observed (2026-10-15, decision 0231).

## Symptom

std/native/llrt/cpu.xo's functions have Xo bodies that the native
emitters replace with instructions. Every crypto golden passed on arm64
and LLVM, but none of them ran the bodies: a wrong body would have
shipped, to surface only in a wasm module or in code an older xo (a
seed) compiled.

## Cause

On arm64 and LLVM the SSA builder replaces the call, so the body is
dead there. The wasm backend runs the bodies, but the crypto goldens
use std/internal, whose TLS code needs the scheduler the wasm backend
does not support yet (`rt.sched_rt`, stage 2): TestSelfhostWasm counts
them as unsupported.

## Fix

Give each body its own name the SSA builder does not know (`carry_soft`,
`aes_round_soft`, ...), make the recognized function a call of it, and
test the `_soft` names directly: the `cpu_prims` golden runs every op
both ways against a Go reference on every backend. Do the same for any
new intrinsic with a body.
