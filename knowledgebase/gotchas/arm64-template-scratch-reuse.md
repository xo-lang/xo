# arm64 templates: spilled operands share scratch registers

Status: Observed (2026-10-16, decision 0231's addendum, adc chains). A
latent miscompile: found by reading the emitter, not by a wrong result.
Fixed in the emitter the same day (the second Fix below); adc_chain can
be switched back on.

Symptom: none yet seen; the risk is a wrong value with no error. An
instruction template with many operands (selfhost/arm64 calls.xo
adc_chain: up to ten uses and six definitions in one template) computes
with the wrong operand when several of them were spilled by the register
allocator.

Cause: selfhost/arm64 emit.xo emit_plain reloads a template's spilled
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
