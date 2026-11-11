# 0599: ECDH key agreement in std/crypto (Go's crypto/ecdh)

Status: Accepted (owner, 2026-11-11 morning, via the lead; proposed by Xo Lang 3, 2026-11-11; the lead's queue: the third gap
of the function level audit, bench/results/std-audit-2026-11-11.md).
Related: 0285 (std/crypto keys and signatures), 0276 (std follows Go's
standard library), 0189 (Ed25519), 0597 part 3 (shared code made public
as in Go).

## Context

The std audit of 2026-11-11 ranks crypto/ecdh third among the gaps: its
17 symbols (X25519, P256, P384, P521, Curve.GenerateKey, NewPrivateKey,
NewPublicKey, PrivateKey.ECDH, Bytes, PublicKey, Equal) are all unmapped.
Xo has the arithmetic, private to the TLS stack:

- std/native/llrt/x25519.xo: `scalar_mult(scalar, point)` and
  `base(scalar)` (RFC 7748), which TLS 1.3's key share uses;
- std/native/llrt/nistec.xo: `generate`, `public_key` and `ecdh(curve,
  priv, peer)` for P-224, P-256, P-384 and P-521 (crypto/ecdh's
  semantics: none for an invalid key or the identity), which TLS and
  std/crypto's ECDSA use.

A program that does its own key agreement (an end to end encrypted
message, HPKE, a Noise handshake, a JWE ECDH-ES key, a WireGuard style
tunnel, an AWS KMS or GCP envelope with an ephemeral key) has no public
way to it.

## Decision (proposed)

1. std/crypto gains crypto/ecdh's surface, with Go's error texts in
   KeyErr:

   ```xo
   enum EcdhCurve { X25519, P256, P384, P521 }

   /// A private key: Debug text `<privatekey>`, never key material.
   struct EcdhPrivateKey {}
   /// A public key: ==.
   struct EcdhPublicKey {}

   fn ecdh_generate_key(r: Rand, c: EcdhCurve) -> EcdhPrivateKey uses rand  // Curve.GenerateKey
   fn ecdh_private_key(c: EcdhCurve, key: Bytes) -> EcdhPrivateKey ! KeyErr // Curve.NewPrivateKey
   fn ecdh_public_key(c: EcdhCurve, key: Bytes) -> EcdhPublicKey ! KeyErr   // Curve.NewPublicKey
   fn (k: EcdhPrivateKey) curve() -> EcdhCurve
   fn (k: EcdhPrivateKey) bytes() -> Bytes
   fn (k: EcdhPrivateKey) public_key() -> EcdhPublicKey
   fn (k: EcdhPrivateKey) ecdh(peer: EcdhPublicKey) -> Bytes ! KeyErr       // PrivateKey.ECDH
   fn (k: EcdhPublicKey) curve() -> EcdhCurve
   fn (k: EcdhPublicKey) bytes() -> Bytes
   ```

   Encodings are Go's: X25519 keys are 32 bytes; NIST private keys are
   the scalar, big endian, as long as the order; NIST public keys are the
   uncompressed point (0x04 || x || y; compressed points and the identity
   are rejected, as Go rejects them). Errors: `crypto/ecdh: invalid
   private key size` (X25519 only; a NIST key of the wrong length is
   `crypto/ecdh: invalid private key`, as out of range), `crypto/ecdh:
   invalid public key` (a wrong X25519 length, the identity, a compressed
   point), nistec's texts for other bad NIST points (`invalid P256 point
   encoding`, `invalid P256 element encoding`, `P384 point not on curve`),
   `crypto/ecdh: private key and public key curves do not match`,
   `crypto/ecdh: bad X25519 remote ECDH input: low order point`.
2. Key generation takes the Rand capability, as generate_key does: X25519
   reads 32 bytes; a NIST curve samples the scalar as Go's randomPoint
   (and generate_key's ECDSA path) does, so one seeded r gives the same
   key on every backend.
3. The native bodies call llrt's x25519 and nistec (no new arithmetic);
   the Go backend's twins in std/rt call Go's crypto/ecdh with the same
   sampling. Checked against Go: a corpus of keys, public keys and shared
   secrets from Go's crypto/ecdh (seeded inputs, invalid sizes, points
   off the curve, the identity, low order X25519 points) on every
   backend, and Go's ecdh_test vectors (RFC 7748, NIST CAVP KAS).
4. Later stage, not in this one: ECDSA and ECDH conversions
   (ecdsa.PrivateKey.ECDH, PublicKey.ECDH) and X25519 in PKIX/PKCS #8
   (x509's parse and marshal of ecdh keys), when a program needs them.

## Consequences

- New public surface in std/crypto (spec/std.md 11 gains an ECDH
  subsection); the audit's map.txt maps crypto/ecdh's rows.
- The TLS stack keeps calling llrt directly; one implementation.
- P-224 is not offered (crypto/ecdh has no P224).

## Alternatives

- A separate std/ecdh module: Go has a package of its own, but Xo keeps
  keys, signatures and PEM in std/crypto (decision 0285), and ECDH keys
  share KeyErr and the Rand discipline there.
- Reuse PrivateKey and PublicKey with new KeyAlg values: Go keeps
  ecdsa and ecdh keys apart (an ECDH key cannot sign), and a KeyAlg of
  X25519 in parse_private_key would change what PKCS #8 parsing returns.

## Progress

- 2026-11-11, stage 1 (branch lang3-ecdh, held for the owner): the
  declarations in std/crypto.xo, native bodies over llrt's x25519 and
  nistec, Go backend twins in std/rt/cryptoecdh.go, the ecdh corpus
  against Go and golden crypto_ecdh. P-256's element text is Go's
  assembly field's (`invalid P256 element encoding`, amd64 and arm64);
  the same fix for parse_public_key landed on its own (3f87adc63).

