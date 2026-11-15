# Wycheproof test vectors

From [C2SP/wycheproof](https://github.com/C2SP/wycheproof), `testvectors_v1/`
at commit d17cc89fa5c8 (2026-10-29): `aes_gcm_test.json`,
`chacha20_poly1305_test.json`, and `hkdf_sha1/256/384/512_test.json`, the
files Go's crypto tests run (crypto/internal/cryptotest, x/crypto), and
`ecdsa_secp256r1_sha256_test.json` (its edge cases reach the special
cases of P-256's point formulas). Apache
License 2.0, Copyright Google LLC and the Wycheproof authors. Read by
scripts/stdcorpus/aead.go to generate internal/stdtest/testdata/corpus/aead
(the ECDSA file: ecdsaraw.go, corpus ecdsaraw);
not edited.
