// xo_tls.c: TLS for the native std/http (decision 0105), included at the
// end of xo_http.c.
//
// The server (Server.with_tls) and the client (https URLs, Client.with_roots)
// speak TLS 1.2 and 1.3 through Mbed TLS 4.1 (third_party/mbedtls), with
// what Go's crypto/tls and net/http do by default as the reference:
//
//   - certificates: http.tls_cert checks a PEM chain and key as
//     tls.X509KeyPair does, with its error texts;
//   - the server: ALPN "h2" or "http/1.1" (HTTP/2, std/native/llrt/h2srv.xo, decision
//     0121, preferred as net/http's server prefers it), the first certificate
//     unless SNI names another one's DNS name (getCertificate), no client
//     certificates, session tickets (TLS 1.3 psk_dhe_ke and TLS 1.2,
//     decision 0117); a client that sends plain HTTP gets
//     net/http's "400 Client sent an HTTP request to an HTTPS server";
//     handshake failures are not logged (std/rt drops net/http's log line
//     too);
//   - the client: ALPN "h2" and "http/1.1", SNI with the URL's host name (not for IP addresses),
//     verification against the system's roots (Linux: Go's bundle files and
//     directories, SSL_CERT_FILE and SSL_CERT_DIR; macOS: the Security
//     framework's anchors) or the PEM of Client.with_roots, the host name
//     checked as x509.Certificate.VerifyHostname does, a 10 s handshake
//     timeout (http.DefaultTransport's), and Go's error texts:
//     "tls: failed to verify certificate: x509: ...", "remote error: tls:
//     ...", "http: server gave HTTP response to HTTPS client".
//
// Mbed TLS calls the runtime's socket functions through the bio_*
// callbacks below, which park the calling fiber while a socket is not
// ready (pd_wait), so a handshake waits like any other socket operation.
//
// Without XO_TLS (a program that does not use std/http, internal/build's
// usesTLS) only stubs are compiled and no TLS code is linked.

#ifdef XO_TLS

#define MBEDTLS_ALLOW_PRIVATE_ACCESS
#include "mbedtls/ssl.h"
#include "mbedtls/ssl_ticket.h"
#include "mbedtls/x509_crt.h"
#include "mbedtls/pk.h"
#include "mbedtls/oid.h"
#include "psa/crypto.h"
#ifndef _WIN32 // Windows: xo_win.c
#include <dirent.h>
#include <sys/stat.h>
#endif
#ifdef __APPLE__
#include <Security/Security.h>
#endif

// Errors the bio callbacks return to Mbed TLS (its own codes for these
// are in net_sockets.h, which the runtime does not build).
#define TLS_ERR_RECV (-0x004C)
#define TLS_ERR_SEND (-0x004E)

static pthread_once_t tls_once = PTHREAD_ONCE_INIT;
static int tls_ready;

// third_party/mbedtls/xo/xo_tls_accel.c (decision 0117)
void xo_tls_threading_init(void);

static void tls_svc_init(void);

static void tls_init_once(void) {
	tls_svc_init();
	xo_tls_threading_init();
	tls_ready = psa_crypto_init() == PSA_SUCCESS;
	// mbedtls_ssl_config_defaults fills a global list of cipher suites the
	// first time without a lock (ThreadSanitizer found the race): fill it
	// here, once
	(void)mbedtls_ssl_list_ciphersuites();
	// SHA-256 and SHA-512 check once whether the CPU has the ARMv8
	// instructions (decision 0117) and keep the answer in statics without
	// a lock: hash once here so every later hash only reads them
	unsigned char h[64];
	size_t hn;
	(void)psa_hash_compute(PSA_ALG_SHA_256, (const uint8_t *)"", 0, h, sizeof h, &hn);
	(void)psa_hash_compute(PSA_ALG_SHA_512, (const uint8_t *)"", 0, h, sizeof h, &hn);
}

static int tls_init(void) {
	pthread_once(&tls_once, tls_init_once);
	return tls_ready;
}

// The cipher suites and groups of Go's crypto/tls defaults (Go 1.26:
// defaultCipherSuites, TLS 1.3's three, X25519 and the NIST curves; Go's
// X25519MLKEM768 has no Mbed TLS counterpart).
static const int tls_suites[] = {
	MBEDTLS_TLS1_3_AES_128_GCM_SHA256,
	MBEDTLS_TLS1_3_AES_256_GCM_SHA384,
	MBEDTLS_TLS1_3_CHACHA20_POLY1305_SHA256,
	MBEDTLS_TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	MBEDTLS_TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	MBEDTLS_TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	MBEDTLS_TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	MBEDTLS_TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
	MBEDTLS_TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
	MBEDTLS_TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
	MBEDTLS_TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
	MBEDTLS_TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
	MBEDTLS_TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
	0,
};

static const uint16_t tls_groups[] = {
	MBEDTLS_SSL_IANA_TLS_GROUP_X25519,
	MBEDTLS_SSL_IANA_TLS_GROUP_SECP256R1,
	MBEDTLS_SSL_IANA_TLS_GROUP_SECP384R1,
	MBEDTLS_SSL_IANA_TLS_GROUP_SECP521R1,
	MBEDTLS_SSL_IANA_TLS_GROUP_NONE,
};

// ALPN: HTTP/2 first, as net/http's server and client offer it (decision
// 0121)
static const char *tls_alpn[] = {"h2", "http/1.1", NULL};

static void tls_conf_common(mbedtls_ssl_config *conf) {
	mbedtls_ssl_conf_min_tls_version(conf, MBEDTLS_SSL_VERSION_TLS1_2);
	mbedtls_ssl_conf_max_tls_version(conf, MBEDTLS_SSL_VERSION_TLS1_3);
	mbedtls_ssl_conf_ciphersuites(conf, tls_suites);
	mbedtls_ssl_conf_groups(conf, tls_groups);
}

// ---------------------------------------------------------------------------
// Certificates (http.tls_cert)
//
// The PEM scan, tls.X509KeyPair's checks and their error texts, the
// roots of Client.with_roots and of the system, the expiry texts, and
// macOS's chain verification are std/native/llrt/tlscert.xo's (the
// zero-C audit's B30); it calls Mbed TLS through tls_svc (its V_
// words), and reads mbedtls_x509_crt at the offsets asserted here.

