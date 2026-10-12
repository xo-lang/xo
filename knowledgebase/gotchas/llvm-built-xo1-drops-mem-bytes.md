# The LLVM-built self-hosted compiler lays out frames without their by reference argument memory

Status: Observed (2026-10-12, xo1-perf agent, bench/results/xo1-perf-2026-10-12.md), not fixed

Symptom: xo1 (selfhost/xo) built with `xo build --backend=llvm`
(`XO_LLVM_SPLIT=12 XO_LLVM_JOBS=4 XO_LLVM_LTO=none`, decision 0168)
compiles selfhost/xo to assembly that differs from the Go emitter's (and
from the arm64-built xo1's, which is byte identical to it) in 457 of 2,826
functions. Every difference seen is the frame size: in `parser.is_path`
the by reference argument area of the recursive call is at `sp + 256`
with 352 bytes, the arm64-built compiler makes a 608 byte frame, the
LLVM-built one a 256 byte frame, so the argument is written above the
frame (into the caller's). The bodies are the same.

Cause (not yet located): `G.mem_bytes` reads as 0 at `G.emit` in the
LLVM-built compiler although `G.frame_area` (selfhost/arm64/byref.xo)
added the area: an increment of a field through a `var` receiver is lost
on the LLVM backend somewhere between `frame_area` and `emit`, or a stale
copy of `G` is written back over it. The arm64 backend runs the same
source as Go's internal/arm64 does.

Avoid: do not use an LLVM-built xo1 for output (only its timing is
meaningful); compare its assembly with `xo asmdump` before trusting it.

Reproduced small: `L asm` and `A asm` (L, A as in the results file)
differ on internal/arm64/testdata/run/byref_var (main's frame 960 bytes
against more than 8 KiB) and internal/arm64/testdata/run/far_out_slot (736 against
4,096 + 1,440 bytes), each in under a second.

Next experiment: on byref_var, add `os.stdio.eprintln` of `g.mem_bytes` after `frame_area` and at the start
of `emit` in a scratch copy of selfhost/arm64 to see where the value is
lost; then reduce to a golden for internal/llvm.
