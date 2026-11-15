# std/hpke changes

## 2026-11-15: feature: std/hpke, Hybrid Public Key Encryption

- Kind: feature
- Commit: this push
- Found by: Lang 3's crypto and TLS gap audit (C13), for TLS ECH (T13)
- Go reference: crypto/hpke, go1.26.11
- Test: corpus hpke (Go's testdata/rfc9180.json and hpke-pq.json base mode vectors, DeriveKeyPair, messages Go sealed, Go's errors, TestRoundTrip), native, go and llvm; 8 of 8 mutants caught
- Shipped in: not yet released

New module: RFC 9180's base mode with DHKEM over P-256, P-384, P-521 and X25519, ML-KEM-768 and ML-KEM-1024, and the hybrids ML-KEM-768 with X25519 or P-256 and ML-KEM-1024 with P-384; HKDF-SHA256/384/512 and the SHAKE128 and SHAKE256 one stage KDFs; AES-128-GCM, AES-256-GCM, ChaCha20-Poly1305 and export only. Sender and Recipient are move only, so a copy cannot seal twice under one nonce.