_Static_assert(offsetof(mbedtls_x509_crt, raw.len) == 16, "tlscert.xo CRT_RAW_LEN");
_Static_assert(offsetof(mbedtls_x509_crt, raw.p) == 24, "tlscert.xo CRT_RAW_P");
_Static_assert(offsetof(mbedtls_x509_crt, next) == 1288, "tlscert.xo CRT_NEXT");
_Static_assert(offsetof(mbedtls_x509_crt, valid_from) == 288, "tlscert.xo CRT_VALID_FROM");
_Static_assert(offsetof(mbedtls_x509_crt, valid_to) == 312, "tlscert.xo CRT_VALID_TO");
_Static_assert(sizeof(mbedtls_x509_time) == 24 && offsetof(mbedtls_x509_time, sec) == 20, "tlscert.xo x509_time");

int64_t xo_x_tls_cert_check(int64_t svc, int64_t c, int64_t crt, int64_t cp, int64_t cn, int64_t kp, int64_t kn);
int64_t xo_x_tls_roots_pem(int64_t svc, int64_t crt, int64_t p, int64_t n);
int64_t xo_x_tls_roots_system(int64_t svc, int64_t crt);
int64_t xo_x_tls_expiry_msg(int64_t now, int64_t off, int64_t tp, int64_t future, int64_t buf, int64_t bn);
int64_t xo_x_tls_apple_verify(int64_t svc, int64_t crt, int64_t hp, int64_t hn, int64_t msg, int64_t mn);

static int64_t tls_svc[7 + 20];

typedef struct {
	xo_hdr h;
	char *err; // the error, NULL for a certificate
	mbedtls_x509_crt crt;
	mbedtls_pk_context pk;
} xo_tcert;

static void tcert_free(void *p) {
	xo_tcert *c = p;
	free(c->err);
	mbedtls_x509_crt_free(&c->crt);
	mbedtls_pk_free(&c->pk);
	free(c);
}

// tls_svc's words: the chain's parse of a DER (args: chain, DER, length)
static int64_t tlssvc_crt_der(int64_t a, int64_t b) {
	(void)b;
	int64_t *v = (int64_t *)(intptr_t)a;
	return mbedtls_x509_crt_parse_der((mbedtls_x509_crt *)(intptr_t)v[0], (const unsigned char *)(intptr_t)v[1], (size_t)v[2]);
}

// the key's parse into c (args: DER, length): 0, 1 for an unknown key
// algorithm, 2 for another error
static int64_t tlssvc_key(int64_t c, int64_t a) {
	int64_t *v = (int64_t *)(intptr_t)a;
	int r = mbedtls_pk_parse_key(&((xo_tcert *)(intptr_t)c)->pk, (const unsigned char *)(intptr_t)v[0], (size_t)v[1], NULL, 0);
	return r == 0 ? 0 : r == MBEDTLS_ERR_PK_UNKNOWN_PK_ALG ? 1 : 2;
}

// the key types of c: 1 the leaf's public key is RSA, 2 EC; 4 the
// private key is RSA, 8 EC
static int64_t tlssvc_kinds(int64_t cp, int64_t b) {
	(void)b;
	xo_tcert *c = (xo_tcert *)(intptr_t)cp;
	psa_key_type_t pt = mbedtls_pk_get_key_type(&c->crt.pk), kt = mbedtls_pk_get_key_type(&c->pk);
	return (PSA_KEY_TYPE_IS_RSA(pt) ? 1 : 0) | (PSA_KEY_TYPE_IS_ECC(pt) ? 2 : 0) | (PSA_KEY_TYPE_IS_RSA(kt) ? 4 : 0) | (PSA_KEY_TYPE_IS_ECC(kt) ? 8 : 0);
}

static int64_t tlssvc_pair(int64_t cp, int64_t b) {
	(void)b;
	xo_tcert *c = (xo_tcert *)(intptr_t)cp;
	return mbedtls_pk_check_pair(&c->crt.pk, &c->pk) != 0;
}

// xo_tls_cert is `http.tls_cert(cert_pem, key_pem)`: tls.X509KeyPair.
xo_tcert *xo_tls_cert(const char *cp, int64_t cn, const char *kp, int64_t kn) {
	xo_tcert *c = calloc(1, sizeof(xo_tcert));
	if (!c) {
		xo_oom();
	}
	h_init(c, tcert_free);
	mbedtls_x509_crt_init(&c->crt);
	mbedtls_pk_init(&c->pk);
	if (!tls_init()) {
		c->err = strdup("tls: the TLS library failed to start");
		return c;
	}
	c->err = (char *)(intptr_t)xo_x_tls_cert_check(XO_ADDR(tls_svc), XO_ADDR(c), XO_ADDR(&c->crt), XO_ADDR(cp), cn, XO_ADDR(kp), kn);
	return c;
}

// xo_tls_cert_err is the error of tls_cert, "" for a certificate.
xo_rstr *xo_tls_cert_err(xo_tcert *c, int64_t *len) { return nstr(c->err ? c->err : "", c->err ? (int64_t)strlen(c->err) : 0, len); }

// ---------------------------------------------------------------------------
// Roots (Client.with_roots and the system's)

struct xo_troots {
	xo_hdr h;
	char *err;
	int n; // certificates parsed
	mbedtls_x509_crt crt;
	mbedtls_ssl_config conf; // the client's configuration with these roots
	int conf_ok;
};

static void troots_free(void *p) {
	xo_troots *r = p;
	free(r->err);
	if (r->conf_ok) {
		mbedtls_ssl_config_free(&r->conf);
	}
	mbedtls_x509_crt_free(&r->crt);
	free(r);
}

// roots_conf makes the client configuration of roots r.
static void roots_conf(xo_troots *r) {
	mbedtls_ssl_config_init(&r->conf);
	if (mbedtls_ssl_config_defaults(&r->conf, MBEDTLS_SSL_IS_CLIENT, MBEDTLS_SSL_TRANSPORT_STREAM, MBEDTLS_SSL_PRESET_DEFAULT) != 0) {
		mbedtls_ssl_config_free(&r->conf);
		return;
	}
	tls_conf_common(&r->conf);
	mbedtls_ssl_conf_authmode(&r->conf, MBEDTLS_SSL_VERIFY_REQUIRED);
	mbedtls_ssl_conf_alpn_protocols(&r->conf, tls_alpn);
	if (r->n > 0) {
		mbedtls_ssl_conf_ca_chain(&r->conf, &r->crt, NULL);
	}
	r->conf_ok = 1;
}

