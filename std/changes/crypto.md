# std/crypto changes

## 2026-10-20: security: hedged ECDSA nonces, the TLS server's signatures too

- Kind: security
- Commit: (filled in after the push)
- Found by: review (Xo Lang lead, decision 0285)
- Go reference: crypto/internal/fips140/ecdsa Sign and hmacdrbg.go (go1.26.1)
- Test: std/native/crypto_test.xo (Go's signatures for given entropy, byte for byte; an all-zero entropy never repeats r); internal/llvm/testdata/golden/crypto_keys

ECDSA signing (std/native/llrt/nistec.xo `sign`, which the TLS server's
CertificateVerify uses) took its nonce from the system's generator alone;
a weak or repeating generator could leak the private key. The nonce now
comes from Go's hedged construction, an HMAC_DRBG over SHA-512 seeded
with the generator's bytes, the private key, and the message hash.

## 2026-10-20: feature: public key signatures, keys, PEM, and JWK

- Kind: feature
- Commit: (filled in after the push)
- Found by: survey
- Go reference: crypto/rsa, crypto/ecdsa, crypto/ed25519, crypto/x509 (key parsing), encoding/pem (go1.26.1); RFC 7517, RFC 8037
- Test: std/native/crypto_test.xo (pem_test.go's inputs and random ones; 610 public and 531 private keys, generated, altered, and mutated; Go's signatures and 706 verdicts); internal/llvm/testdata/golden/crypto_keys; bench/ct

Decision 0285: RSA PKCS #1 v1.5 and PSS, ECDSA on P-224 to P-521 (DER
and the JWS raw form), and Ed25519 signing and verifying; PKIX, PKCS #1,
PKCS #8, and SEC 1 keys with Go's checks and texts; PEM; JWK public keys;
ECDSA and Ed25519 key generation, on every backend.

## 2026-10-20: feature: Hash, digest, hmac, and a streaming Hasher

- Kind: feature
- Commit: 4c5b0df2
- Found by: survey
- Go reference: crypto/md5/md5.go and md5block.go (go1.26.1), crypto/hmac/hmac.go
- Test: std/native/crypto_test.xo (160 Go digest and HMAC cases); std/native/llrt/md5_test.xo (Go's md5_test.go table); internal/llvm/testdata/golden/crypto_hash

MD5, SHA-1, SHA-224, SHA-256, SHA-384, SHA-512, SHA3-256, and SHA3-512
digests, HMAC over any of them, and a Hasher value for data that
arrives in pieces, with Go's results on every backend. MD5 is ported
from Go; the others are the native TLS stack's own (SHA-224 added).
