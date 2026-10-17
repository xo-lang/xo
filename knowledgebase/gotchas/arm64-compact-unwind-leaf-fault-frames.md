# Compact unwind says FRAME for a leaf whose frame is only on its fault paths

Status: Observed (2026-10-17: decision 0253's unwinder, and the runtime's
CPU profiler on darwin/arm64); fixed 2026-10-17 for leaves without landing
pads (step 1 below, xo-4), open for leaves with them

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
assembler (compiler/macho, and clang's) and ld64 make it
`UNWIND_ARM64_MODE_FRAME` for the whole function, which is wrong for its
body.

Fix (the debugger, compiler/debug unwind.xo): for the stopped frame
only, a compact FRAME encoding counts once the function's code from its
start to the pc holds the frame record's store (`stp x29, x30, [sp, ...]`);
before that the frame is a leaf (the return address in lr, sp
unchanged). A caller's pc is always at a call, past its frame's set up.

The runtime's profiler (Observed, 2026-10-17, darwin/arm64, main at
49ea43ce): a program whose time is in `mix`, a leaf with checked
arithmetic called only from `outer` (both kept out of line by a never
taken self call), profiled with XO_PROF=cpu: 130 of 130 samples in mix
have the stack `mix <- main`; `outer` is missing from every one (lldb
stopped in mix shows mix, outer, main). mix's compact encoding is
0x04000000 (FRAME), outer's DWARF mode. std/native/llrt/unwind.xo
(cstep_a64) trusts the encoding, so in mix's body it takes the caller's
frame record for mix's. Fault tracebacks are right (they start in a
stub, after its frame is set up). Linux is not affected: .eh_frame keeps
the directives' positions.

Fix plan (the arm64 emitter and the assemblers' compact encoding; for
Xo lang 4, who owns the profiler and its emitter end):

1. In compiler/arm64, give a function whose frame record is built only
   in its fault stubs (no `stp x29, x30` in its entry: the shrink
   wrapped leaf) two unwind regions: `.cfi_endproc` after the body's
   last instruction, then `.cfi_startproc` for the stubs with the rules
   they already state (`.cfi_def_cfa w29, 16`, `.cfi_offset w30, -8`,
   `.cfi_offset w29, -16`) and the function's `.cfi_personality` and
   `.cfi_lsda` when it has landing pads (a stub's call to the fault
   raise may be in the LSDA's call sites). The body's region has no
   directives: compact FRAMELESS with no stack (the return address in
   lr), which is exact for it; the stubs' region is FRAME, exact for
   them. No new symbol is needed (an __unwind_info entry or an FDE per
   region), no DWARF-mode FDEs (an FDE per function would cost
   __eh_frame space), and the same on Linux (two FDEs, each exact).
   A body that moves sp without a frame record (spills below sp with no
   fp) gets `.cfi_def_cfa_offset` in its region, which compact encodes
   as FRAMELESS with a stack size.
2. Check that compiler/macho's assembler (cfi.xo) and linker, and ld64
   with clang's assembler (--ld=cc, --as=cc), emit one compact entry
   per region and that `__unwind_info` keeps them in address order
   (objdump --unwind-info: two entries for such a function, 0x02000000
   then 0x04000000).
3. Tests: the profile above (every mix sample has outer as its caller:
   a test beside the profiler's in internal/arm64), a fault in mix
   (its traceback still shows mix, outer, main), lldb `bt` in mix's
   body and in a stub, and decision 0252's same code gate (only
   directives change). Then drop the frame 0 check in compiler/debug
   unwind.xo (frame_set_up), or keep it for programs built before the
   fix.

Done (2026-10-17, xo-4): compiler/arm64 emit.xo closes the body's region
before a leaf's fault stubs and opens one for them (`.cfi_endproc`,
`.cfi_startproc`, then the stubs' frame rules) when the function has no
landing pads. testdata/prof/leaf's mix: two compact entries, 0x02000000
(the body, frameless) then 0x04000000 (the stubs); outer is the caller in
100% of mix's samples (was 0); a fault in mix still unwinds mix, outer,
main (internal/llvm TestProfLeafCaller, which fails on the old emitter).
Only directives change, so the code is the same (decision 0252's gate).
Open: a leaf with landing pads keeps one region, because the LSDA's
call-site offsets count from its region's start; the stubs' region would
need an LSDA of its own (or @LPStart set to the body's start).