// roots_add_pem adds the CERTIFICATE blocks of a PEM text that parse
// (CertPool.AppendCertsFromPEM, tlscert.xo).
static void roots_add_pem(xo_troots *r, const char *p, size_t n) { r->n += (int)xo_x_tls_roots_pem(XO_ADDR(tls_svc), XO_ADDR(&r->crt), XO_ADDR(p), (int64_t)n); }

// xo_tls_roots is `Client.with_roots(pem)`: the certificates of pem
// replace the system's roots.
xo_troots *xo_tls_roots(const char *p, int64_t n) {
	xo_troots *r = calloc(1, sizeof(xo_troots));
	if (!r) {
		xo_oom();
	}
	h_init(r, troots_free);
	mbedtls_x509_crt_init(&r->crt);
	if (!tls_init()) {
		r->err = strdup("tls: the TLS library failed to start");
		return r;
	}
	roots_add_pem(r, p, (size_t)n);
	if (r->n == 0) {
		r->err = strdup("tls: no certificates found in PEM input");
	}
	roots_conf(r);
	return r;
}

xo_rstr *xo_tls_roots_err(xo_troots *r, int64_t *len) { return nstr(r->err ? r->err : "", r->err ? (int64_t)strlen(r->err) : 0, len); }

static xo_troots sys_roots = {.h = {XO_RC_IMMORTAL, NULL}};
static pthread_once_t sys_once = PTHREAD_ONCE_INIT;

// sys_roots_load reads the system's roots once (tlscert.xo
// roots_system: the Security framework's on macOS, the ROOT store on
// Windows, crypto/x509's files and directories on Linux).
static void sys_roots_load(void) {
	xo_troots *r = &sys_roots;
	mbedtls_x509_crt_init(&r->crt);
	if (!tls_init()) {
		return;
	}
	r->n = (int)xo_x_tls_roots_system(XO_ADDR(tls_svc), XO_ADDR(&r->crt));
	roots_conf(r);
}

// xo_tls_roots_system is the client's default: the system's roots (a
// handle that is never freed), loaded at the first https request.
xo_troots *xo_tls_roots_system(void) { return &sys_roots; }

// xo_tls_c_apple_svc is the Security framework's table (tls_svc) when
// roots r are the system's on macOS, where Go asks the framework to verify
// (tlscert.xo apple_trust, for tlsio.xo); else 0.
int64_t xo_tls_c_apple_svc(int64_t p) {
#ifdef __APPLE__
	if ((xo_troots *)(intptr_t)p == &sys_roots && tls_init()) {
		return XO_ADDR(tls_svc);
	}
#endif
	(void)p;
	return 0;
}

// xo_tls_c_roots_crt is the certificate chain of roots r for the Xo
// client (tlsio.xo), the system's loaded first: NULL when it has none.
int64_t xo_tls_c_roots_crt(int64_t p) {
	xo_troots *r = (xo_troots *)(intptr_t)p;
	if (r == &sys_roots) {
		pthread_once(&sys_once, sys_roots_load);
	}
	return r->n > 0 ? XO_ADDR(&r->crt) : 0;
}

// ---------------------------------------------------------------------------
// Connections

struct tls_conn {
	mbedtls_ssl_context ssl;
	xo_pd *pd;
	int64_t when;   // deadline of socket waits (monotonic), 0: none
	int status;     // why the last bio call failed: CC_TIMEOUT, CC_CANCELED, CC_ERR
	int handshaken; // 1 done, -1 failed
	unsigned char first[5];
	int nfirst; // the first bytes received (HTTP detection)
	// the client's verification
	const char *host; // the URL's host name (no brackets)
	int host_ip;
	uint32_t upper; // flags of the chain above the leaf
	int apple;      // macOS, the system's roots: apple_verify decides
	char *alert;    // a fatal alert received after the handshake
	int probed;     // tls_alert read once more
	char *vmsg;     // Go's x509 error text
	int nb;         // reads do not wait (tls_nb_read)
};

static void alert_msg(tls_conn *t, char *m, size_t mn);

static int64_t tls_read(tls_conn *t, char *p, int64_t n);

// tls_alert is the text of a fatal alert the peer sent after the
// handshake (Go: a net.OpError "remote error: tls: ..."), or NULL. After
// a failed exchange it reads once more, for at most 100 ms: a TLS 1.3
// server that rejects the client's certificate sends its alert after the
// handshake and closes, and the client's request may fail to go out
// before the alert is read (seen on Linux under x86-64 emulation).
static const char *tls_alert(tls_conn *t) {
	if (!t) {
		return NULL;
	}
	if (!t->alert && !t->probed && t->handshaken == 1) {
		t->probed = 1;
		int64_t d = deadline((int64_t)100 * 1000000);
		if (!t->when || t->when > d) {
			t->when = d;
		}
		t->status = 0;
		char b[512];
		(void)tls_read(t, b, sizeof b);
	}
	return t->alert;
}

static int bio_send(void *p, const unsigned char *buf, size_t len) {
	tls_conn *t = p;
	xo_ctx *c = ctx();
	if (!pd_begin(t->pd)) {
		t->status = CC_ERR;
		return TLS_ERR_SEND;
	}
	for (;;) {
		int64_t w = x_write(t->pd->fd, buf, len);
		if (w >= 0) {
			pd_end(t->pd);
			return (int)w;
		}
		if (w == -EINTR) {
			continue;
		}
		if (w == -EAGAIN || w == -EWOULDBLOCK) {
			int r = pd_wait_until(t->pd, 1, c, t->when);
			if (r == PD_READY) {
				continue;
			}
			t->status = r == PD_TIMEOUT ? CC_TIMEOUT : r == PD_CANCELED ? CC_CANCELED : CC_ERR;
		} else {
			t->status = CC_ERR;
		}
		pd_end(t->pd);
		return TLS_ERR_SEND;
	}
}

