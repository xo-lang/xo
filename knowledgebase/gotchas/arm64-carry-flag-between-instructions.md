# arm64: an adc chain's carry lives in the flags between instructions

Status: Anticipated (2026-10-17)

Symptom (expected): a wrong sum with no error in code with cpu.adc
chains on the arm64 backend (P-256, P-384, X25519, bigmod), only after
a change to a pass or to the emitter, not to the program.

Cause: since 2026-10-17 (compiler/arm64 calls.xo ADC_SPLIT) a chain of
cpu.adc pairs is one MIR instruction per pair: adds, then adcs reading
the carry the previous pair left in the flags, with the products the
chain adds (mul, umulh) and the movs of constants and spills between
them. Nothing records that dependence: the flags are no register, so
regalloc, the opt passes and the emitter see independent instructions.
Every pair is marked keep, so dce and live_dce leave a pair whose sum
nothing reads. Anything that puts a flag setting instruction (cmp, adds,
subs, ands, tst) between two pairs, or moves a pair, breaks the chain:
a new scheduler or reordering pass, a spill sequence that adjusts sp
with adds or subs, a rematerialization that emits a compare, a fault
check inserted between them.

Fix (when it happens, or before writing such a pass): keep the pairs in
order and adjacent apart from instructions that set no flags (mov, movk,
movn, orr of an immediate, ldr, str, mul, umulh, add and sub without s),
or mark the flags as a register the chain defines and uses. To rule the
split out, set ADC_SPLIT false: the chain is one template again (more
registers: nistec.mul4p saves 5 callee saved pairs, not 1). Checks:
test/codegen/intrinsics (mul4p's pattern), the nistec and x25519 tests,
and the TLS handshakes on arm64 (bench/perf/tls), which fail on a wrong
field product.
