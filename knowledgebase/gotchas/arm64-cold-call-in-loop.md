# A never-taken call in a hot loop slows the loop on arm64

Status: Observed (2026-10-21, Xo Lang 3, decision 0291's hybrid Map); fixed 2026-10-21

Symptom: a Map get loop with one more branch, to a call that never runs
(the hybrid Map's out-of-line log lookup), took about 1.5x the time of
the lists' loop although it retired only 1.19x the instructions.

Cause: the call, even in a cold block, clobbers the caller-saved
registers, so arm64's register allocator keeps the loop's live values
in callee-saved registers (more saved in the prologue) and stops
hoisting loop invariants: the loop rebuilt a constant (movz/movk), the
static base (adrp/add), and the lists' element bases each iteration,
which the call-free lists loop computed once.

Fix (2026-10-21, Xo lang 4; optional pass cold_saves): a call of a
runtime slow path (ssa cold_fn, such as xo.maph.loglookup) is flagged
cold_call in isel; the register allocator no longer counts it as
clobbering the caller saved general registers, and stores the values
it finds in them across the call to slots right before its argument
moves, loading them after its results' moves, all in its block; and
hoist_consts and hoist_invariants hoist past it. A Map[Int, Int] get
loop with XO_MAP_LOG=1: one callee saved pair fewer, the constants, the
static base of the preemption polls and the lists' element bases out
of the loop, 110.3 to 100.0 million instructions retired (-9.3%, the
whole program). arm64 only (x86_64 still counts the call), and only
cold_fn calls, not every call of a cold block (the landing pads'
cleanup calls freed so many caller saved registers that an instruction
with ten spilled operands found no scratch register). For another slow
path, add it to ssa cold_fn.
