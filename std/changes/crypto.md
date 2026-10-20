# std/crypto changes

## 2026-10-20: feature: Hash, digest, hmac, and a streaming Hasher

- Kind: feature
- Commit: (filled in after the push)
- Found by: survey
- Go reference: crypto/md5/md5.go and md5block.go (go1.26.1), crypto/hmac/hmac.go
- Test: std/native/crypto_test.xo (160 Go digest and HMAC cases); std/native/llrt/md5_test.xo (Go's md5_test.go table); internal/llvm/testdata/golden/crypto_hash

MD5, SHA-1, SHA-224, SHA-256, SHA-384, SHA-512, SHA3-256, and SHA3-512
digests, HMAC over any of them, and a Hasher value for data that
arrives in pieces, with Go's results on every backend. MD5 is ported
from Go; the others are the native TLS stack's own (SHA-224 added).
