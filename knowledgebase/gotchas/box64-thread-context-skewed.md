# Box64 reports a thread's Rip and Rsp from different instructions

Status: Observed 2026-10-16 (xo-4), under Wine 10.0 and Box64 0.3.4 in
Docker on the Mac (scripts/docker-wine.sh box64).

## Symptom

The Windows CPU profile (decision 0238, a sampler thread that suspends
each thread and reads its CONTEXT) of testdata/prof/fib named fib at
100% flat, but only 61 to 76% of the samples reached main: 25 to 40% of
the stacks stopped after one frame.

## Cause

Every short stack started at one of fib's two `call fib` instructions,
and the "return address" read from its frame was 3, 13 or 233: fib's
saved n, 8 bytes from the return address. Box64 (a dynamic recompiler)
gives GetThreadContext a Rip still at the call while Rsp has already
taken the call's push, so RtlVirtualUnwind reads the frame 8 bytes off.
The walker rejects such a return address (it must follow a call in the
program's text), so the stack is short, not wrong.

## Fix

None in Xo: native Windows reports a precise context at an instruction
boundary. Read flat profiles under Box64 as they are; do not judge the
walker by cumulative figures there. Rosetta (MODE amd64) cannot run the
sampler at all ("rosetta error: invalid gdt selector index 5").
