# 0669: the native Rand draws from a per-thread ChaCha8 keystream seeded by the OS

Status: Accepted (owner, 2026-11-16; proposed by Xo Lang 1, 2026-11-15). Its refusal of Lemire's multiply is superseded for buffered draws by 0696 (Proposed).
Stage 1 is on main (06afd1243, the scalar block); if the owner rejects this decision, stage 1 is reverted.

## Context

`Rand.int`, `float` and `bool` on the native backends (std/native/llrt/rand.xo `next_u64`) made one operating system
entropy call per draw (getrandom on Linux, getentropy on macOS) and an 8 byte Bytes. bench/perf/random, the Go twin
added for std/random (decision 0606), measured that against Go's math/rand/v2 top-level functions on the Mac
(twincheck -time 5, 2026-11-15):

| workload | Xo ms | Go ms | Xo / Go |
|---|---|---|---|
| normal, 10^6 | 2907 | 13.2 | 220 |
| exp, 10^6 | 2959 | 8.4 | 354 |
| perm, 2000 of 1000 | 3014 | 19.1 | 158 |
| zipf, 10^6 | 1522 | 25.3 | 60 |

Go's top-level functions draw from the runtime's per-M ChaCha8 generator (internal/chacha8rand), seeded from the
OS; Go's crypto/rand reads a userspace buffered source too (vgetrandom on Linux, arc4random_buf on macOS).
spec/std.md 3.4 promises a Rand that is cryptographically secure for real capabilities.

## Decision

1. **Generator.** Rand's words come from Go's chacha8rand, ported: four ChaCha8 blocks per refill (counters c to
   c + 3, zero nonce) in Go's interleaved layout, the key words added back, 32 words a buffer; after four refills
   the buffer's last four words, never handed out, become the next key (fast key erasure: a later state does not
   give earlier words). The words are Go's for the same seed (the known answer test below).
2. **Per thread, never shared.** The state (320 bytes) hangs off the worker record (layout.W_RAND), made and seeded
   (32 bytes of the OS generator) on the worker's first draw, so a new thread seeds its own. A thread that is not a
   worker (a C library's thread calling in) draws from the OS as before. Runtime code is never preempted (decision
   0318: the runtime section and the inlined runtime's unsafe ranges), so a draw or a refill is not interleaved with
   another fiber's on the same worker.
3. **Fork.** A forked child must not repeat its parent's words. Linux: the state's page is `MADV_WIPEONFORK` (since
   4.14; a kernel that refuses it gives the worker the OS path); FreeBSD: `minherit(INHERIT_ZERO)`; the child sees
   zeros, an unseeded state, and seeds anew. macOS has no inherit-zero, so a `pthread_atfork` child handler
   (cabi.xo `on_fork_child`, registered with the first state) zeroes the forking thread's state; only that thread
   exists in the child. Windows has no fork.
4. **What stays OS-direct.** `Rand.bytes` (llrt `next_bytes`), which std/crypto's keys, nonces and salts read, and
   every llrt crypto caller (rsagen, mlkem, nistec, tls12, tlsech, pg, mysqlauth: all `next_bytes`) keep reading the
   OS generator per call. The Str hash seed (`system_seed_at`) too. A test binary's seeded Rand (testworld's PCG) is
   unchanged.
5. **Same draws.** `rand.int`'s rejection keeps every draw it kept before (std/rt's randRange parity): a draw below
   2^64 - span is kept without computing the limit (the limit is at least that), and a power of two span takes a
   mask.

## Consequences

- Stage 1 (06afd1243, the scalar block): bench/perf/random normal 380.5 to 18.88 ms on linux/arm64 host 1 (0.050,
  0.048 to 0.050; xo bench --ab, 3 rounds). On the Mac (twincheck -time 7) against Go: normal 1.70, exp 2.15, perm
  1.54, zipf 1.81 (zipf is now 60% `Float.exp`, a math item of its own).
- Stage 2 (next push): the block as `cpu.chacha8_rand4`, a vector kernel on arm64 (NEON in the arm64 emitter,
  vector IR in LLVM; amd64 and wasm run the Xo body), as decision 0329's chacha_blocks4, and the draw's fast path
  inline. The scalar block was 60% of exp's time; Go's block is assembly. Against stage 1 on linux/arm64 host 1
  (xo bench --ab, 3 rounds): exp 17.39 to 14.42 ms (0.830, 0.807 to 0.843), perm 1.021 (within noise: perm is the
  remainder and the list, not the block). On the Mac against Go: normal 1.57, exp 1.91, perm 1.26, zipf 1.93.
- One page (4 KiB) per worker that draws; one more word in the worker record (W_SIZE 3616).
- Tests: internal/llvm TestRuntimeRandChaCha8 (testdata/rt/randchacha) on both native backends: Go's chacha8rand
  TestOutput vector, 372 words (three key erasures), from std/internal rt_probe "rand_kat"; and rt_probe
  "rand_fork": a forked child's next word against its parent's, which fails (0) with the atfork reset removed
  (checked on macOS 2026-11-15).

## Alternatives

- arc4random_buf on macOS per draw: fork safe (Apple's), but a library call and lock per word.
- A pid check per refill: up to 31 repeated words after a fork; refused.
- Lemire's multiply for `rand.int`: changes the seeded streams the Go backend and tests rely on.
- A thread local word (rt.tlocal_get) for the state: all 16 words are owned; the worker record grows instead.
