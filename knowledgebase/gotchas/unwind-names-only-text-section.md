# A frame in __xo_text had no name in xo dev unwind

Status: Observed (2026-10-22, Xo Publish).

Symptom: internal/arm64 TestUnwind/fiber/llvm failed: `xo dev debug`
printed `no variable "units" in frame 2`, and xo dev unwind's frames
lacked lldb's `x.xo.run[Task[(), ()]]+36` between the task's closure and
tasks.run_claimed. Not in -short, so the hourly check missed it from
5841a7efb (2026-10-21 17:53) until a gate ran it on 2026-10-22.

Cause: decision 0318 stage 4 puts an LLVM build's program functions in
their own section (__TEXT,__xo_text on macOS, xo_text on Linux).
compiler/debug unwind.xo sym_at named a frame without DWARF (the task
runner, compiler/ssa conc.xo runner, has no source position) only when
its pc was in __TEXT,__text, so the runner printed `?`. The test drops
`?` frames from its list, so its frame index was one short of the
session's numbering and `frame 2` selected the runner.

Fix: Tables.code holds every instruction section (Mach-O
S_ATTR_PURE_INSTRUCTIONS, ELF SHF_EXECINSTR) and sym_at names a pc in
any of them. Bisecting it by building old commits failed: today's seed
builds an older compiler that references runtime symbols its std lacks
(`Symbol not found: _xo_x_preempt`), so the frames were read directly.
