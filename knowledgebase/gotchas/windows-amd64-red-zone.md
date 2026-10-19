# windows/amd64: the x86 emitter's red zone has no Windows guarantee

Status: Anticipated (2026-10-19, decision 0254 6b step 1).

The x86 emitter saves a borrowed register below rsp (x86_scratch's red
zone at -8(k+1)), which System V's ABI keeps for the function. Windows
has no red zone: exception dispatch writes the CONTEXT and the
exception record below the faulting rsp. A fault inside a borrowed
register's sequence that lands in a landing pad of the same function
which reads that register would see the saved value lost.

Fix (planned): on windows/amd64, save borrowed registers in frame slots
(the spill area) instead of below rsp. Until then the window is the
few instructions of a borrow; no golden has hit it.
