# 0618: RSA encryption in std/crypto (OAEP, PKCS #1 v1.5)

Status: Proposed (Xo Lang 3, 2026-11-12; the owner's std track, medium list: "RSA OAEP").

## Context

std/crypto signs and verifies with RSA (PKCS #1 v1.5 and PSS, decision
0285) and generates RSA keys, but cannot encrypt with them. The std audit
of 2026-11-12 lists `crypto/rsa.EncryptOAEP`, `DecryptOAEP`,
`EncryptPKCS1v15`, `DecryptPKCS1v15` and `DecryptPKCS1v15SessionKey` as
the module's largest gap group (12 symbols). Programs need it to wrap a key
for a recipient's RSA public key (JWE `RSA-OAEP` and `RSA-OAEP-256`, AWS KMS
and Google Cloud KMS asymmetric decrypt, PKCS #7 and CMS envelopes, legacy
TLS and SAML peers).

The pieces exist in std/native/llrt/rsa.xo: the public and private
operations over bigmod (the private one by the CRT, with its result checked
by the public operation), MGF1 and the PKCS #1 encodings of signing.

## Decision

Four functions in std/crypto, on every backend:

```
/// RSA-OAEP (Go's EncryptOAEP): msg under k with h for the hash and MGF1,
/// and label; a seed of h.size() bytes from r. KeyErr for a key that is not
/// RSA or a msg longer than the key's size - 2 * h.size() - 2.
fn encrypt_oaep(r: Rand, k: PublicKey, h: Hash, msg: Bytes, label: Bytes = b"") -> Bytes ! KeyErr uses rand
/// Go's DecryptOAEP: KeyErr `crypto/rsa: decryption error` for any
/// ciphertext that does not decrypt, with no detail of why.
fn decrypt_oaep(k: PrivateKey, h: Hash, ciphertext: Bytes, label: Bytes = b"") -> Bytes ! KeyErr
/// PKCS #1 v1.5 encryption (Go's EncryptPKCS1v15), for protocols that
/// require it; encrypt_oaep otherwise.
fn encrypt_pkcs1v15(r: Rand, k: PublicKey, msg: Bytes) -> Bytes ! KeyErr uses rand
/// Go's DecryptPKCS1v15, the same single error.
fn decrypt_pkcs1v15(k: PrivateKey, ciphertext: Bytes) -> Bytes ! KeyErr
```

- **Constant time.** Decryption checks the padding as Go does, without
  branches or indexes on secret bytes:
  - the leading zero, the label hash and the 0x01 separator (OAEP);
  - the 0x00 0x02 head and the first zero after eight padding bytes
    (PKCS #1 v1.5).
  The checks use arithmetic masks and selects over every byte, and only
  the final valid or invalid answer branches. A timing test under
  decision 0616's review checklist confirms it.
- **One error.** Every decryption failure is the same KeyErr text, Go's
  `crypto/rsa: decryption error`: ciphertext too long or not below the
  modulus, a bad padding, a label mismatch, or a failed private operation.
  This keeps an oracle (Bleichenbacher, Manger) from telling causes apart.
  Encryption errors are Go's texts (`crypto/rsa: message too long for RSA
  key size`).
- **Hashes.** One hash for OAEP's label hash and MGF1, as Go's
  EncryptOAEP. Go's OAEPOptions.MGFHash, a separate MGF1 hash, is left
  out until a peer needs it. Any std/crypto Hash is allowed.
- **Randomness.** From the Rand capability, as signing does: OAEP's seed
  is h.size() bytes; PKCS #1 v1.5's padding is nonzero bytes, a zero drawn
  again. On the Go backend the same bytes are read from r, and the
  ciphertext comes from Go's crypto/rsa, which may use its own randomness.
- **Not included:** `DecryptPKCS1v15SessionKey` (the TLS 1.2 RSA key
  exchange's form, which Xo's TLS does not offer) and the `crypto.Decrypter`
  interface (no interface of that shape in std/crypto).

Tests:
- Go's TestEncryptOAEP known-answer vectors, with fixed seeds, through the
  llrt functions.
- Go's TestDecryptPKCS1v15 vectors.
- A golden that decrypts ciphertexts made by Go's EncryptOAEP (SHA-1,
  SHA-256, labels) and EncryptPKCS1v15 under a fixed 2048-bit key, on every
  backend, plus every rejection case (a changed byte, a wrong label, a
  short or long ciphertext, one not below the modulus) giving the same
  error.
- Round trips with generated keys.

## Consequences

- JWE, KMS and envelope formats become writable in Xo without a C
  library.
- PKCS #1 v1.5 encryption ships for compatibility, documented as legacy,
  as in Go (whose documentation marks it Deprecated: "PKCS #1 v1.5
  encryption is dangerous and should not be used").
- The constant-time claim is backed by a test, not only by review.

## Alternatives

- **OAEP only**: simpler, but SAML, PKCS #7 and older KMS integrations
  still use PKCS #1 v1.5. Shipping it with OAEP as the documented default
  matches Go.
- **An MGF1 hash option**: deferred; no known peer of Xo programs uses a
  different MGF1 hash (Go's own Test2DecryptOAEP is the one example).
- **Blinding the private operation**: Go's decryption functions call
  their random parameter "legacy and ignored" (crypto/rsa pkcs1v15.go, Go
  1.26); like Go's bigmod, Xo's bigmod operations are constant time in the
  exponent and modulus, so decryption takes no Rand.
