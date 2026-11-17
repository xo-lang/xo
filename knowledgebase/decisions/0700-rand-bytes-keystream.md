# 0700: Rand.bytes draws from the worker's ChaCha8 keystream, not a system call each

Status: Accepted (owner's terms of 2026-11-17: accepted when host 1 `xo bench --ab` shows a gain, which it
did; proposed by Xo Lang 1, 2026-11-17, task #87). Supersedes decision 0669's "Rand.bytes, which std/crypto's keys and
nonces read, stays the system's generator"; the rest of 0669 stands.

## Context

Every `Rand.bytes(n)` on the native backends is one `getrandom` (Linux, FreeBSD), `getentropy` (macOS) or
`BCryptGenRandom` (Windows) call (std/native/llrt/rand.xo `entropy`). Lang 2's #88 (lever 10) found it in
std/trace's span ids (since moved to `Rand.int`); it is still under std/uuid `v4`, multipart boundaries,
smtp and amqp ids, bcrypt salts and hpke keys. Go's crypto/rand reads the kernel's vDSO `getrandom` on Linux
(a userspace ChaCha20 the kernel seeds) and `arc4random_buf` on macOS (a userspace ChaCha20 the system seeds):
no system call per read.

Decision 0669 gave each worker thread a ChaCha8 keystream (Go's runtime `internal/chacha8rand`, the generator
behind math/rand/v2's top-level functions), seeded with 32 bytes of the system's generator, with fast key
erasure, wiped in a forked child (Linux MADV_WIPEONFORK, FreeBSD INHERIT_ZERO, a macOS atfork handler). It
kept `Rand.bytes` on the system call because std/crypto reads keys and nonces through it.

The owner's terms (2026-11-17): accepted if host 1 `--ab` shows a gain, with the per-thread state seeded from
the OS as in Go's runtime, and seeded streams unchanged.

## Decision

- `Rand.bytes(n)` of the system's Rand on a worker thread copies n bytes of that worker's ChaCha8 keystream,
  8 bytes a word, the first lowest (`next_bytes` and `stream_bytes`), the same state `int`, `float` and
  `bool` draw from.
- Each word it takes is zeroed in the buffer as it is taken, so a later read of the state does not give
  bytes already handed out (Go's chacha8rand leaves spent words in its buffer; the key erasure every four
  refills covers the earlier keys' words).
- Unchanged: a seeded Rand and the test world (`xo test`, `--seed`) draw one seeded word a byte as before,
  so every seeded stream is the same; a thread with no worker record, or a system with no fork wipe, keeps
  the system call; wasm keeps WASI `random_get`; the Go backend keeps Go's crypto/rand.
- The generator is ChaCha8 with a 256 bit key from the OS and fast key erasure, which Go documents as
  cryptographically strong (math/rand/v2 `ChaCha8`); its known answer test (TestRuntimeRandChaCha8, Go's
  chacha8rand TestOutput vector) and fork test cover the bytes too, as they share the state.

## Consequences

- Measured: host 1 (linux/arm64, `xo bench --ab`, 3 rounds, 2026-11-17), bench/perf/uuid v4 (300,000 random
  UUIDs): 55.61 to 4.17 ms (B/A 0.075, 95% interval 0.072 to 0.075); max RSS unchanged.
- Tests: internal/llvm TestRuntimeRandChaCha8 (testdata/rt/randchacha) on arm64 and LLVM now also checks
  `Rand.bytes` lengths (0 to 4099), two draws that differ, each byte value of 256,000 within 6 standard
  deviations of 1/256, and the taken words zeroed (rt_probe "rand_bytes_wipe").
- bench/perf/uuid gains a `v4` workload (Go's `uuid.New`, crypto/rand) for the comparison with Go.
- A memory disclosure of a worker's state after a key is drawn gives neither that key nor earlier output;
  it gives the rest of the current buffer (at most 31 words) and, until the next key erasure, the words
  the current key will produce, as Go's chacha8rand and the kernels' generators do.

## Alternatives

- Keep the system call (0669): one call a draw, the cost Lang 2 measured.
- A second, ChaCha20 state only for `Rand.bytes`: 20 rounds against 8 for no property ChaCha8 lacks here
  (Go's runtime and math/rand/v2 rely on ChaCha8's strength), and a second page a worker.
- vDSO `getrandom` on Linux (kernel 6.11 and later): Linux only, needs vDSO symbol lookup and the kernel's
  opaque state allocation; macOS would still need `arc4random_buf` from libSystem.
- std/crypto on the system call and the rest on the keystream: bcrypt and hpke call `Rand.bytes` directly,
  so the split would not hold without a new API.