static int bio_recv(void *p, unsigned char *buf, size_t len) {
	tls_conn *t = p;
	xo_ctx *c = ctx();
	if (ctx_canceled(c)) {
		t->status = CC_CANCELED;
		return TLS_ERR_RECV;
	}
	if (!pd_begin(t->pd)) {
		t->status = CC_ERR;
		return TLS_ERR_RECV;
	}
	for (;;) {
		int64_t r = x_read(t->pd->fd, buf, len);
		if (r >= 0) {
			pd_end(t->pd);
			for (int64_t i = 0; i < r && t->nfirst < 5; i++) {
				t->first[t->nfirst++] = buf[i];
			}
			return (int)r;
		}
		if (r == -EINTR) {
			continue;
		}
		if ((r == -EAGAIN || r == -EWOULDBLOCK) && t->nb) {
			// the HTTP/2 connection's fiber waits itself (h2srv.xo)
			pd_end(t->pd);
			return MBEDTLS_ERR_SSL_WANT_READ;
		}
		if (r == -EAGAIN || r == -EWOULDBLOCK) {
			int w = pd_wait_until(t->pd, 0, c, t->when);
			if (w == PD_READY) {
				continue;
			}
			t->status = w == PD_TIMEOUT ? CC_TIMEOUT : w == PD_CANCELED ? CC_CANCELED : CC_ERR;
		} else {
			t->status = CC_ERR;
		}
		pd_end(t->pd);
		return TLS_ERR_RECV;
	}
}

static tls_conn *tls_conn_new(const mbedtls_ssl_config *conf, xo_pd *pd) {
	tls_conn *t = calloc(1, sizeof(tls_conn));
	if (!t) {
		xo_oom();
	}
	t->pd = pd;
	mbedtls_ssl_init(&t->ssl);
	if (mbedtls_ssl_setup(&t->ssl, conf) != 0) {
		xo_oom();
	}
	mbedtls_ssl_set_bio(&t->ssl, t, bio_send, bio_recv, NULL);
	return t;
}

static void tls_conn_free(tls_conn *t) {
	if (!t) {
		return;
	}
	mbedtls_ssl_free(&t->ssl);
	free(t->vmsg);
	free(t->alert);
	free(t);
}

// tls_read reads decrypted bytes: their count, 0 at the end of the
// stream, or minus why it failed (CC_TIMEOUT, CC_CANCELED, CC_ERR).
static int64_t tls_read(tls_conn *t, char *p, int64_t n) {
	if (n > INT32_MAX) {
		n = INT32_MAX;
	}
	for (;;) {
		int r = mbedtls_ssl_read(&t->ssl, (unsigned char *)p, (size_t)n);
		if (r > 0) {
			return r;
		}
		if (r == 0 || r == MBEDTLS_ERR_SSL_PEER_CLOSE_NOTIFY || r == MBEDTLS_ERR_SSL_CONN_EOF) {
			return 0;
		}
		if (r == MBEDTLS_ERR_SSL_RECEIVED_NEW_SESSION_TICKET || r == MBEDTLS_ERR_SSL_WANT_READ || r == MBEDTLS_ERR_SSL_WANT_WRITE) {
			continue;
		}
		if (r == MBEDTLS_ERR_SSL_FATAL_ALERT_MESSAGE && !t->alert) {
			char m[120];
			alert_msg(t, m, sizeof m);
			t->alert = strdup(m);
		}
		if (!t->status) {
			t->status = CC_ERR;
		}
		return -(int64_t)t->status;
	}
}

// tls_nb_read is tls_read without waiting for the socket: the bytes
// read, 0 at the end of the stream, -1 when it failed, -2 when no record
// is complete yet.
static int64_t tls_nb_read(tls_conn *t, char *p, int64_t n) {
	if (n > INT32_MAX) {
		n = INT32_MAX;
	}
	for (;;) {
		t->nb = 1;
		int r = mbedtls_ssl_read(&t->ssl, (unsigned char *)p, (size_t)n);
		t->nb = 0;
		if (r > 0) {
			return r;
		}
		if (r == 0 || r == MBEDTLS_ERR_SSL_PEER_CLOSE_NOTIFY || r == MBEDTLS_ERR_SSL_CONN_EOF) {
			return 0;
		}
		if (r == MBEDTLS_ERR_SSL_WANT_READ) {
			return -2;
		}
		if (r == MBEDTLS_ERR_SSL_RECEIVED_NEW_SESSION_TICKET || r == MBEDTLS_ERR_SSL_WANT_WRITE) {
			continue;
		}
		return -1;
	}
}

// tls_set_when sets the deadline of t's socket waits (a request on a
// kept HTTP/2 connection has its own).
static void tls_set_when(tls_conn *t, int64_t when) {
	if (t) {
		t->when = when;
		t->status = 0;
	}
}

// tls_alpn_h2 tells whether ALPN agreed on HTTP/2.
static int tls_alpn_h2(tls_conn *t) {
	const char *p = t ? mbedtls_ssl_get_alpn_protocol(&t->ssl) : NULL;
	return p && strcmp(p, "h2") == 0;
}

// tls_h2_bad_cipher tells whether the connection's cipher suite is one
// HTTP/2 prohibits (RFC 9113 Appendix A: TLS 1.2 without an AEAD; Go's
// isBadCipher), with net/http's GOAWAY text.
static int tls_h2_bad_cipher(tls_conn *t, char *why, size_t wn) {
	if (mbedtls_ssl_get_version_number(&t->ssl) != MBEDTLS_SSL_VERSION_TLS1_2) {
		return 0;
	}
	int id = mbedtls_ssl_get_ciphersuite_id_from_ssl(&t->ssl);
	switch (id) {
	case MBEDTLS_TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256:
	case MBEDTLS_TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256:
	case MBEDTLS_TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384:
	case MBEDTLS_TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384:
	case MBEDTLS_TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256:
	case MBEDTLS_TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256:
		return 0;
	}
	snprintf(why, wn, "Prohibited TLS 1.2 Cipher Suite: %x", (unsigned)id);
	return 1;
}

