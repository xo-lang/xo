# arm64: "reads virtual register N before it is written" through an inlined fault's landing block

Status: Observed (2026-11-11, ADR 0603 stage 1's std/sse, Azure 1); fixed 2026-11-11

Symptom: `xo build` (arm64) failed with `arm64 backend: internal: all
reads virtual register 140 before it is written` in a function that
passes its parameter into a generic constructor from another module
(sse.reader) whose inlined callee (sse.parser) faults on a bad argument,
then reads the result after a `while true { match r.next() { ... break } }`
loop. The LLVM backend built and ran it. Small unrelated edits (a literal
default instead of an imported const, another std module in the build)
moved it in and out of reach. Repro (a reduced copy of
std/sse as a local package): internal/llvm/testdata/golden/landing_memo,
which the arm64 goldens (TestGoldenRun) run too.

Cause: the per block memo (opt.xo: constants, string literals, facts)
starts from the memo of the block's dominator, and the dominators come
from the SSA predecessor edges only. A block holding an `rt.landing`
value is also entered by the unwinder from every instruction that
`lands` there, and an inlined callee's fault check puts such
instructions in blocks that are not its predecessors (here the entry
block and the fault branch beside the block that continues to the
landing). The landing block inherited the `""` its only predecessor had
put in a register; through it a join and the `fault_continue` return
block used that register on the unwinding path, where the predecessor
never ran.

Fix (compiler/arm64/isel.xo): the blocks that land at each landing block
are recorded (G.land_srcs); such a block inherits only from the nearest
common dominator of its own dominator and those blocks' dominators
(dom_nca), and starts empty when there is none (the entry block lands
there).

Avoid: a memo or register reuse across blocks must follow every way a
block is entered, the unwinder's included; the landing pads drop their
memo (decision 0085), and the block holding the `rt.landing` test is
entered the same way.
