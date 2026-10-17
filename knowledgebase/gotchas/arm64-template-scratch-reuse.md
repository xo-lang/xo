# arm64 templates: spilled operands share scratch registers

Status: Observed (2026-10-16, decision 0231's addendum, adc chains). A
latent miscompile: found by reading the emitter, not by a wrong result.
Fixed in the emitter the same day (the second Fix below); the chains
are back on (ADC_CHAINS true, up to 8 pairs, 2026-10-16).

Symptom: none yet seen; the risk is a wrong value with no error. An
instruction template with many operands (compiler/arm64 calls.xo
adc_chain: up to ten uses and six definitions in one template) computes
with the wrong operand when several of them were spilled by the register
allocator.

Cause: compiler/arm64 emit.xo emit_plain reloads a template's spilled
uses into scratch registers gp_scratch(0), (1), (2): x16, x17, then x15
for the third and every later one (d30, d29, then d31 for floating
point). A fourth spilled use is reloaded into x15 over the third, and
both read one value. A spilled definition of a template not marked early
is stored from x16, so two spilled definitions of one template both come
from x16 and the first is lost (add_with_flag's `adds; cset` with both
defs spilled). An early definition gets a scratch register only while
fewer than three are taken, then x16, unless the template is marked
keep, which faults instead. Nothing checks it.

Fix: until the emitter gives spilled operands registers free at the
instruction or fails loudly (xo-fa, 2026-10-16), templates stay within
the scratch registers: adc pairs are one template each (at most three
uses and two definitions), marked early and keep, so exhaustion faults
at compile time (calls.xo ADC_CHAINS false turns the chains off).
internal/arm64 TestNoSharedScratchReload scans the arm64 assembly of
test/codegen/intrinsics and bench/perf/tls's client for two reloads into
x15 with only loads between, and for two spilled definitions stored from
x16 in a row.

Fix (the emitter, 2026-10-16): emit.xo scratch_reg gives every spilled operand of one instruction
its own register: the three scratch registers, then a caller saved
register no value holds at that instruction (regalloc keeps its final
occupancy in G.occ_regs; the emitter sets G.pos per instruction), then a
callee saved register the prologue saves when the block runs with the
frame. If none is left the build fails (`arm64: no free register for a
spilled operand in FN`) instead of sharing one. Definitions written
after every use is read reuse the uses' registers; an `early`
template's definitions avoid them.

Test: `XO_ARM64_REGS=N` (test only) gives the allocator N caller saved
and N callee saved registers per class, so most values spill;
internal/arm64 TestGoldenRunSpill runs the adc chain, P-256, crypto,
overflow, fault, concurrency, json, map, and closure goldens at N=2 and
N=4. Run a new multi-operand template through it.

Second trap (Observed 2026-10-16, chains back on): under the spill test
a long chain cannot fit at all. At XO_ARM64_REGS=2 there are 7 general
registers (two caller saved, two callee saved, x15 to x17), and a chain
of 6 pairs in x25519.fe64_sq is one early instruction needing 19
distinct registers: the build failed loudly with "no free register"
(the emitter fix working as designed, not a miscompile). With the full
set the same chains build and pass. adc_chain now caps a chain at
(2 N + 2) / 3 pairs under XO_ARM64_REGS = N (none at N = 2, 3 pairs at
N = 4); any long multi-operand template needs the same care.

Update (2026-10-16): x15 is no longer a scratch register (it is
allocated, last); the emitter's fixed scratch registers are x16 and x17,
and from the third spilled operand on scratch_reg finds a register free
at the instruction (which needs G.pos: copies of a block, a latch or a
return, set it to the block's positions). A template that writes a
register as a temporary lists it in Inst.clobbers.