// tls_write writes all of p: CC_OK or why it failed.
static int tls_write(tls_conn *t, const char *p, int64_t n) {
	int64_t off = 0;
	while (off < n) {
		int64_t k = n - off;
		if (k > (1 << 20)) {
			k = 1 << 20;
		}
		int r = mbedtls_ssl_write(&t->ssl, (const unsigned char *)p + off, (size_t)k);
		if (r > 0) {
			off += r;
			continue;
		}
		if (r == MBEDTLS_ERR_SSL_WANT_READ || r == MBEDTLS_ERR_SSL_WANT_WRITE) {
			continue;
		}
		return t->status ? t->status : CC_ERR;
	}
	return CC_OK;
}

// tls_close_notify ends a connection whose handshake completed with a
// close_notify alert (tls.Conn.Close), without waiting for the peer.
static void tls_close_notify(tls_conn *t) {
	if (t && t->handshaken == 1) {
		t->when = deadline((int64_t)5 * 1000000000);
		mbedtls_ssl_close_notify(&t->ssl);
	}
}

// ---------------------------------------------------------------------------
// The server

struct tls_srv {
	mbedtls_ssl_config conf;
	mbedtls_ssl_ticket_context ticket; // session tickets (decision 0117)
	xo_tcert **certs;
	int n;
};

// The name checks (x509's matchHostnames, validHostname, matchExactly,
// VerifyHostname, HostnameError; net.ParseIP and net.IP.String) are Xo
// (std/native/llrt/tlsname.xo through cabi.xo, decision 0207): the C
// reads a certificate's names and hands them over flat.
int64_t xo_x_tls_host_ok(int64_t dp, int64_t dn, int64_t ip, int64_t ni, int64_t hp, int64_t hn);
int64_t xo_x_tls_hostname_msg(int64_t args, int64_t buf, int64_t bn);
int64_t xo_x_tls_is_ip(int64_t p, int64_t n);
int64_t xo_x_tls_alert_msg(int64_t buf, int64_t bn, int64_t a);

// The names of a certificate's subject alternative name extension.
typedef struct {
	int has_san;
	int ndns, nip;
	char *dns[100];
	unsigned char ip[100][16];
	int iplen[100];
} cert_names;

static void names_load(const mbedtls_x509_crt *crt, cert_names *nm) {
	memset(nm, 0, sizeof *nm);
	nm->has_san = (crt->ext_types & MBEDTLS_X509_EXT_SUBJECT_ALT_NAME) != 0;
	for (const mbedtls_x509_sequence *s = &crt->subject_alt_names; s && s->buf.p; s = s->next) {
		mbedtls_x509_subject_alternative_name san;
		if (mbedtls_x509_parse_subject_alt_name(&s->buf, &san) != 0) {
			continue;
		}
		if (san.type == MBEDTLS_X509_SAN_DNS_NAME && nm->ndns < 100) {
			nm->dns[nm->ndns++] = cstr_of((const char *)san.san.unstructured_name.p, (int64_t)san.san.unstructured_name.len);
		} else if (san.type == MBEDTLS_X509_SAN_IP_ADDRESS && nm->nip < 100 && (san.san.unstructured_name.len == 4 || san.san.unstructured_name.len == 16)) {
			memcpy(nm->ip[nm->nip], san.san.unstructured_name.p, san.san.unstructured_name.len);
			nm->iplen[nm->nip++] = (int)san.san.unstructured_name.len;
		}
		mbedtls_x509_free_subject_alt_name(&san);
	}
}

static void names_free(cert_names *nm) {
	for (int i = 0; i < nm->ndns; i++) {
		free(nm->dns[i]);
	}
}

static void ip_to16(const unsigned char *ip, int n, unsigned char out[16]) {
	if (n == 4) {
		memset(out, 0, 10);
		out[10] = out[11] = 0xff;
		memcpy(out + 12, ip, 4);
	} else {
		memcpy(out, ip, 16);
	}
}

// names_flat is nm as tlsname.names_at reads it: the DNS names each
// followed by a zero byte in dns, the IP addresses as 16 bytes each in ips.
static void names_flat(const cert_names *nm, nbuf *dns, unsigned char ips[100][16]) {
	for (int i = 0; i < nm->ndns; i++) {
		nbuf_put(dns, nm->dns[i], (int64_t)strlen(nm->dns[i]) + 1);
	}
	for (int i = 0; i < nm->nip; i++) {
		ip_to16(nm->ip[i], nm->iplen[i], ips[i]);
	}
}

// host_ok is x509.Certificate.VerifyHostname (tlsname.xo).
static int host_ok(const cert_names *nm, const char *host) {
	nbuf d = {0};
	unsigned char ips[100][16];
	names_flat(nm, &d, ips);
	int ok = (int)xo_x_tls_host_ok(XO_ADDR(d.p), d.n, XO_ADDR(ips), nm->nip, XO_ADDR(host), (int64_t)strlen(host));
	free(d.p);
	return ok;
}

// sni_cb picks the certificate for the name the client sent (Go's
// getCertificate: the first whose leaf is valid for the name, else the
// first).
static int sni_cb(void *p, mbedtls_ssl_context *ssl, const unsigned char *name, size_t n) {
	tls_srv *s = p;
	xo_tcert *pick = s->certs[0];
	if (s->n > 1 && n > 0 && n < 256) {
		char host[256];
		memcpy(host, name, n);
		host[n] = 0;
		for (int i = 0; i < s->n; i++) {
			cert_names nm;
			names_load(&s->certs[i]->crt, &nm);
			int ok = host_ok(&nm, host);
			names_free(&nm);
			if (ok) {
				pick = s->certs[i];
				break;
			}
		}
	}
	return mbedtls_ssl_set_hs_own_cert(ssl, &pick->crt, &pick->pk);
}

static void tls_srv_free(tls_srv *s) {
	if (!s) {
		return;
	}
	mbedtls_ssl_config_free(&s->conf);
	mbedtls_ssl_ticket_free(&s->ticket);
	for (int i = 0; i < s->n; i++) {
		h_release(s->certs[i]);
	}
	free(s->certs);
	free(s);
}

