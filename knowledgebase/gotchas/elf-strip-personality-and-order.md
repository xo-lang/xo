# A stripped ELF link called the personality routine at address 0

Status: Observed (2026-10-28)

## Symptom

A linux/arm64 XO_STD_ROOTS build linked with elflink's new section
strip faulted with SIGSEGV at address 0 the moment a fault unwound
(a caught one in task.isolate, an uncaught one's traceback); the same
program built normally ran.

## Cause

Each CIE in .eh_frame names the personality routine through a data word,
DW.ref.xo_x_personality, in a COMDAT section of its own. The strip never
walks .eh_frame (its FDEs would make every function live), so nothing
reached that section and it was dropped: the unwinder read 0 for the
personality and called it.

## Fix

elfstrip.xo roots what the CIEs name (their relocations), as the CIEs
always stay; the ELF store gives .eh_frame those names as its edges.
The FDEs, which name their functions, only follow a live function.
A second trap of the same family in the store's link: computing the
dead FDEs needs the globals, and calling add_globals twice on one linker
made its COMDAT bookkeeping drop every group's copy
(`DW.ref.xo_x_personality is not defined`): the FDE pass uses a linker
of its own.

A third, found by XO_STD_STORE_CHECK=1 (2026-10-28): the store's link and
the parts' were the same size but differed in 117 `bl` offsets. elflink
synthesizes the `__aarch64_*` outline atomics in order of first use,
before the GC; a stored object counted only what its kept sections named
while a parts object counted every undefined symbol, so the helpers were
placed in another order. A stored object now counts as stripped only
after the GC, like an atoms mode object.
