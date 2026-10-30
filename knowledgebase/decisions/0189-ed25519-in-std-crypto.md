# 0189: Ed25519 in std/crypto

Status: Accepted (2026-10-12), implemented on every backend: std/crypto.xo,
std/rt/crypto.go (Go's crypto/ed25519), std/native/crypto.xo and
rtcrypto.xo with std/llrt/xo_crypto.c (SHA-512 and Ed25519 in C), spec
std.md 11; tests: internal/llvm TestCryptoDiff (ed25519Cases) and the
golden internal/llvm/testdata/golden/native_ed25519.

## Context

Decision 0178 stage 4 moves the module commands (`get`, `mod`, `vendor`,
`publish`, `audit`) to Xo. Their checksum log client (internal/sumdb,
decision 0124) verifies signed tree heads with Ed25519, and the log
server signs them; key generation makes a seed and its public key. Xo's
std/crypto had SHA-256, HMAC, PBKDF2, Argon2id, and random bytes, but no
signature scheme. Decision 0178 named this "a std decision": the module
code in Xo cannot be written without it, and a program verifying signed
data (webhooks, tokens) needs it too.

## Decision

Three pure functions in std/crypto (spec std.md 11):

```
pub fn ed25519_public_key(seed: Bytes) -> Bytes                   // 32 bytes
pub fn ed25519_sign(seed: Bytes, msg: Bytes) -> Bytes             // 64 bytes
pub fn ed25519_verify(public_key: Bytes, msg: Bytes, sig: Bytes) -> Bool
```

- The private key is the 32 byte RFC 8032 seed, as Go's
  `ed25519.NewKeyFromSeed` takes it and as the checksum log's signer key
  stores it (`PRIVATE+KEY+<name>+<id>+<base64 seed>`). Key generation is
  `crypto.random_bytes(r, 32)`, so no new function needs `Rand`.
- A seed that is not 32 bytes is a fault (a programming error, like a
  negative length to `random_bytes`): `crypto.ed25519_sign: the seed is N
  bytes, want 32`. Verification never faults: wrong lengths, a key that is
  no point, and a non canonical S are `false`.
- The semantics are Go's `crypto/ed25519` exactly: cofactorless
  verification comparing the encoding of [S]B - [k]A with R byte for byte,
  S must be below the group order L, and the point encodings Go's
  edwards25519 accepts (y not reduced modulo p, x = 0 with the sign bit
  set) are accepted. So a signature both backends see gets one answer.
- Go backend: std/rt calls crypto/ed25519. Native backends: C in
  xo_crypto.c, written for this decision from RFC 8032 and FIPS 180-4:
  SHA-512 (round constants copied from the vendored Mbed TLS sha512.c),
  field arithmetic modulo 2^255 - 19 in five 51 bit limbs with 128 bit
  products, extended coordinates with the complete addition law (one
  formula adds and doubles), a double and always add ladder with masked
  selection (constant time in the secret scalar), scalars reduced modulo L
  bit by bit with masked subtraction, and the curve constants (d, 2d,
  sqrt(-1), the base point) computed at first use instead of typed in.
  SHA-512 stays internal: std/crypto does not expose it.

## Consequences

- TestCryptoDiff compares the C code with Go on the RFC 8032 section 7.1
  vectors 1 to 3, SHA-512 of the FIPS 180-4 examples and of lengths
  around the padding boundaries, random seeds and messages (public key,
  signature, verification), mutated signatures, messages, and keys,
  random 32 byte keys (about half are no point), S + L, the non canonical
  y encodings p to p + 18 with both sign bits, and x = 0 with the sign
  bit: 992 operations agree with `-short` (2026-10-12).
- The golden native_ed25519 prints the same lines on the Go, arm64, and
  LLVM backends (checked by `xo run --backend=...` on 2026-10-12; TestGolden
  compares them on the next full run).
- The ladder does 256 complete additions per doubling and per add: a
  signature or a verification is two to three scalar multiplications,
  fast enough for tree heads and tokens; a faster table based method is
  possible later without changing results.
- The wasm backend has no std/crypto yet; nothing changes there.

## Alternatives

- **Vendor an Ed25519 implementation** (ref10, Monocypher, TweetNaCl):
  more code to review than the field and group operations needed here,
  and the vendored Mbed TLS has no Ed25519.
- **Expose SHA-512 too**: not needed by any caller yet; adding a std
  function later is cheap, removing one is not.
- **A key pair type** (`Ed25519Key` holding seed and public key): the
  module code and the log formats pass raw bytes; a type can wrap these
  functions later.
- **Ed25519ph or contexts (Ed25519ctx)**: Go's `VerifyWithOptions`
  supports them; nothing in Xo uses them.

## Amendment (owner, 2026-10-29)

`ed25519_sign` also accepts Go's 64 byte private key (the seed, then its
public key). It signs with the stored public key, as Go's `ed25519.Sign`
does, and skips the base multiplication that derives it from the seed,
about half of a signature's work. The 32 byte seed form is unchanged; any
other length faults with `crypto.ed25519_sign: the key is N bytes, want
32 or 64`. Like Go, a stored public key that does not belong to the seed
is not detected; it yields a signature that does not verify.