// xo_srv_tls adds a certificate to the server's TLS configuration; the
// first call turns TLS on.
void xo_srv_tls(xo_srv *srv, xo_tcert *c) {
	tls_srv *s = srv->tls;
	if (!s) {
		if (!tls_init()) {
			return;
		}
		s = calloc(1, sizeof(tls_srv));
		if (!s) {
			xo_oom();
		}
		mbedtls_ssl_config_init(&s->conf);
		if (mbedtls_ssl_config_defaults(&s->conf, MBEDTLS_SSL_IS_SERVER, MBEDTLS_SSL_TRANSPORT_STREAM, MBEDTLS_SSL_PRESET_DEFAULT) != 0) {
			xo_oom();
		}
		tls_conf_common(&s->conf);
		mbedtls_ssl_conf_authmode(&s->conf, MBEDTLS_SSL_VERIFY_NONE);
		mbedtls_ssl_conf_alpn_protocols(&s->conf, tls_alpn);
		mbedtls_ssl_conf_sni(&s->conf, sni_cb, s);
		// Session tickets, as Go's server sends them by default
		// (decision 0117): a returning client resumes without the
		// certificate and its signature, in TLS 1.3 still with a fresh
		// ECDHE (psk_dhe_ke; Go offers no psk_ke either). The ticket keys
		// are random per server, AES-256-GCM, and rotate every 24 hours
		// (Go: every 24 hours); a ticket is good for 24 hours (Go: 7 days).
		mbedtls_ssl_ticket_init(&s->ticket);
		if (mbedtls_ssl_ticket_setup(&s->ticket, PSA_ALG_GCM, PSA_KEY_TYPE_AES, 256, 24 * 3600) != 0) {
			xo_oom();
		}
		mbedtls_ssl_conf_session_tickets_cb(&s->conf, mbedtls_ssl_ticket_write, mbedtls_ssl_ticket_parse, &s->ticket);
		mbedtls_ssl_conf_tls13_key_exchange_modes(&s->conf, MBEDTLS_SSL_TLS1_3_KEY_EXCHANGE_MODE_EPHEMERAL | MBEDTLS_SSL_TLS1_3_KEY_EXCHANGE_MODE_PSK_EPHEMERAL);
		srv->tls = s;
	}
	h_retain(c);
	s->certs = realloc(s->certs, sizeof(xo_tcert *) * (size_t)(s->n + 1));
	if (!s->certs) {
		xo_oom();
	}
	s->certs[s->n++] = c;
	if (s->n == 1) {
		mbedtls_ssl_conf_own_cert(&s->conf, &c->crt, &c->pk);
	}
}

static tls_conn *tls_server_conn(tls_srv *s, xo_pd *pd) { return tls_conn_new(&s->conf, pd); }

// tls_server_handshake runs the server's handshake: 1 when it
// succeeded. A client that sent plain HTTP gets net/http's answer.
static int tls_server_handshake(tls_conn *t) {
	if (t->handshaken) {
		return t->handshaken == 1;
	}
	int r;
	do {
		r = mbedtls_ssl_handshake(&t->ssl);
	} while (r == MBEDTLS_ERR_SSL_WANT_READ || r == MBEDTLS_ERR_SSL_WANT_WRITE);
	if (r == 0) {
		t->handshaken = 1;
		return 1;
	}
	t->handshaken = -1;
	// net/http's tlsRecordHeaderLooksLikeHTTP
	static const char *http[] = {"GET /", "HEAD ", "POST ", "PUT /", "OPTIO"};
	if (t->nfirst == 5) {
		for (int i = 0; i < 5; i++) {
			if (memcmp(t->first, http[i], 5) == 0) {
				static const char msg[] = "HTTP/1.0 400 Bad Request\r\n\r\nClient sent an HTTP request to an HTTPS server.\n";
				if (pd_begin(t->pd)) {
					x_write(t->pd->fd, msg, sizeof msg - 1);
					pd_end(t->pd);
				}
				break;
			}
		}
	}
	return 0;
}

// tls_hc_read reads from a server connection, after the handshake: the
// bytes read, 0 at the end, -1 when it failed or closed.
static int64_t tls_hc_read(tls_conn *t, char *p, int64_t n) {
	if (!tls_server_handshake(t)) {
		return -1;
	}
	int64_t r = tls_read(t, p, n);
	return r < 0 ? -1 : r;
}

// ---------------------------------------------------------------------------
// The client

// local_off is the local zone's offset from UTC at t, in seconds.
static long local_off(time_t t) {
	struct tm tm;
	localtime_r(&t, &tm);
#ifdef _WIN32
	// no tm_gmtoff: the local time read back as UTC, minus t
	return (long)(_mkgmtime(&tm) - t);
#else
	return tm.tm_gmtoff;
#endif
}

// hostname_msg checks host against the leaf crt as
// x509.Certificate.VerifyHostname: 0 when it matches, else 1 and the
// text of x509.HostnameError in msg.
static int hostname_msg(const mbedtls_x509_crt *crt, const char *host, char *msg, size_t mn) {
	cert_names nm;
	names_load(crt, &nm);
	const char *cn = "";
	size_t cnl = 0;
	for (const mbedtls_x509_name *n = &crt->subject; n; n = n->next) {
		if (MBEDTLS_OID_CMP(MBEDTLS_OID_AT_CN, &n->oid) == 0 && n->val.len < 256) {
			cn = (const char *)n->val.p;
			cnl = n->val.len;
		}
	}
	nbuf d = {0};
	unsigned char ips[100][16];
	names_flat(&nm, &d, ips);
	int64_t args[9] = {XO_ADDR(d.p), d.n, XO_ADDR(ips), nm.nip, nm.has_san, XO_ADDR(cn), (int64_t)cnl, XO_ADDR(host), (int64_t)strlen(host)};
	int bad = (int)xo_x_tls_hostname_msg(XO_ADDR(args), XO_ADDR(msg), (int64_t)mn);
	free(d.p);
	names_free(&nm);
	return bad;
}

// tls_svc's host name word (args: the leaf, the host's C string, the
// message buffer, its size): 0 when the host matches
static int64_t tlssvc_hostmsg(int64_t a, int64_t b) {
	(void)b;
	int64_t *v = (int64_t *)(intptr_t)a;
	return hostname_msg((const mbedtls_x509_crt *)(intptr_t)v[0], (const char *)(intptr_t)v[1], (char *)(intptr_t)v[2], (size_t)v[3]);
}

