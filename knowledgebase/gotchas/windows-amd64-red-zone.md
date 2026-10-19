# windows/amd64: the x86 emitter's red zone has no Windows guarantee

Status: Anticipated (2026-10-19, decision 0254 6b step 1); fixed in step 2
the same day before any golden hit it.

The x86 emitter saves a borrowed register below rsp (x86_scratch's red
zone at -8(k+1)), which System V's ABI keeps for the function. Windows
has no red zone: exception dispatch writes the CONTEXT and the
exception record below the faulting rsp. A fault inside a borrowed
register's sequence that lands in a landing pad of the same function
which reads that register would see the saved value lost.

Fix: on windows/amd64 a function whose emission borrows a register is
emitted again with three frame words past the spill slots (x86emit.xo
x86_emit, X86_BORROWS, x86_borrow_at), where the borrowed register is
saved instead of below rsp; more than three borrows in one instruction
is a compile error there. Reserving the words in every frame with spill
slots grew frames by 32 bytes everywhere (TestWindowsAmd64Identity
caught it) while no golden borrows at all.
