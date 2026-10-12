# The LLVM-built self-hosted compiler lays out frames without their by reference argument memory

Status: Observed (2026-10-12, xo1-perf agent, bench/results/xo1-perf-2026-10-12.md); fixed by decision 0159
(checked 2026-10-12 on main 13e232e: the LLVM-built xo1's assembly of
selfhost/xo is byte identical to the Go emitter's, see "Resolution" below)

Symptom: xo1 (selfhost/xo) built with `xo build --backend=llvm`
(`XO_LLVM_SPLIT=12 XO_LLVM_JOBS=4 XO_LLVM_LTO=none`, decision 0168)
compiles selfhost/xo to assembly that differs from the Go emitter's (and
from the arm64-built xo1's, which is byte identical to it) in 457 of 2,826
functions. Every difference seen is the frame size: in `parser.is_path`
the by reference argument area of the recursive call is at `sp + 256`
with 352 bytes, the arm64-built compiler makes a 608 byte frame, the
LLVM-built one a 256 byte frame, so the argument is written above the
frame (into the caller's). The bodies are the same.

Cause (as first suspected): `G.mem_bytes` reads as 0 at `G.emit` in the
LLVM-built compiler although `G.frame_area` (selfhost/arm64/byref.xo)
added the area: an increment of a field through a `var` receiver is lost
on the LLVM backend somewhere between `frame_area` and `emit`, or a stale
copy of `G` is written back over it. The arm64 backend runs the same
source as Go's internal/arm64 does.

Avoid: build xo1 with LLVM only from sources that include decision 0159
(17abac2 or later); an older LLVM-built xo1's output must not be used.
Compare a new LLVM-built xo1's assembly with `xo asmdump` (inside a
module, below) before trusting it.

Reproduced small (before 0159): `L asm` and `A asm` (L, A as in the results file)
differ on internal/arm64/testdata/run/byref_var (main's frame 960 bytes
against more than 8 KiB) and internal/arm64/testdata/run/far_out_slot (736 against
4,096 + 1,440 bytes), each in under a second.

Resolution (2026-10-12, llvm-xo1 agent): the observation came from a
branch (df055d5) cut before decision 0159 merged (17abac2). Before 0159
a compound assignment `t.f += e` read `t` before `e`, so the change a
`var` call inside `e` made to `t` was lost; on the LLVM backend this is
`g.mem_bytes += 8 * g.nwords(t)` in selfhost/arm64 byref.xo, frames
256 bytes short, exactly this symptom. On main 13e232e (0159 included),
L rebuilt with the same settings (954.8 s): `L asm` and `A asm` are byte
identical on byref_var and far_out_slot, and `L asm selfhost/xo` is
byte identical to `cd selfhost && xo asmdump xo` (sha256
d58a7339...ccae, 13,930,156 lines). Figures:
bench/results/xo1-perf-2026-10-12.md, "Note (2026-10-12): the LLVM-built
xo1 after decision 0159".

Second trap met on the way: comparing `xo1 asm <dir>` with `xo asmdump
<dir>` on a package outside a module (no xo.mod at or above it) shows
spurious differences. xo1 makes fault positions relative to the line
root (the directory itself, so `main.xo:300`), the Go command's asmdump
to the working directory (`internal/arm64/testdata/run/byref_var/main.xo:300`);
the string lengths differ (11 against 49), which changes which
constants are shared and so the register allocation of whole functions.
Compare xo1 against xo1 (A against L) on such packages, or against
`xo asmdump` only inside a module (selfhost/xo from selfhost/).