// tls_svc's call4 (macOS): the function at v[0] with the four words
// v[1] to v[4] (one with fewer parameters ignores the rest)
static int64_t tlssvc_call4(int64_t a, int64_t b) {
	(void)b;
	int64_t *v = (int64_t *)(intptr_t)a;
	int64_t (*f)(int64_t, int64_t, int64_t, int64_t) = (int64_t (*)(int64_t, int64_t, int64_t, int64_t))(intptr_t)v[0];
	return f(v[1], v[2], v[3], v[4]);
}

static void tls_svc_init(void) {
	tls_svc[0] = XO_ADDR(&tlssvc_crt_der);
	tls_svc[1] = XO_ADDR(&tlssvc_key);
	tls_svc[2] = XO_ADDR(&tlssvc_kinds);
	tls_svc[3] = XO_ADDR(&tlssvc_pair);
	tls_svc[4] = XO_ADDR(&tlssvc_hostmsg);
	tls_svc[6] = XO_ADDR(&tlssvc_call4);
#ifdef __APPLE__
	tls_svc[5] = XO_ADDR(&kCFTypeArrayCallBacks);
	// tlscert.xo's F_ words, in its order
	tls_svc[7] = XO_ADDR(&CFRelease);
	tls_svc[8] = XO_ADDR(&CFArrayCreateMutable);
	tls_svc[9] = XO_ADDR(&CFArrayAppendValue);
	tls_svc[10] = XO_ADDR(&CFArrayGetCount);
	tls_svc[11] = XO_ADDR(&CFArrayGetValueAtIndex);
	tls_svc[12] = XO_ADDR(&CFDataCreate);
	tls_svc[13] = XO_ADDR(&CFDataGetBytePtr);
	tls_svc[14] = XO_ADDR(&CFDataGetLength);
	tls_svc[15] = XO_ADDR(&CFStringCreateWithCString);
	tls_svc[16] = XO_ADDR(&CFStringGetCString);
	tls_svc[17] = XO_ADDR(&CFErrorGetCode);
	tls_svc[18] = XO_ADDR(&CFErrorCopyDescription);
	tls_svc[19] = XO_ADDR(&SecCertificateCopyData);
	tls_svc[20] = XO_ADDR(&SecCertificateCreateWithData);
	tls_svc[21] = XO_ADDR(&SecPolicyCreateSSL);
	tls_svc[22] = XO_ADDR(&SecTrustCreateWithCertificates);
	tls_svc[23] = XO_ADDR(&SecTrustEvaluateWithError);
	tls_svc[24] = XO_ADDR(&SecTrustCopyAnchorCertificates);
	tls_svc[25] = XO_ADDR(&SecTrustSettingsCopyCertificates);
	tls_svc[26] = XO_ADDR(&SecTrustSettingsCopyTrustSettings);
#endif
}

// cli_verify is the client's verification callback: at the leaf (depth
// 0, called last) it decides in the order of x509.Certificate.Verify
// (the leaf's validity period, then the host name, then the chain) and
// keeps Go's text of the first failure. On macOS with the system's roots
// the Security framework decides instead (apple_verify), as Go's
// verifier does there.
static int cli_verify(void *p, mbedtls_x509_crt *crt, int depth, uint32_t *flags) {
	tls_conn *t = p;
	char msg[1024] = "";
#ifdef __APPLE__
	if (t->apple) {
		if (depth > 0) {
			*flags = 0;
			return 0;
		}
		const char *h = t->host ? t->host : "";
		if (xo_x_tls_apple_verify(XO_ADDR(tls_svc), XO_ADDR(crt), XO_ADDR(h), (int64_t)strlen(h), XO_ADDR(msg), (int64_t)sizeof msg)) {
			free(t->vmsg);
			t->vmsg = strdup(msg);
			*flags = MBEDTLS_X509_BADCERT_OTHER;
		} else {
			*flags = 0;
		}
		return 0;
	}
#endif
	if (depth > 0) {
		t->upper |= *flags;
		return 0;
	}
	uint32_t f = *flags & ~(uint32_t)MBEDTLS_X509_BADCERT_CN_MISMATCH;
	if (f & (MBEDTLS_X509_BADCERT_EXPIRED | MBEDTLS_X509_BADCERT_FUTURE)) {
		time_t now = time(NULL);
		int future = (f & MBEDTLS_X509_BADCERT_FUTURE) != 0;
		xo_x_tls_expiry_msg((int64_t)now, (int64_t)local_off(now), XO_ADDR(future ? &crt->valid_from : &crt->valid_to), future, XO_ADDR(msg), (int64_t)sizeof msg);
	}
	if (!msg[0] && t->host) {
		hostname_msg(crt, t->host, msg, sizeof msg);
	}
	uint32_t chain = f | t->upper;
	if (!msg[0] && (chain & (MBEDTLS_X509_BADCERT_EXT_KEY_USAGE | MBEDTLS_X509_BADCERT_KEY_USAGE)) &&
	    !(chain & MBEDTLS_X509_BADCERT_NOT_TRUSTED)) {
		snprintf(msg, sizeof msg, "x509: certificate specifies an incompatible key usage");
	}
	if (!msg[0] && (t->upper & (MBEDTLS_X509_BADCERT_EXPIRED | MBEDTLS_X509_BADCERT_FUTURE)) && !(chain & MBEDTLS_X509_BADCERT_NOT_TRUSTED)) {
		snprintf(msg, sizeof msg, "x509: certificate has expired or is not yet valid: a certificate of the chain is outside its validity period");
	}
	if (!msg[0] && chain) {
		snprintf(msg, sizeof msg, "x509: certificate signed by unknown authority");
	}
	if (msg[0]) {
		free(t->vmsg);
		t->vmsg = strdup(msg);
		*flags = f ? f : MBEDTLS_X509_BADCERT_OTHER;
	} else {
		*flags = 0;
	}
	return 0;
}

// alert_msg is Go's text of the fatal alert t received:
// "remote error: tls: <alertText>" (tlsname.xo's alert_text).
static void alert_msg(tls_conn *t, char *m, size_t mn) {
	xo_x_tls_alert_msg(XO_ADDR(m), (int64_t)mn, mbedtls_ssl_get_fatal_alert(&t->ssl));
}

