// xo_tls.c: TLS for the native std/http (decision 0105), included at the
// end of xo_http.c.
//
// The server (Server.with_tls) and the client (https URLs, Client.with_roots)
// speak TLS 1.2 and 1.3 through Mbed TLS 4.1 (third_party/mbedtls), with
// what Go's crypto/tls and net/http do by default as the reference:
//
//   - certificates: http.tls_cert checks a PEM chain and key as
//     tls.X509KeyPair does, with its error texts;
//   - the server: ALPN "h2" or "http/1.1" (HTTP/2, xo_h2.c, decision
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

static void tls_init_once(void) {
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
// PEM (encoding/pem's Decode, as tls.X509KeyPair and
// CertPool.AppendCertsFromPEM use it)

typedef struct {
	char type[80];
	unsigned char *der; // malloc'd
	size_t n;
} pem_block;

static int pem_b64(int c) {
	if (c >= 'A' && c <= 'Z') {
		return c - 'A';
	}
	if (c >= 'a' && c <= 'z') {
		return c - 'a' + 26;
	}
	if (c >= '0' && c <= '9') {
		return c - '0' + 52;
	}
	if (c == '+') {
		return 62;
	}
	if (c == '/') {
		return 63;
	}
	return -1;
}

static const char *pem_find(const char *p, const char *end, const char *s) {
	size_t k = strlen(s);
	for (; p + k <= end; p++) {
		if (memcmp(p, s, k) == 0) {
			return p;
		}
	}
	return NULL;
}

// pem_next decodes the next PEM block of [*p, end): 1 and the block (its
// der to free), 0 when there is none. A block that does not decode is
// skipped, as pem.Decode does.
static int pem_next(const char **p, const char *end, pem_block *b) {
	for (;;) {
		const char *begin = pem_find(*p, end, "-----BEGIN ");
		if (!begin) {
			*p = end;
			return 0;
		}
		// pem.Decode requires BEGIN at the start of a line
		if (begin != *p && begin[-1] != '\n') {
			*p = begin + 11;
			continue;
		}
		const char *t = begin + 11;
		const char *tend = pem_find(t, end, "-----");
		*p = begin + 11;
		if (!tend || (size_t)(tend - t) >= sizeof b->type) {
			continue;
		}
		const char *nl = tend + 5;
		while (nl < end && (*nl == ' ' || *nl == '\t' || *nl == '\r')) {
			nl++;
		}
		if (nl < end && *nl != '\n') {
			continue;
		}
		memcpy(b->type, t, (size_t)(tend - t));
		b->type[tend - t] = 0;
		char endline[100];
		snprintf(endline, sizeof endline, "-----END %s-----", b->type);
		const char *body = nl < end ? nl + 1 : end;
		const char *stop = pem_find(body, end, endline);
		if (!stop) {
			continue;
		}
		// headers ("Key: value" lines up to an empty line) are skipped
		const char *q = body;
		const char *line_end = memchr(q, '\n', (size_t)(stop - q));
		if (line_end && memchr(q, ':', (size_t)(line_end - q))) {
			const char *blank = pem_find(q, stop, "\n\n");
			const char *blank2 = pem_find(q, stop, "\n\r\n");
			if (blank2 && (!blank || blank2 < blank)) {
				blank = blank2 + 1;
			}
			if (!blank) {
				continue;
			}
			q = blank + 2;
		}
		size_t cap = (size_t)(stop - q) / 4 * 3 + 3;
		unsigned char *der = malloc(cap ? cap : 1);
		if (!der) {
			xo_oom();
		}
		size_t n = 0;
		uint32_t acc = 0;
		int bits = 0, pad = 0, bad = 0;
		for (const char *c = q; c < stop; c++) {
			if (*c == '\n' || *c == '\r' || *c == ' ' || *c == '\t') {
				continue;
			}
			if (*c == '=') {
				pad++;
				continue;
			}
			int v = pem_b64((unsigned char)*c);
			if (v < 0 || pad) {
				bad = 1;
				break;
			}
			acc = (acc << 6) | (uint32_t)v;
			bits += 6;
			if (bits >= 8) {
				bits -= 8;
				der[n++] = (unsigned char)(acc >> bits);
			}
		}
		if (bad || pad > 2) {
			free(der);
			continue;
		}
		b->der = der;
		b->n = n;
		*p = stop + strlen(endline);
		return 1;
	}
}

static int ends_with(const char *s, const char *suf) {
	size_t n = strlen(s), k = strlen(suf);
	return n >= k && strcmp(s + n - k, suf) == 0;
}

// types_list appends Go's %v of a []string: [a b c].
static void types_list(nbuf *o, char **ts, int n) {
	nbuf_byte(o, '[');
	for (int i = 0; i < n; i++) {
		if (i > 0) {
			nbuf_byte(o, ' ');
		}
		nbuf_str(o, ts[i]);
	}
	nbuf_byte(o, ']');
}

// ---------------------------------------------------------------------------
// Certificates (http.tls_cert)

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

static xo_tcert *tcert_fail(xo_tcert *c, const char *msg) {
	c->err = strdup(msg);
	return c;
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
		return tcert_fail(c, "tls: the TLS library failed to start");
	}
	const char *p = cp, *end = cp + cn;
	pem_block b;
	char *skipped[64];
	int nskip = 0, ncert = 0;
	unsigned char *leaf = NULL;
	size_t leafn = 0;
	int bad_leaf = 0;
	while (pem_next(&p, end, &b)) {
		if (strcmp(b.type, "CERTIFICATE") == 0) {
			if (ncert == 0) {
				leaf = b.der;
				leafn = b.n;
			}
			// the chain is sent as it is; only the leaf must parse
			// (X509KeyPair parses only Certificate[0])
			if (mbedtls_x509_crt_parse_der(&c->crt, b.der, b.n) != 0 && ncert == 0) {
				bad_leaf = 1;
			}
			ncert++;
			if (b.der != leaf) {
				free(b.der);
			}
		} else {
			if (nskip < 64) {
				skipped[nskip++] = strdup(b.type);
			}
			free(b.der);
		}
	}
	free(leaf);
	nbuf o = {0};
	if (ncert == 0) {
		if (nskip == 0) {
			nbuf_str(&o, "tls: failed to find any PEM data in certificate input");
		} else if (nskip == 1 && ends_with(skipped[0], "PRIVATE KEY")) {
			nbuf_str(&o, "tls: failed to find certificate PEM data in certificate input, but did find a private key; PEM inputs may have been switched");
		} else {
			nbuf_str(&o, "tls: failed to find \"CERTIFICATE\" PEM block in certificate input after skipping PEM blocks of the following types: ");
			types_list(&o, skipped, nskip);
		}
	}
	for (int i = 0; i < nskip; i++) {
		free(skipped[i]);
	}
	if (o.n > 0) {
		nbuf_byte(&o, 0);
		tcert_fail(c, o.p);
		free(o.p);
		return c;
	}
	// the key: the first block whose type ends in PRIVATE KEY
	p = kp;
	end = kp + kn;
	nskip = 0;
	unsigned char *key = NULL;
	size_t keyn = 0;
	while (pem_next(&p, end, &b)) {
		if (strcmp(b.type, "PRIVATE KEY") == 0 || ends_with(b.type, " PRIVATE KEY")) {
			key = b.der;
			keyn = b.n;
			break;
		}
		if (nskip < 64) {
			skipped[nskip++] = strdup(b.type);
		}
		free(b.der);
	}
	if (!key) {
		if (nskip == 0) {
			nbuf_str(&o, "tls: failed to find any PEM data in key input");
		} else if (nskip == 1 && strcmp(skipped[0], "CERTIFICATE") == 0) {
			nbuf_str(&o, "tls: found a certificate rather than a key in the PEM for the private key");
		} else {
			nbuf_str(&o, "tls: failed to find PEM block with type ending in \"PRIVATE KEY\" in key input after skipping PEM blocks of the following types: ");
			types_list(&o, skipped, nskip);
		}
	}
	for (int i = 0; i < nskip; i++) {
		free(skipped[i]);
	}
	if (!key) {
		nbuf_byte(&o, 0);
		tcert_fail(c, o.p);
		free(o.p);
		return c;
	}
	if (bad_leaf) {
		free(key);
		return tcert_fail(c, "x509: malformed certificate");
	}
	int r = mbedtls_pk_parse_key(&c->pk, key, keyn, NULL, 0);
	mbedtls_platform_zeroize(key, keyn);
	free(key);
	if (r != 0) {
		return tcert_fail(c, r == MBEDTLS_ERR_PK_UNKNOWN_PK_ALG ? "tls: found unknown private key type in PKCS#8 wrapping" : "tls: failed to parse private key");
	}
	psa_key_type_t pt = mbedtls_pk_get_key_type(&c->crt.pk), kt = mbedtls_pk_get_key_type(&c->pk);
	int pub_rsa = PSA_KEY_TYPE_IS_RSA(pt), key_rsa = PSA_KEY_TYPE_IS_RSA(kt);
	int pub_ec = PSA_KEY_TYPE_IS_ECC(pt), key_ec = PSA_KEY_TYPE_IS_ECC(kt);
	if (!pub_rsa && !pub_ec) {
		return tcert_fail(c, "tls: unknown public key algorithm");
	}
	if ((pub_rsa && !key_rsa) || (pub_ec && !key_ec)) {
		return tcert_fail(c, "tls: private key type does not match public key type");
	}
	if (mbedtls_pk_check_pair(&c->crt.pk, &c->pk) != 0) {
		return tcert_fail(c, "tls: private key does not match public key");
	}
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
// (CertPool.AppendCertsFromPEM).
static void roots_add_pem(xo_troots *r, const char *p, size_t n) {
	const char *end = p + n;
	pem_block b;
	while (pem_next(&p, end, &b)) {
		if (strcmp(b.type, "CERTIFICATE") == 0 && mbedtls_x509_crt_parse_der(&r->crt, b.der, b.n) == 0) {
			r->n++;
		}
		free(b.der);
	}
}

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

static int read_file(const char *path, nbuf *out) {
	FILE *f = fopen(path, "rb");
	if (!f) {
		return 0;
	}
	char buf[65536];
	size_t k;
	while ((k = fread(buf, 1, sizeof buf, f)) > 0) {
		nbuf_put(out, buf, (int64_t)k);
	}
	fclose(f);
	return 1;
}

#ifdef __APPLE__
static void roots_add_cf(xo_troots *r, CFArrayRef certs) {
	for (CFIndex i = 0; i < CFArrayGetCount(certs); i++) {
		SecCertificateRef c = (SecCertificateRef)CFArrayGetValueAtIndex(certs, i);
		CFDataRef d = SecCertificateCopyData(c);
		if (d) {
			if (mbedtls_x509_crt_parse_der(&r->crt, CFDataGetBytePtr(d), (size_t)CFDataGetLength(d)) == 0) {
				r->n++;
			}
			CFRelease(d);
		}
	}
}
#endif

// sys_roots_load reads the system's roots once. macOS: the Security
// framework's anchors, and the admin's and user's certificates trusted as
// roots without conditions (Go asks the framework to verify each chain;
// see the decision); /etc/ssl/cert.pem when the framework gives none.
// Elsewhere: crypto/x509's root_unix.go (Linux lists).
static void sys_roots_load(void) {
	xo_troots *r = &sys_roots;
	mbedtls_x509_crt_init(&r->crt);
	if (!tls_init()) {
		return;
	}
#ifdef __APPLE__
	CFArrayRef anchors = NULL;
	if (SecTrustCopyAnchorCertificates(&anchors) == errSecSuccess && anchors) {
		roots_add_cf(r, anchors);
		CFRelease(anchors);
	}
	SecTrustSettingsDomain doms[] = {kSecTrustSettingsDomainAdmin, kSecTrustSettingsDomainUser};
	for (int d = 0; d < 2; d++) {
		CFArrayRef certs = NULL;
		if (SecTrustSettingsCopyCertificates(doms[d], &certs) != errSecSuccess || !certs) {
			continue;
		}
		for (CFIndex i = 0; i < CFArrayGetCount(certs); i++) {
			SecCertificateRef c = (SecCertificateRef)CFArrayGetValueAtIndex(certs, i);
			CFArrayRef settings = NULL;
			if (SecTrustSettingsCopyTrustSettings(c, doms[d], &settings) == errSecSuccess && settings) {
				// an empty array: trusted as a root for everything
				if (CFArrayGetCount(settings) == 0) {
					CFDataRef der = SecCertificateCopyData(c);
					if (der) {
						if (mbedtls_x509_crt_parse_der(&r->crt, CFDataGetBytePtr(der), (size_t)CFDataGetLength(der)) == 0) {
							r->n++;
						}
						CFRelease(der);
					}
				}
				CFRelease(settings);
			}
		}
		CFRelease(certs);
	}
	if (r->n == 0) {
		nbuf b = {0};
		if (read_file("/etc/ssl/cert.pem", &b)) {
			roots_add_pem(r, b.p ? b.p : "", (size_t)b.n);
		}
		free(b.p);
	}
#elif defined(_WIN32)
	// Windows (decision 0197): the certificates of the system's ROOT
	// store (Go asks CertGetCertificateChain to verify instead, with
	// the store's policies: a later stage)
	HCERTSTORE st = CertOpenSystemStoreW(0, L"ROOT");
	if (st) {
		PCCERT_CONTEXT c = NULL;
		while ((c = CertEnumCertificatesInStore(st, c)) != NULL) {
			if (mbedtls_x509_crt_parse_der(&r->crt, c->pbCertEncoded, c->cbCertEncoded) == 0) {
				r->n++;
			}
		}
		CertCloseStore(st, 0);
	}
#else
	static const char *files[] = {
		"/etc/ssl/certs/ca-certificates.crt",                // Debian, Ubuntu, Gentoo
		"/etc/pki/tls/certs/ca-bundle.crt",                  // Fedora, RHEL 6
		"/etc/ssl/ca-bundle.pem",                            // OpenSUSE
		"/etc/pki/tls/cacert.pem",                           // OpenELEC
		"/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem", // CentOS, RHEL 7
		"/etc/ssl/cert.pem",                                 // Alpine
	};
	const char *env = getenv("SSL_CERT_FILE");
	int nf = env && *env ? 1 : (int)(sizeof files / sizeof files[0]);
	for (int i = 0; i < nf; i++) {
		nbuf b = {0};
		int ok = read_file(env && *env ? env : files[i], &b);
		if (ok) {
			roots_add_pem(r, b.p ? b.p : "", (size_t)b.n);
		}
		free(b.p);
		if (ok) {
			break;
		}
	}
	static const char *dirs[] = {"/etc/ssl/certs", "/etc/pki/tls/certs"};
	const char *denv = getenv("SSL_CERT_DIR");
	char *dlist = strdup(denv && *denv ? denv : "/etc/ssl/certs:/etc/pki/tls/certs");
	(void)dirs;
	for (char *save = NULL, *d = strtok_r(dlist, ":", &save); d; d = strtok_r(NULL, ":", &save)) {
		DIR *dir = opendir(d);
		if (!dir) {
			continue;
		}
		struct dirent *e;
		while ((e = readdir(dir)) != NULL) {
			if (e->d_name[0] == '.') {
				continue;
			}
			char path[4096];
			snprintf(path, sizeof path, "%s/%s", d, e->d_name);
			// readUniqueDirectoryEntries: a link to a file of the same
			// directory is read once, as that file
			char target[4096];
			ssize_t tl = readlink(path, target, sizeof target - 1);
			if (tl > 0) {
				target[tl] = 0;
				if (!strchr(target, '/')) {
					continue;
				}
			}
			struct stat st;
			if (stat(path, &st) != 0 || !S_ISREG(st.st_mode)) {
				continue;
			}
			nbuf b = {0};
			if (read_file(path, &b)) {
				roots_add_pem(r, b.p ? b.p : "", (size_t)b.n);
			}
			free(b.p);
		}
		closedir(dir);
	}
	free(dlist);
#endif
	roots_conf(r);
}

// xo_tls_roots_system is the client's default: the system's roots (a
// handle that is never freed), loaded at the first https request.
xo_troots *xo_tls_roots_system(void) { return &sys_roots; }

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
			// the HTTP/2 connection's fiber waits itself (h2_wait)
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

// rfc3339 formats t as Go's time.Format(time.RFC3339) (local: the
// local zone, else UTC).
static void rfc3339(time_t t, int local, char *out, size_t on) {
	struct tm tm;
	if (local) {
		localtime_r(&t, &tm);
	} else {
		gmtime_r(&t, &tm);
	}
	size_t k = strftime(out, on, "%Y-%m-%dT%H:%M:%S", &tm);
#ifdef _WIN32
	// no tm_gmtoff: the local time read back as UTC, minus t
	long off = local ? (long)(_mkgmtime(&tm) - t) : 0;
#else
	long off = local ? tm.tm_gmtoff : 0;
#endif
	if (off == 0) {
		snprintf(out + k, on - k, "Z");
	} else {
		char sign = off < 0 ? '-' : '+';
		if (off < 0) {
			off = -off;
		}
		snprintf(out + k, on - k, "%c%02ld:%02ld", sign, off / 3600, (off / 60) % 60);
	}
}

static time_t x509_time(const mbedtls_x509_time *t) {
	struct tm tm = {0};
	tm.tm_year = t->year - 1900;
	tm.tm_mon = t->mon - 1;
	tm.tm_mday = t->day;
	tm.tm_hour = t->hour;
	tm.tm_min = t->min;
	tm.tm_sec = t->sec;
	return timegm(&tm);
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

#ifdef __APPLE__
// apple_verify is crypto/x509's systemVerify (root_darwin.go), the
// verification of a chain against the system's roots on macOS: the
// Security framework evaluates the presented chain for an SSL server
// with the host name, then Go checks the host name itself. 0 when the
// chain is trusted, else 1 and Go's text in msg.
static int apple_verify(const mbedtls_x509_crt *crt, const char *host, char *msg, size_t mn) {
	CFMutableArrayRef certs = CFArrayCreateMutable(NULL, 0, &kCFTypeArrayCallBacks);
	for (const mbedtls_x509_crt *c = crt; c && c->raw.p; c = c->next) {
		CFDataRef d = CFDataCreate(NULL, c->raw.p, (CFIndex)c->raw.len);
		SecCertificateRef sc = d ? SecCertificateCreateWithData(NULL, d) : NULL;
		if (d) {
			CFRelease(d);
		}
		if (!sc) {
			CFRelease(certs);
			snprintf(msg, mn, c == crt ? "invalid leaf certificate" : "x509: malformed certificate");
			return 1;
		}
		CFArrayAppendValue(certs, sc);
		CFRelease(sc);
	}
	CFStringRef name = host[0] ? CFStringCreateWithCString(NULL, host, kCFStringEncodingUTF8) : NULL;
	SecPolicyRef pol = SecPolicyCreateSSL(true, name);
	if (name) {
		CFRelease(name);
	}
	SecTrustRef trust = NULL;
	OSStatus st = SecTrustCreateWithCertificates(certs, pol, &trust);
	CFRelease(certs);
	if (pol) {
		CFRelease(pol);
	}
	if (st != errSecSuccess || !trust) {
		snprintf(msg, mn, "SecTrustCreateWithCertificates error: %d", (int)st);
		return 1;
	}
	CFErrorRef err = NULL;
	int bad = 0;
	if (!SecTrustEvaluateWithError(trust, &err)) {
		bad = 1;
		CFIndex code = err ? CFErrorGetCode(err) : 0;
		char desc[600] = "";
		if (err) {
			CFStringRef ds = CFErrorCopyDescription(err);
			if (ds) {
				CFStringGetCString(ds, desc, sizeof desc, kCFStringEncodingUTF8);
				CFRelease(ds);
			}
			CFRelease(err);
		}
		if (code == errSecCertificateExpired) {
			snprintf(msg, mn, "x509: certificate has expired or is not yet valid: %s", desc);
		} else if (code == errSecHostNameMismatch) {
			if (!hostname_msg(crt, host, msg, mn)) {
				snprintf(msg, mn, "x509: certificate is valid for %s", host);
			}
		} else if (code == errSecNotTrusted) {
			snprintf(msg, mn, "x509: certificate signed by unknown authority");
		} else {
			snprintf(msg, mn, "x509: %s", desc);
		}
	}
	CFRelease(trust);
	if (!bad && host[0] && hostname_msg(crt, host, msg, mn)) {
		bad = 1;
	}
	return bad;
}
#endif

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
		if (apple_verify(crt, t->host ? t->host : "", msg, sizeof msg)) {
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
		char now[40], lim[40];
		rfc3339(time(NULL), 1, now, sizeof now);
		if (f & MBEDTLS_X509_BADCERT_FUTURE) {
			rfc3339(x509_time(&crt->valid_from), 0, lim, sizeof lim);
			snprintf(msg, sizeof msg, "x509: certificate has expired or is not yet valid: current time %s is before %s", now, lim);
		} else {
			rfc3339(x509_time(&crt->valid_to), 0, lim, sizeof lim);
			snprintf(msg, sizeof msg, "x509: certificate has expired or is not yet valid: current time %s is after %s", now, lim);
		}
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

// alert_text is crypto/tls's alertText.
static const char *alert_text(int a) {
	switch (a) {
	case 0: return "close notify";
	case 10: return "unexpected message";
	case 20: return "bad record MAC";
	case 21: return "decryption failed";
	case 22: return "record overflow";
	case 30: return "decompression failure";
	case 40: return "handshake failure";
	case 42: return "bad certificate";
	case 43: return "unsupported certificate";
	case 44: return "revoked certificate";
	case 45: return "expired certificate";
	case 46: return "unknown certificate";
	case 47: return "illegal parameter";
	case 48: return "unknown certificate authority";
	case 49: return "access denied";
	case 50: return "error decoding message";
	case 51: return "error decrypting message";
	case 60: return "export restriction";
	case 70: return "protocol version not supported";
	case 71: return "insufficient security level";
	case 80: return "internal error";
	case 86: return "inappropriate fallback";
	case 90: return "user canceled";
	case 100: return "no renegotiation";
	case 109: return "missing extension";
	case 110: return "unsupported extension";
	case 111: return "certificate unobtainable";
	case 112: return "unrecognized name";
	case 113: return "bad certificate status response";
	case 114: return "bad certificate hash value";
	case 115: return "unknown PSK identity";
	case 116: return "certificate required";
	case 120: return "no application protocol";
	case 121: return "encrypted client hello required";
	}
	return NULL;
}

// alert_msg is Go's text of the fatal alert t received:
// "remote error: tls: <alertText>".
static void alert_msg(tls_conn *t, char *m, size_t mn) {
	int a = mbedtls_ssl_get_fatal_alert(&t->ssl);
	const char *txt = alert_text(a);
	if (txt) {
		snprintf(m, mn, "remote error: tls: %s", txt);
	} else {
		snprintf(m, mn, "remote error: tls: alert(%d)", a);
	}
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
static int tls_client_start(xo_pd *pd, const char *host, xo_troots *roots, int64_t when, tls_conn **out, char *msg, size_t mn, int *kind) {
	(void)pd, (void)host, (void)roots, (void)when;
	*out = NULL;
	*kind = 4;
	snprintf(msg, mn, "https is not supported: the program was built without TLS");
	return CC_ERR;
}

#endif // XO_TLS
