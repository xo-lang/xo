# windows/amd64: a direct C call without a thunk overwrote the caller's frame

Status: Observed (2026-10-19, Wine under Box64), fixed in compiler/arm64
x86win.xo (w64_libc).

Symptom: the amd64 backend's arith golden on windows/amd64 crashed in
the C runtime's memcpy (MoveSmall1) with a double's bits as the source
pointer (read access to 0x3FF8000000000010), after its float line.

Cause: `7.5 % 2.0` calls fmod directly from generated code (isel's
g.rt("fmod")). The two doubles are in xmm0 and xmm1 under both
conventions, so the call looked right, but a Win64 callee may write
the 32 bytes of shadow space above its return address, which the
System V caller had not reserved: fmod overwrote the caller's own stack
words, and a later load read a double as a pointer.

Fix: every C function generated code calls by name goes through a
System V to Win64 thunk on windows/amd64 (memcpy, memmove, memset,
memcmp, fmod; platform calls by their own path). A new direct C call
in a template needs its entry in w64_libc.
