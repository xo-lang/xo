# The native runtime's entropy has no source in a WASI module

Status: Observed (2026-10-16, xo-4b, TestSelfhostWasm).

## Symptom

On the wasm backend the TLS goldens that sign (tls12_server,
tls12_tickets, tls13_server, tls_mlkem_off, tls_nistec, tls_rsa) stop
with `fault: the system random number generator failed`.

## Cause

std/native/llrt/rand.xo entropy() picks the system generator by
target (getrandom, getentropy, BCryptGenRandom) and faults for a wasm32
module (source 0). A WASI program's `Rand` capability works: the SSA
builder sends Rand's methods to the wasm runtime's random_get
(selfhost/ssa rand_rt). But the crypto code calls rand.xo directly (no
capability), so in a WASI module it reaches the faulting arm.

## Fix (to do)

An rt intrinsic for entropy that the wasm runtime lowers to random_get
and the native backends never reach (rand.xo's wasm32 arm), declared
in std/native/rt.xo: a new intrinsic needs the owner's approval and,
where the compiler's own code reaches it, a seed. Until then
TestSelfhostWasm counts this fault as unsupported (wasm_test.go
wasmCCallRE), so it is listed, not hidden.
