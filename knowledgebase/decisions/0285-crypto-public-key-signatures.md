# 0285: Public key signatures in std/crypto

Status: Accepted (owner, 2026-10-20)

## Context

The 2026-10-20 survey of five backend services (knowledgebase/
infrastructure/std-coverage.md) ranked public key verification tenth: an
auth service checks JWTs signed RS256 or ES256 by an identity provider,
and other services sign tokens or webhooks with keys from PEM files. Go
covers this with crypto/rsa, crypto/ecdsa, crypto/ed25519, crypto/x509's
key parsing, and encoding/pem. Xo's native std already has the
primitives for TLS (std/native/llrt rsa.xo, nistec.xo, ed25519.xo,
asn1.xo), with no public API over them beyond Ed25519 (spec 11).
Decision 0276 says std follows Go's scope, code, and tests with Xo's own
API.

## Decision

std/crypto gains public key signatures, keys, and PEM, following Go's
packages and tests:

```
enum KeyAlg { Rsa, EcdsaP224, EcdsaP256, EcdsaP384, EcdsaP521, Ed25519 }

struct PublicKey {}    // a handle-free value: algorithm and key bytes; Eq, Debug shows alg and size
struct PrivateKey {}   // Debug never shows key material (as Secret)

struct KeyErr { msg: Str }   // Go's texts: "x509: failed to parse public key", ...

/// PEM blocks (encoding/pem): type, headers, and bytes; text outside blocks skipped.
struct PemBlock { kind: Str, headers: Map[Str, Str], bytes: Bytes }
pub fn pem_decode(text: Str) -> List[PemBlock]
pub fn pem_encode(b: PemBlock) -> Str

/// x509.ParsePKIXPublicKey, ParsePKCS1PublicKey, and a certificate's key.
pub fn parse_public_key(der: Bytes) -> PublicKey ! KeyErr
/// x509.ParsePKCS8PrivateKey, ParsePKCS1PrivateKey, ParseECPrivateKey.
pub fn parse_private_key(der: Bytes) -> PrivateKey ! KeyErr
/// A JWK (RFC 7517) object's public key: kty RSA, EC (P-256/384/521), OKP (Ed25519).
pub fn jwk_public_key(jwk: Str) -> PublicKey ! KeyErr
pub fn marshal_public_key(k: PublicKey) -> Bytes          // PKIX DER
pub fn (k: PublicKey) alg() -> KeyAlg
pub fn (k: PrivateKey) public_key() -> PublicKey

/// Signatures over a message, hashed with h (SHA-256/384/512; Ed25519 ignores h).
/// RSA is PKCS #1 v1.5; ECDSA is ASN.1 DER (Go's SignASN1) or, for JWS, raw r||s.
pub fn verify(k: PublicKey, h: Hash, msg: Bytes, sig: Bytes) -> Bool
pub fn verify_pss(k: PublicKey, h: Hash, msg: Bytes, sig: Bytes) -> Bool
pub fn verify_raw_ecdsa(k: PublicKey, h: Hash, msg: Bytes, sig: Bytes) -> Bool   // JWS ES256
pub fn sign(r: Rand, k: PrivateKey, h: Hash, msg: Bytes) -> Bytes ! KeyErr uses rand
pub fn sign_pss(r: Rand, k: PrivateKey, h: Hash, msg: Bytes) -> Bytes ! KeyErr uses rand
pub fn sign_raw_ecdsa(r: Rand, k: PrivateKey, h: Hash, msg: Bytes) -> Bytes ! KeyErr uses rand   // JWS
pub fn generate_key(r: Rand, alg: KeyAlg) -> PrivateKey ! KeyErr uses rand   // ECDSA and Ed25519; Rsa is KeyErr (later)
```

- Verification never faults on bad input: a wrong length or malformed
  signature is `false`, as Go.
