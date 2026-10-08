# 0056: std/crypto on the Go standard library, randomness through Rand

Status: Accepted (2026-10-08)

## Context
notes-service stored and compared bearer tokens in plain text (gap 4 in
`bench/results/real-app-2026-10-08.md`). `use go "crypto/sha256"` works
(decision 0041), but an auth service should not need Go interop for the
basics, and interop does not say how randomness relates to capabilities.

## Decision
- New module `std/crypto` (std.md 11): `sha256`, `hmac_sha256`, `equal`
  (constant time), `random_bytes(r: Rand, n)`, `token(r: Rand, n = 32)`,
  `hash_password(r: Rand, password, iterations = 600000)`,
  `verify_password(password, hash) -> Bool`.
- **Randomness is the `Rand` capability.** std.md 3.4 already defines
  `Rand` as cryptographically secure in programs and seeded in tests, with
  the `rand` effect. Every random std/crypto function takes a `Rand` and
  declares `uses rand`, so code that mints tokens shows it in its
  signature and tests are reproducible from the seed. No ambient secure
  random source is added.
- **Password hashing is PBKDF2-HMAC-SHA256** from Go's `crypto/pbkdf2`
  (standard library since Go 1.24; the toolchain is Go 1.26), 16 byte
  salt, 32 byte key, self describing string
  `pbkdf2-sha256$<iter>$<salt>$<key>` so the iteration count can be raised
  later without breaking stored hashes. Default 600000 iterations (OWASP
  2023 for PBKDF2-HMAC-SHA256).
- Hashing, HMAC, and comparison are pure (no effect).

## Consequences
- The compiler and runtime stay standard library only
  (`infrastructure/toolchain.md`, "Go module").
- PBKDF2 is not memory hard: it is weaker than scrypt or Argon2id against
  GPU attackers at equal latency. The format prefix leaves room for an
  `argon2id$...` variant later; `verify_password` would dispatch on the
  prefix.
- A test that hashes passwords at the default count is slow (hundreds of
  milliseconds each); tests pass a small `iterations`.
- API tokens (high entropy) are stored as `sha256` digests, which can be
  looked up by value; `hash_password` is for human chosen secrets.

## Alternatives
- scrypt or Argon2id from `golang.org/x/crypto`: stronger, but
  `golang.org/x/crypto` is not a dependency and the project policy is
  standard library only for the compiler and runtime. Rejected for now;
  revisit if a dependency policy for the runtime is adopted.
- bcrypt (also `x/crypto`): same objection, plus a 72 byte password limit.
- `crypto.random_bytes(n)` reading `crypto/rand` directly: an ambient
  effect outside the capability model (core 6), not deterministic in
  tests. Rejected.
- Naming `equal_constant_time` (as the gap list proposed): `crypto.equal`
  reads well with the module prefix, and the doc states the timing
  property. Chosen `equal`.
