# Mbed TLS: an RSA key in PKCS#8 fails mbedtls_pk_check_pair

Status: Observed (2026-10-15, reported by Xo Lang 2 from
http_tls_keys under XO_LLRT_C=tlssrv. Main fails the same way, since
its http.tls_cert is all C.)

Symptom: `http.tls_cert` on the Mbed path (XO_LLRT_C=tlssrv, or main's
C) rejects a matching RSA certificate and PKCS#8 key with "tls: private
key does not match public key". The same key as PKCS#1 is accepted;
openssl says the pair matches and the inner RSAPrivateKey is byte for
byte the same.

Cause: in TF-PSA-Crypto 1.1.1 (third_party/mbedtls,
extras/pkparse.c), mbedtls_pk_parse_key's PKCS#1 fallback calls
`mbedtls_pk_set_pubkey_from_prv` after `mbedtls_pk_rsa_set_key`, but
`mbedtls_pk_parse_key_pkcs8_unencrypted_der` does not, for RSA. The EC
path (sec1) sets the public half. `mbedtls_pk_check_pair` then sees
`pub_raw_len == 0` on the private key and fails.

Fix: std/llrt/xo_tls.c's tlssvc_key calls
`mbedtls_pk_set_pubkey_from_prv` (declared there; extras/pk_internal.h,
a no-op when already set) after a successful parse. third_party stays
unpatched. The same function now answers X25519 and X448 PKCS#8 keys as
an unknown key type, as Go's X509KeyPair does.

Still different from Go on the Mbed path: TF-PSA-Crypto has no Ed25519,
so Ed25519 certificates are "x509: malformed certificate" and Ed25519
keys "tls: failed to parse private key". http_tls_keys expects Go's
texts, so it passes only on the Xo server (the default).
