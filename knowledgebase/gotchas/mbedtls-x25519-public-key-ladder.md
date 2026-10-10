# Mbed TLS computes X25519 public keys without HACL*

Status: Observed (2026-10-10, decision 0117)

Symptom: with Everest (HACL* X25519, `MBEDTLS_ECDH_VARIANT_EVEREST_ENABLED`)
on, a native HTTPS server still spent 41 percent of each TLS 1.3 handshake
in `psa_export_public_key` (`ecp_mul`, `ecp_double_add_mxz`, bignum
multiplications and a binary GCD), against 5 percent in HACL*'s scalar
multiplication.

Cause: Mbed TLS 4.1 routes only the X25519 key agreement
(`mbedtls_psa_key_agreement_ecdh`) to Everest. The key share's public key
(TLS 1.3 generates a key pair and exports its public half for every
handshake) goes through the generic Montgomery ladder on heap bignums,
about 9 times slower.

Fix: `third_party/mbedtls/xo/xo_tls_accel.c` defines
`mbedtls_psa_ecp_export_public_key` in front of the library's (renamed by
`mbedtls.FileDefines`) and computes X25519(k, 9) with
`mbedtls_x25519_scalarmult`. TestTLSAccel checks it against RFC 7748 and
the library's own function. Profile a handshake path (`sample` on macOS,
`bench/perf/http/run.sh` with `SCHEME=https NEWCONN=1`) after a library
update: a configured accelerator may cover only part of an operation.