// tls_client_start runs the client's handshake on pd for host (the
// URL's host name) with roots, within when (and http.DefaultTransport's
// 10 s TLSHandshakeTimeout): CC_OK and the connection in *out, or why it
// failed with Go's error text in msg (CC_ERR: the ClientErr tag in *kind,
// 2 Connect for an alert, which Go reports as a net.OpError, else 4
// Protocol).
static int tls_client_start(xo_pd *pd, const char *host, xo_troots *roots, int64_t when, tls_conn **out, char *msg, size_t mn, int *kind) {
	*out = NULL;
	*kind = 4;
	if (roots == &sys_roots) {
		pthread_once(&sys_once, sys_roots_load);
	}
	if (!roots->conf_ok) {
		snprintf(msg, mn, "tls: the TLS library failed to start");
		return CC_ERR;
	}
	if (roots->n == 0) {
		// x509.SystemRootsError (no bundle file or directory had any)
		snprintf(msg, mn, "tls: failed to verify certificate: x509: failed to load system roots and no roots provided");
		return CC_ERR;
	}
	tls_conn *t = tls_conn_new(&roots->conf, pd);
	int64_t hs = deadline((int64_t)10 * 1000000000);
	t->when = when && when < hs ? when : hs;
	char *name = strdup(host);
	size_t hn = strlen(name);
	if (hn >= 2 && name[0] == '[' && name[hn - 1] == ']') {
		memmove(name, name + 1, hn - 2);
		name[hn - 2] = 0;
	}
	t->host = name;
	t->host_ip = (int)xo_x_tls_is_ip(XO_ADDR(name), (int64_t)strlen(name));
#ifdef __APPLE__
	t->apple = roots == &sys_roots;
#endif
	// SNI for a host name (Go's hostnameInSNI: not for an IP address,
	// without a trailing dot); the name itself is checked by cli_verify
	if (t->host_ip) {
		mbedtls_ssl_set_hostname(&t->ssl, NULL);
	} else {
		char *sni = strdup(name);
		size_t sn = strlen(sni);
		if (sn > 0 && sni[sn - 1] == '.') {
			sni[sn - 1] = 0;
		}
		mbedtls_ssl_set_hostname(&t->ssl, sni);
		free(sni);
	}
	mbedtls_ssl_set_verify(&t->ssl, cli_verify, t);
	int r;
	do {
		r = mbedtls_ssl_handshake(&t->ssl);
	} while (r == MBEDTLS_ERR_SSL_WANT_READ || r == MBEDTLS_ERR_SSL_WANT_WRITE);
	t->host = NULL;
	free(name);
	if (r == 0) {
		t->handshaken = 1;
		t->when = when;
		*out = t;
		return CC_OK;
	}
	int st = CC_ERR;
	if (t->status == CC_TIMEOUT || t->status == CC_CANCELED) {
		st = t->status;
	} else if (t->nfirst == 5 && memcmp(t->first, "HTTP/", 5) == 0) {
		snprintf(msg, mn, "http: server gave HTTP response to HTTPS client");
	} else if (r == MBEDTLS_ERR_X509_CERT_VERIFY_FAILED || t->vmsg) {
		snprintf(msg, mn, "tls: failed to verify certificate: %s", t->vmsg ? t->vmsg : "x509: certificate signed by unknown authority");
	} else if (r == MBEDTLS_ERR_SSL_FATAL_ALERT_MESSAGE) {
		alert_msg(t, msg, mn);
		*kind = 2;
	} else if (r == MBEDTLS_ERR_SSL_CONN_EOF || (t->status == 0 && r == TLS_ERR_RECV)) {
		snprintf(msg, mn, "EOF");
	} else if (t->nfirst >= 1 && t->first[0] != 0x15 && t->first[0] != 0x16) {
		snprintf(msg, mn, "tls: first record does not look like a TLS handshake");
	} else if (r == MBEDTLS_ERR_SSL_HANDSHAKE_FAILURE || r == MBEDTLS_ERR_SSL_BAD_PROTOCOL_VERSION) {
		snprintf(msg, mn, "tls: handshake failure");
	} else if (t->status == CC_ERR) {
		snprintf(msg, mn, "EOF");
	} else {
		snprintf(msg, mn, "tls: handshake failed (mbedtls -0x%04x)", (unsigned)-r);
	}
	tls_conn_free(t);
	return st;
}

#else // !XO_TLS: the stubs of programs without std/http

struct tls_conn {
	int unused;
};
struct tls_srv {
	int unused;
};

static int64_t tls_read(tls_conn *t, char *p, int64_t n) {
	(void)t, (void)p, (void)n;
	return -1;
}
static int tls_write(tls_conn *t, const char *p, int64_t n) {
	(void)t, (void)p, (void)n;
	return CC_ERR;
}
static void tls_conn_free(tls_conn *t) { (void)t; }
static void tls_close_notify(tls_conn *t) { (void)t; }
static void tls_srv_free(tls_srv *s) { (void)s; }
static tls_conn *tls_server_conn(tls_srv *s, xo_pd *pd) {
	(void)s, (void)pd;
	return NULL;
}
static int64_t tls_hc_read(tls_conn *t, char *p, int64_t n) {
	(void)t, (void)p, (void)n;
	return -1;
}
static int tls_server_handshake(tls_conn *t) {
	(void)t;
	return 0;
}
static int64_t tls_nb_read(tls_conn *t, char *p, int64_t n) {
	(void)t, (void)p, (void)n;
	return -1;
}
static int tls_alpn_h2(tls_conn *t) {
	(void)t;
	return 0;
}
static void tls_set_when(tls_conn *t, int64_t when) { (void)t, (void)when; }
static int tls_h2_bad_cipher(tls_conn *t, char *why, size_t wn) {
	(void)t, (void)why, (void)wn;
	return 0;
}
static const char *tls_alert(tls_conn *t) {
	(void)t;
	return NULL;
}
int64_t xo_tls_c_roots_crt(int64_t p) {
	(void)p;
	return 0;
}
int64_t xo_tls_c_apple_svc(int64_t p) {
	(void)p;
	return 0;
}
static int tls_client_start(xo_pd *pd, const char *host, xo_troots *roots, int64_t when, tls_conn **out, char *msg, size_t mn, int *kind) {
	(void)pd, (void)host, (void)roots, (void)when;
	*out = NULL;
	*kind = 4;
	snprintf(msg, mn, "https is not supported: the program was built without TLS");
	return CC_ERR;
}

#endif // XO_TLS
