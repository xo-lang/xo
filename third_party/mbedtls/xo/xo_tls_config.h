// xo_tls_config.h: the Xo runtime's changes to mbedTLS's default TLS and
// X.509 configuration (MBEDTLS_USER_CONFIG_FILE, decision 0105). The
// native std/http speaks TLS 1.2 and 1.3 over TCP with ECDHE key
// exchanges, as Go's crypto/tls does by default; what it never uses is
// left out of the binary.

// no DTLS, no PSK key exchanges, no EC J-PAKE
#undef MBEDTLS_SSL_PROTO_DTLS
#undef MBEDTLS_SSL_DTLS_ANTI_REPLAY
#undef MBEDTLS_SSL_DTLS_CLIENT_PORT_REUSE
#undef MBEDTLS_SSL_DTLS_CONNECTION_ID
#undef MBEDTLS_SSL_DTLS_HELLO_VERIFY
#undef MBEDTLS_SSL_COOKIE_C
#undef MBEDTLS_KEY_EXCHANGE_ECDHE_PSK_ENABLED
#undef MBEDTLS_KEY_EXCHANGE_PSK_ENABLED

// Session resumption as Go's crypto/tls does it by default (decision
// 0117): the server sends session tickets (MBEDTLS_SSL_TICKET_C, TLS 1.3
// resumption with a fresh ECDHE only, PSK_EPHEMERAL), keeps no session ID
// cache, and never takes pure PSK (psk_ke) or early data. The client
// keeps no sessions (net/http's default Transport has no
// ClientSessionCache) and drops the tickets servers send.
#undef MBEDTLS_SSL_CACHE_C
#undef MBEDTLS_SSL_TLS1_3_KEY_EXCHANGE_MODE_PSK_ENABLED
#undef MBEDTLS_SSL_CONTEXT_SERIALIZATION
#undef MBEDTLS_SSL_RENEGOTIATION
#undef MBEDTLS_SSL_KEYING_MATERIAL_EXPORT
#undef MBEDTLS_SSL_MAX_FRAGMENT_LENGTH

// I/O, timing, and error texts are the runtime's (xo_tls.c maps errors to
// the texts of Go's crypto/tls)
#undef MBEDTLS_NET_C
#undef MBEDTLS_TIMING_C
#undef MBEDTLS_DEBUG_C
#undef MBEDTLS_ERROR_C
#undef MBEDTLS_VERSION_FEATURES

// certificates are parsed and verified, never written
#undef MBEDTLS_PKCS7_C
#undef MBEDTLS_X509_CREATE_C
#undef MBEDTLS_X509_CRT_WRITE_C
#undef MBEDTLS_X509_CSR_PARSE_C
#undef MBEDTLS_X509_CSR_WRITE_C
#undef MBEDTLS_X509_CRL_PARSE_C
