# The native runtime's entropy has no source in a WASI module

Status: Observed (2026-10-16, xo-4b, TestSelfhostWasm); fixed 2026-10-16.

## Symptom

On the wasm backend the TLS goldens that sign (tls12_server,
tls12_tickets, tls13_server, tls_mlkem_off, tls_nistec, tls_rsa) stop
with `fault: the system random number generator failed`.

## Cause

std/native/llrt/rand.xo entropy() picks the system generator by
target (getrandom, getentropy, BCryptGenRandom) and faults for a wasm32
module (source 0). A WASI program's `Rand` capability works: the SSA
builder sends Rand's methods to the wasm runtime's random_get
(compiler/ssa rand_rt). But the crypto code calls rand.xo directly (no
capability), so in a WASI module it reaches the faulting arm.

## Fix

In a WASI module the SSA builder sends a call of rand.xo's `entropy` to
the wasm runtime's `rand_bytes` (WASI random_get), as `Rand.bytes`
(compiler/ssa call.xo wasi_served; decision 0231's "WASI entropy"
addendum, owner 2026-10-16). It is a lowering, not a std/native/rt.xo
declaration, so the native backends compile rand.xo unchanged and seed
0.18.0 still builds the compiler. TestSelfhostWasm no longer counts the
fault as unsupported.

The same holds for process.xo's `real_env_get` (the C environment, an
environ address of 0 in a WASI module): it goes to the wasm runtime's
`env_get` (WASI environ_get). It showed only once entropy worked:
tls_mlkem_off then ran and offered ML-KEM despite its XO_TLS_MLKEM=0.