- RSA keys of 1024 bits or more verify (Go 1.24's minimum); 2048 to 8192
  for signing.
- RSA key generation (needs prime generation, math/big) is a later step;
  services import keys from PEM.


### One combined module (an exception to decision 0279)

Decision 0279 puts each Go package in its own std module (std/url,
std/base64, ...). This decision keeps every signature algorithm, key
parsing, and PEM in std/crypto, an explicit exception approved by the
owner: the callers (JWT and webhook libraries) pick the algorithm at run
time from a token header, so one `PublicKey` type over all algorithms,
next to the existing `crypto.Hash` (decision 0279), serves them where
five modules with five key types would make every caller switch over
them. Go's split follows its package history rather than a caller's
need. Key parsing (x509) and PEM go with it because they exist here only
to produce these keys; certificates stay in std/tls's domain.

### Nonces and timing


- **Hedged ECDSA nonces, as Go (crypto/internal/fips140/ecdsa, Go 1.24
  on).** The nonce k comes from an HMAC_DRBG (SP 800-90A, the hash of
  `h`) seeded with the private key, fresh bytes from the caller's `Rand`,
  and the message hash (RFC 6979 with added randomness, the
  draft-irtf-cfrg-det-sigs-with-noise construction Go uses). A `Rand`
  that repeats or returns zeros then degrades to deterministic RFC 6979
  signing, which leaks nothing; it never reuses a nonce across two
  messages. The native signer today takes k from the system generator
  alone; it moves to this construction, so a seeded test `Rand` gives
  reproducible, still safe, signatures. Tests: RFC 6979 appendix A.2
  vectors with the random input empty (the deterministic core), Go's
  results for the same key, message, and random bytes (a differential
  program, since Go's internal DRBG inputs can be fixed through its
  testing hooks), and a property test: with an all-zero `Rand`, two
  different messages never share r, and every signature verifies.
- **Constant time private key operations, a stated requirement.** Every
  operation on a private key or a nonce (ECDSA and Ed25519 scalar
  multiplication, the nonce inversion, RSA's CRT exponentiation) runs in
  time independent of the secret: no branch or memory index depends on
  it. The native code is built that way already (nistec.xo's fixed
  window ladders over ct.xo's masked selects; rsa.xo's private operation
  over bigmod.xo's constant time arithmetic; Go's rules), and the spec
  states it for every backend (the Go backend is Go's own packages).
  Test: a dudect style timing test (Welch's t-test between a fixed and a
  random secret class, 10^5 samples each, failing at |t| > 4.5) for
  ECDSA P-256 and P-384 signing, Ed25519 signing, and RSA 2048 signing,
  in bench/ct (on demand and in the nightly run, not per push, since
  timing tests need a quiet machine). A static check (no branch or index
  on a secret) would need taint tracking the compiler does not have; the
  timing test is the check.


Refinements while implementing (2026-10-20, within the approved scope):
`EcdsaP224` (Go parses P-224 keys); `sign_raw_ecdsa` for JWS signing;
the signers return `Bytes ! KeyErr` (Go refuses keys below 1024 bits
and messages too long for the key), and RSA signs from 1024 bits as Go
does; the Go backend's go.mod carries `godebug cryptocustomrand=1`, so
Go's signers read the program's `Rand` (Go 1.26 otherwise ignores a
custom reader) and both backends sign the same bytes for the same
`Rand`.

## Consequences

- JWT RS256, PS256, ES256/384/512, and EdDSA verification and signing
  can be written in Xo libraries over std/crypto.
- The native ECDSA signer (TLS servers too) moves from a system random
  nonce to the hedged construction: a std/native change, so it passes
  the self rebuild fixed point, and std/changes/crypto.md records it as
  Kind: security.
- RSA key generation waits for prime generation (a later step); keys
  come from PEM.
- spec/std.md 11 grows by the API above; bench/ct holds the timing test.

## Tests


Go's tests: rsa (pkcs1v15_test, pss_test vectors), ecdsa (Wycheproof
vectors that Go's crypto/internal tests use), x509 key parsing
(parse tests of x509_test for keys), pem_test; RFC 7515 appendix A JWS
examples (RS256, ES256, EdDSA) for the JWK and raw ECDSA paths; a
differential program against Go on generated keys and signatures; goldens
on every backend (Go backend through Go's packages).


## Alternatives

- A module per Go package (std/rsa, std/ecdsa, std/x509, std/pem):
  decision 0279's rule, set aside here for the reason above.
- A std/jwt module: not in Go's std; left to libraries, which this API
  is enough for.
- Deterministic RFC 6979 nonces alone: safe against a bad generator but
  exposed to fault attacks that Go's hedging also covers; Go chose
  hedging, and so does this.
