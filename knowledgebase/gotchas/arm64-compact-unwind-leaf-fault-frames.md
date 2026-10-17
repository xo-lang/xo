# Compact unwind says FRAME for a leaf whose frame is only on its fault paths

Status: Observed (2026-10-17, decision 0253's unwinder; the effect on the
runtime's own walkers is Anticipated)

Symptom: a stop (or a sample) in the body of an arm64 function such as
testdata/debugvars' `sum` on macOS: an unwinder that trusts
`__unwind_info` reads the caller's frame record as the function's own and
skips the caller (`sum`'s caller `main` was missing; the walk went from
`sum` to `xo_entry`). lldb is right: for the stopped frame it emulates
the instructions instead.

Cause: the arm64 backend sets a function's frame up only where it needs
one; a leaf whose only calls are its fault stubs (the overflow and
bounds checks' `bl _xo_x_fault_raise_*` after its last `ret`) builds the
frame record in each stub, and its `.cfi_def_cfa w29, 16` and
`.cfi_offset` directives come after the returns. DWARF CFI (`.eh_frame`,
Linux) keeps the position: before the directives the CFA is sp and the
return address is in lr. A compact encoding has no position: the
assembler (selfhost/macho, and clang's) and ld64 make it
`UNWIND_ARM64_MODE_FRAME` for the whole function, which is wrong for its
body.

Fix (the debugger, selfhost/debug unwind.xo): for the stopped frame
only, a compact FRAME encoding counts once the function's code from its
start to the pc holds the frame record's store (`stp x29, x30, [sp, ...]`);
before that the frame is a leaf (the return address in lr, sp
unchanged). A caller's pc is always at a call, past its frame's set up.

Anticipated: the runtime's sampling profiler and tracebacks on darwin
walk the same way for an interrupted leaf; a sample in such a body may
attribute to the caller's caller. Next experiment: XO_PROF=cpu on a
program spending its time in a checked arithmetic leaf (sum's loop), and
compare the profile's callers with the source. If they are wrong, the
same check in the walker, or a compact encoding of FRAMELESS for such
functions with the stubs' frames in `__eh_frame` (DWARF mode costs an FDE
per function).
