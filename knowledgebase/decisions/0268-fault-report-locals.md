# 0268: Fault reports with the frames' locals

Status: Accepted (xo-4, 2026-10-18; owner approved the item in the
agents' survey recommendations; the design agreed with Xo Lang 3).
Implemented 2026-10-18: std/native/llrt dwline.xo's variables section
(not a separate dwvars.xo: it shares dwline's DWARF readers), fault.xo,
unwind.xo trace_frames, cabi.xo fault_traceback, prof.xo trace_text_at;
test internal/llvm TestFaultLocals.

## Context

A native program's fault report (`XO_TRACEBACK`, fault.xo
init_traceback; cabi.xo fault_traceback, prof.xo trace_text) lists the
frames with their files and lines from the program's own DWARF
(dwline.xo, decision 0239). Seeing the values that led to the fault
needs a debugger (`xo debug`, decision 0253) or a rerun with prints; the
agents' survey of 2026-10-18 ranked "the faulting frame's locals in the
report" among the largest gains for finding bugs in Xo code.

Decision 0252 makes the DWARF variables (DW_TAG_variable and
formal_parameter, their types, location expressions and lists) the one
source of truth for every consumer: lldb, gdb, `xo dev unwind --locals`
(TestUnwind checks its values against lldb's). compiler/debug reads them
from files; dwline.xo already reads the running program's DWARF at report
time (Linux: /proc/self/exe's .debug_*; macOS: the dSYM next to the
executable when its LC_UUID matches the image), with plain system calls
and never in a signal handler.

## Decision

1. **Trigger**: `XO_TRACEBACK=locals` (a value of the existing variable),
   off by default. Not on in debug builds by default: fault output goes
   to standard error, which goldens and diff tests compare, and reading
   DWARF after a fault adds time. `xo run` and `xo test` may set it under
   a dev flag later.
2. **Data**: the program's DWARF only (decision 0252), read as dwline.xo
   reads it (decision 0253's reader rules); no side table in the binary
   (it would cost bytes in every program and a second description to
   keep consistent with the emitters').
3. **Reader**: std/native/llrt dwvars.xo, beside dwline.xo, ported from
   compiler/debug's variable and location decoding: per function the pc
   ranges, its variables and parameters with names, types (base types,
   structures and members, typedefs, pointers by name), and locations
   (DW_OP_reg, regx, breg, bregx, fbreg, addr, piece; exprloc and DWARF 5
   loclists).
4. **Frames**: the trace block records each frame's register state next
   to its return address (the unwinder's 96 words, by DWARF number), and
   the report's text is made at the fault itself: the unwinding before
   the report releases the frames' values (a list read at the report
   showed reused memory). The faulting frame's registers are those the
   fault path saved (the trap context for a signal fault; the callee
   saved registers and fp, sp at the raise for a software fault); a
   caller frame has x19 to x28 (as the unwinder restores them), fp, and
   sp. A variable in a register the frame does not have shows as
   `<unavailable>`, as lldb shows it.
5. **Values**: Int, Float, Bool, Rune as values; Str as its text
   (bounded, escaped); a list or map as its length; other aggregates as
   their type name and words. Reads go through a checked reader (the
   address in a mapped range of the image or a heap chunk), so a bad
   pointer prints `<bad address>` instead of faulting again.
6. **Failure**: any failure while reading DWARF or values (no dSYM, a
   form the reader does not know) prints the plain report as today,
   with a note `(locals unavailable: REASON)`; the report is never lost.

## Consequences

- A fault report can answer "which value" without a rerun:
  `XO_TRACEBACK=locals ./prog` prints, under each frame, `name = value`.
- dwvars.xo is a second Xo reader of DWARF variables beside
  compiler/debug's; a test runs both over the same binaries and compares
  (as TestUnwind compares with lldb).
- A release build without `--debug-info` has no DWARF: the report says
  so.

## Alternatives

- **A compact locals table in an allocated section**: readable in the
  fault path without files, but bytes in every binary and a second
  description of the variables (Xo Lang 3, 2026-10-18).
- **On by default in debug builds**: changes the standard error that
  tests compare, and makes every fault slower.
- **Print only the faulting frame**: cheaper, but a fault in a helper
  (an index out of bounds in a std function) needs the caller's values.

## Addendum (2026-10-20, Xo lang 4): the faulting frame's caller saved values

Point 4 gave the faulting frame of a software fault only its callee
saved registers, fp, and sp: a variable in a caller saved register at
the fault (a leaf's, or a value just computed) printed `<unavailable>`,
since the fault stub's argument moves and the fault function overwrite
those registers before the report reads them.

- On arm64 the fault stub stores the caller saved registers (x0 to x15)
  that hold a word of a source variable's binding live at the faulting
  instruction (at most 8), below sp before its moves (`sub sp, sp, #N`;
  `stp`/`str`), and its DWARF entries over the stub place those words
  at the stored slots. The report reads them as it reads any sp slot.
- The same code with and without debug information: which registers a
  stub stores depends on the binding records and live ranges every
  build has, not on --debug-info (TestDebugInfoSameCode).
- Not in a stub whose fault can land in the same function (a landing
  pad resumes with sp as at the stub's call, and its slots would be off
  by the saves), nor in a far frame (slots through x28). A function that
  holds references has cleanup landing pads on its checks (decision
  0085), so its stubs store nothing and its caller saved values stay
  `<unavailable>` (every stub of the fib and sieve asm goldens is such
  a one); the saves cover the functions without pads.
  Lifted on arm64 (2026-10-21, Xo lang 4, poll item: three agents saw
  the faulting frame's locals `<unavailable>`, a division by zero in a
  loop and an index fault in main): a stub with a landing pad stores
  too, and its call site in the LSDA names the pad's entry instead of
  the pad, `mov sp, x29` or `sub sp, x29, #rec` then a branch to the
  pad (emit.xo pad_entry), one per pad among the stubs, so the pad runs
  with sp where the body keeps it. Only in a frame with its record and
  sp a fixed offset below x29 (not a leaf, not a shrink wrapped block,
  the record's offset under 4096). In bench/perf/http/xo 3,198 of the
  6,402 stubs have a pad; arm64 instructions 403,449 to 407,579
  (+1.0%, all in stubs: 1,255 more stubs store, 492 pad entries); no
  instruction of a body changes. TestFaultLocalsFaultingFramePad.
  x86_64 keeps the exclusion for now.
- Cost: xo's __text 35.36 to 35.43 MB (+0.20%); the arm64 sweep flat
  (stubs are cold).
- Secrets: the stored values are only read and printed under the opt
  in XO_TRACEBACK=locals, as every other local is; a default fault
  report prints none of them.
- x86_64 (2026-10-21) does the same: its stubs store rax, rcx, rdx,
  rsi, rdi, r8, r9, and r10 (r11 is the emitter's scratch) below rsp,
  described with DW_OP_breg7, under the same exclusions and also not in
  a Windows leaf (which jumps; windows/amd64 has no DWARF yet).

## Addendum (2026-10-22, Xo Publish): frames, literals, partial Str

From a three-frame fault (an index fault in pick, inlined into total,
called from main), on both backends:

- **Each variable under its own frame.** The reader (dwline.xo) records
  for every variable the innermost DW_TAG_inlined_subroutine holding it
  (its function's name, through the abstract origin, and its depth), and
  prof.xo trace_text_at prints it under that call's `[inlined]` line;
  the frame's own variables stay under the function's line. Before, an
  inlined call's variables were listed under the function holding the
  call, and LLVM's (which inlines main into xo_entry) all under main. A
  variable of an inlined call not on the fault's chain (one that ended
  before it, its register still described) is left out: arm64 printed a
  prelude helper's `v = 9`.
- **Each line once per frame**: LLVM described `items` twice (the
  parameter and its inlined copy).
- **A Str without its length** prints `<unavailable>`, not its raw words
  (`Str { obj: pointer 0x..., len: <unavailable> }`).
- **A literal's binding** (`let label = "order-7"`, `let n = 5`): its
  words are made again at each use (compiler/arm64 isel.xo words), so no
  register holds them across a call and it showed `<unavailable>` in
  every caller frame. The arm64 backend (dwvars.xo dv_plan, dv_const)
  describes a binding whose SSA value is an Int, Bool, or string
  constant as that constant over the whole function: DW_OP_constu or
  consts, and a string's object by DW_OP_addrx when the function already
  names it (so debug information adds no data). LLVM keeps the string's
  object out (gotchas/llvm-aggregate-dbg-value.md), so there a literal Str
  stays `<unavailable>`.
- Test: internal/llvm TestFaultLocalsFrames (arm64 and LLVM).

## Windows (2026-10-30)

XO_TRACEBACK=locals works on native windows/amd64 too (it printed a note
and no variables since the W9 field report, 2026-10-23): unwind.xo
trace_frames takes each frame's registers from the RtlVirtualUnwind
walk's CONTEXT (win_walk_regs: rax to r15 by DWARF number, before the
frame is unwound, so the nonvolatile ones and rsp are the frame's);
dwline.xo vars_load reads the exe's own .debug_ sections (pe_sects, as
the line tables already did); probe asks VirtualQuery whether memory can
be read. The callee saved set stays System V's (Xo's own calls are
System V's on Windows). The fault stubs' caller saved registers are in
frame words there (x86emit.xo stub_at), so the faulting frame's
variables print as on Linux. Checked on the owner's windows/amd64 box
against linux/amd64: the same variables and values.

