// tls_accel.c: TestTLSAccel's program. Checks the functions
// third_party/mbedtls/xo/xo_tls_accel.c and xo_p256.c put in front of
// Mbed TLS's own (decisions 0117, 0132) against known answers (RFC 6979
// A.2.5, RFC 7748 6.1) and against the library's renamed originals on
// random inputs. "tls_accel N" runs the checks; "tls_accel serve" answers
// signing and verification requests on stdin for TestP256 (Go's
// crypto/ecdsa, Wycheproof); "tls_accel bench N" times P-256 signing.
#define MBEDTLS_DECLARE_PRIVATE_IDENTIFIERS
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include "psa/crypto.h"
#include "mbedtls/private/bignum.h"

psa_status_t xo_builtin_ecp_export_public_key(const psa_key_attributes_t *attributes,
	const uint8_t *key_buffer, size_t key_buffer_size, uint8_t *data, size_t data_size, size_t *data_length);
psa_status_t mbedtls_psa_ecp_export_public_key(const psa_key_attributes_t *attributes,
	const uint8_t *key_buffer, size_t key_buffer_size, uint8_t *data, size_t data_size, size_t *data_length);
int xo_builtin_mpi_gcd_modinv_odd(mbedtls_mpi *G, mbedtls_mpi *I, const mbedtls_mpi *A, const mbedtls_mpi *N);
int mbedtls_mpi_gcd_modinv_odd(mbedtls_mpi *G, mbedtls_mpi *I, const mbedtls_mpi *A, const mbedtls_mpi *N);

psa_status_t xo_builtin_psa_ecdsa_sign_hash(const psa_key_attributes_t *attributes,
	const uint8_t *key_buffer, size_t key_buffer_size, psa_algorithm_t alg, const uint8_t *hash, size_t hash_length,
	uint8_t *signature, size_t signature_size, size_t *signature_length);
psa_status_t mbedtls_psa_ecdsa_sign_hash(const psa_key_attributes_t *attributes,
	const uint8_t *key_buffer, size_t key_buffer_size, psa_algorithm_t alg, const uint8_t *hash, size_t hash_length,
	uint8_t *signature, size_t signature_size, size_t *signature_length);
psa_status_t xo_builtin_psa_ecdsa_verify_hash(const psa_key_attributes_t *attributes,
	const uint8_t *key_buffer, size_t key_buffer_size, psa_algorithm_t alg, const uint8_t *hash, size_t hash_length,
	const uint8_t *signature, size_t signature_length);
psa_status_t mbedtls_psa_ecdsa_verify_hash(const psa_key_attributes_t *attributes,
	const uint8_t *key_buffer, size_t key_buffer_size, psa_algorithm_t alg, const uint8_t *hash, size_t hash_length,
	const uint8_t *signature, size_t signature_length);

static int fails;

#define CHECK(c, ...) do { if (!(c)) { printf("FAIL line %d: ", __LINE__); printf(__VA_ARGS__); printf("\n"); fails++; } } while (0)

static void unhex(const char *s, uint8_t *out, size_t n) {
	for (size_t i = 0; i < n; i++) {
		unsigned v;
		sscanf(s + 2 * i, "%2x", &v);
		out[i] = (uint8_t)v;
	}
}

static void rfc6979(void) {
	uint8_t d[32], want[64], hash[32], sig[64];
	size_t n;
	unhex("C9AFA9D845BA75166B5C215767B1D6934E50C3DB36E89B127B8A622B120F6721", d, 32);
	unhex("EFD48B2AACB6A8FD1140DD9CD45E81D69D2C877B56AAF991C34D0EA84EAF3716"
	      "F7CB1C942D657C41D436C7A1B6E29F65F3E900DBB9AFF4064DC4AB2F843ACDA8", want, 64);
	CHECK(psa_hash_compute(PSA_ALG_SHA_256, (const uint8_t *)"sample", 6, hash, 32, &n) == PSA_SUCCESS, "sha256");
	psa_key_attributes_t a = PSA_KEY_ATTRIBUTES_INIT;
	psa_set_key_type(&a, PSA_KEY_TYPE_ECC_KEY_PAIR(PSA_ECC_FAMILY_SECP_R1));
	psa_set_key_bits(&a, 256);
	psa_set_key_usage_flags(&a, PSA_KEY_USAGE_SIGN_HASH | PSA_KEY_USAGE_VERIFY_HASH);
	psa_set_key_algorithm(&a, PSA_ALG_DETERMINISTIC_ECDSA(PSA_ALG_SHA_256));
	psa_key_id_t k;
	CHECK(psa_import_key(&a, d, 32, &k) == PSA_SUCCESS, "import P-256 key");
	CHECK(psa_sign_hash(k, PSA_ALG_DETERMINISTIC_ECDSA(PSA_ALG_SHA_256), hash, 32, sig, 64, &n) == PSA_SUCCESS && n == 64, "sign");
	CHECK(memcmp(sig, want, 64) == 0, "RFC 6979 A.2.5 signature differs");
	psa_destroy_key(k);
}

static void rfc7748(void) {
	uint8_t d[32], want[32], pub[32];
	size_t n;
	unhex("77076d0a7318a57d3c16c17251b26645df4c2f87ebc0992ab177fba51db92c2a", d, 32);
	unhex("8520f0098930a754748b7ddcb43ef75a0dbf3a0d26381af4eba4a98eaa9b4e6a", want, 32);
	psa_key_attributes_t a = PSA_KEY_ATTRIBUTES_INIT;
	psa_set_key_type(&a, PSA_KEY_TYPE_ECC_KEY_PAIR(PSA_ECC_FAMILY_MONTGOMERY));
	psa_set_key_bits(&a, 255);
	psa_set_key_usage_flags(&a, PSA_KEY_USAGE_DERIVE);
	psa_set_key_algorithm(&a, PSA_ALG_ECDH);
	psa_key_id_t k;
	CHECK(psa_import_key(&a, d, 32, &k) == PSA_SUCCESS, "import X25519 key");
	CHECK(psa_export_public_key(k, pub, 32, &n) == PSA_SUCCESS && n == 32, "export");
	CHECK(memcmp(pub, want, 32) == 0, "RFC 7748 6.1 public key differs");
	psa_destroy_key(k);
}

// X25519 public keys of random private keys: the HACL* path and the
// library's ladder agree.
static void x25519_random(int rounds) {
	psa_key_attributes_t a = PSA_KEY_ATTRIBUTES_INIT;
	psa_set_key_type(&a, PSA_KEY_TYPE_ECC_KEY_PAIR(PSA_ECC_FAMILY_MONTGOMERY));
	psa_set_key_bits(&a, 255);
	for (int i = 0; i < rounds; i++) {
		uint8_t d[32], p1[32], p2[32];
		size_t n1 = 0, n2 = 0;
		psa_generate_random(d, 32);
		if (i == 0) memset(d, 0, 32);
		if (i == 1) memset(d, 0xff, 32);
		CHECK(mbedtls_psa_ecp_export_public_key(&a, d, 32, p1, 32, &n1) == PSA_SUCCESS, "fast export");
		CHECK(xo_builtin_ecp_export_public_key(&a, d, 32, p2, 32, &n2) == PSA_SUCCESS, "builtin export");
		CHECK(n1 == 32 && n2 == 32 && memcmp(p1, p2, 32) == 0, "X25519 public keys differ (round %d)", i);
	}
}

// Inverses modulo P-256's prime and order: Fermat and the binary GCD
// agree on random values and the edges.
static void inverses(int rounds) {
	const char *mods[2] = {
		"FFFFFFFF00000001000000000000000000000000FFFFFFFFFFFFFFFFFFFFFFFF",
		"FFFFFFFF00000000FFFFFFFFFFFFFFFFBCE6FAADA7179E84F3B9CAC2FC632551",
	};
	for (int m = 0; m < 2; m++) {
		mbedtls_mpi N, A, I1, I2, P;
		mbedtls_mpi_init(&N); mbedtls_mpi_init(&A); mbedtls_mpi_init(&I1); mbedtls_mpi_init(&I2); mbedtls_mpi_init(&P);
		mbedtls_mpi_read_string(&N, 16, mods[m]);
		for (int i = 0; i < rounds; i++) {
			uint8_t b[32];
			psa_generate_random(b, 32);
			mbedtls_mpi_read_binary(&A, b, 32);
			if (i == 0) mbedtls_mpi_lset(&A, 1);
			if (i == 1) mbedtls_mpi_sub_int(&A, &N, 1);
			if (i == 2) mbedtls_mpi_lset(&A, 2);
			if (mbedtls_mpi_cmp_mpi(&A, &N) >= 0) mbedtls_mpi_sub_mpi(&A, &A, &N);
			CHECK(mbedtls_mpi_gcd_modinv_odd(NULL, &I1, &A, &N) == 0, "fast inverse");
			CHECK(xo_builtin_mpi_gcd_modinv_odd(NULL, &I2, &A, &N) == 0, "builtin inverse");
			CHECK(mbedtls_mpi_cmp_mpi(&I1, &I2) == 0, "inverses differ (modulus %d, round %d)", m, i);
			// in place, as ecp.c's normalization calls it
			mbedtls_mpi_copy(&I2, &A);
			CHECK(mbedtls_mpi_gcd_modinv_odd(NULL, &I2, &I2, &N) == 0 && mbedtls_mpi_cmp_mpi(&I1, &I2) == 0, "in place inverse differs");
			mbedtls_mpi_mul_mpi(&P, &I1, &A);
			mbedtls_mpi_mod_mpi(&P, &P, &N);
			CHECK(mbedtls_mpi_cmp_int(&P, 1) == 0, "A * A^-1 != 1 (modulus %d, round %d)", m, i);
		}
		mbedtls_mpi_free(&N); mbedtls_mpi_free(&A); mbedtls_mpi_free(&I1); mbedtls_mpi_free(&I2); mbedtls_mpi_free(&P);
	}
}

// Random P-256 keys sign and verify, deterministic and randomized.
static void sign_verify(int rounds) {
	for (int i = 0; i < rounds; i++) {
		psa_algorithm_t alg = i % 2 ? PSA_ALG_ECDSA(PSA_ALG_SHA_256) : PSA_ALG_DETERMINISTIC_ECDSA(PSA_ALG_SHA_256);
		psa_key_attributes_t a = PSA_KEY_ATTRIBUTES_INIT;
		psa_set_key_type(&a, PSA_KEY_TYPE_ECC_KEY_PAIR(PSA_ECC_FAMILY_SECP_R1));
		psa_set_key_bits(&a, 256);
		psa_set_key_usage_flags(&a, PSA_KEY_USAGE_SIGN_HASH | PSA_KEY_USAGE_VERIFY_HASH);
		psa_set_key_algorithm(&a, alg);
		psa_key_id_t k;
		uint8_t hash[32], sig[64];
		size_t n;
		psa_generate_random(hash, 32);
		CHECK(psa_generate_key(&a, &k) == PSA_SUCCESS, "generate");
		CHECK(psa_sign_hash(k, alg, hash, 32, sig, 64, &n) == PSA_SUCCESS, "sign");
		CHECK(psa_verify_hash(k, alg, hash, 32, sig, n) == PSA_SUCCESS, "verify (round %d)", i);
		hash[0] ^= 1;
		CHECK(psa_verify_hash(k, alg, hash, 32, sig, n) == PSA_ERROR_INVALID_SIGNATURE, "verify of another hash (round %d)", i);
		psa_destroy_key(k);
	}
}

static const uint8_t p256_n_be[32] = {
	0xff, 0xff, 0xff, 0xff, 0x00, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	0xbc, 0xe6, 0xfa, 0xad, 0xa7, 0x17, 0x9e, 0x84, 0xf3, 0xb9, 0xca, 0xc2, 0xfc, 0x63, 0x25, 0x51,
};

static void p256_attrs(psa_key_attributes_t *a, int pair) {
	*a = psa_key_attributes_init();
	psa_set_key_type(a, pair ? PSA_KEY_TYPE_ECC_KEY_PAIR(PSA_ECC_FAMILY_SECP_R1) : PSA_KEY_TYPE_ECC_PUBLIC_KEY(PSA_ECC_FAMILY_SECP_R1));
	psa_set_key_bits(a, 256);
}

// n_minus sets out = n - k (big endian) for a small k.
static void n_minus(uint8_t out[32], int k) {
	memcpy(out, p256_n_be, 32);
	int borrow = k;
	for (int i = 31; i >= 0 && borrow; i--) {
		int v = out[i] - borrow;
		borrow = v < 0;
		out[i] = (uint8_t)(v + (borrow ? 256 : 0));
	}
}

static const psa_algorithm_t det_algs[3] = {
	PSA_ALG_DETERMINISTIC_ECDSA(PSA_ALG_SHA_256),
	PSA_ALG_DETERMINISTIC_ECDSA(PSA_ALG_SHA_384),
	PSA_ALG_DETERMINISTIC_ECDSA(PSA_ALG_SHA_512),
};

// both_verify checks that the fast and the library's verification agree
// on sig, for the public key (and the key pair when d is given), and
// returns the status.
static psa_status_t both_verify(const uint8_t *d, const uint8_t pub[65], psa_algorithm_t alg,
	const uint8_t *hash, size_t hn, const uint8_t *sig, size_t sn, const char *what, int round) {
	psa_key_attributes_t ap, apub;
	p256_attrs(&ap, 1);
	p256_attrs(&apub, 0);
	psa_status_t f1 = mbedtls_psa_ecdsa_verify_hash(&apub, pub, 65, alg, hash, hn, sig, sn);
	psa_status_t b1 = xo_builtin_psa_ecdsa_verify_hash(&apub, pub, 65, alg, hash, hn, sig, sn);
	CHECK(f1 == b1, "verify %s (round %d): fast %d, library %d", what, round, (int)f1, (int)b1);
	if (d != NULL) {
		psa_status_t f2 = mbedtls_psa_ecdsa_verify_hash(&ap, d, 32, alg, hash, hn, sig, sn);
		CHECK(f2 == f1, "verify %s with the key pair (round %d): %d, public key %d", what, round, (int)f2, (int)f1);
	}
	return f1;
}

// ecdsa_p256 compares xo_p256.c's signatures with the library's, byte for
// byte (deterministic ECDSA with SHA-256, -384, -512 and hashes of 0 to
// 64 bytes, edge keys and hashes), and its verification with the
// library's on good, altered, and out of range signatures.
static void ecdsa_p256(int rounds) {
	static const size_t hlens[] = {32, 48, 64, 20, 0, 31, 33};
	psa_key_attributes_t ap;
	p256_attrs(&ap, 1);
	for (int i = 0; i < rounds; i++) {
		uint8_t d[32], hash[64], s1[64], s2[64], pub[65];
		size_t n1 = 0, n2 = 0, pn = 0;
		psa_generate_random(d, 32);
		psa_generate_random(hash, 64);
		if (i % 3 == 0) d[0] &= 0x7f;
		switch (i) {
		case 0: memset(d, 0, 32); d[31] = 1; break;
		case 1: memset(d, 0, 32); d[31] = 2; break;
		case 2: n_minus(d, 1); break;
		case 3: n_minus(d, 2); break;
		case 4: memset(hash, 0, 64); break;
		case 5: memset(hash, 0xff, 64); break;
		case 6: memcpy(hash, p256_n_be, 32); break;
		case 7: n_minus(hash, 1); break;
		case 8: memset(d, 0xff, 32); break; // out of range: both refuse
		case 9: memcpy(d, p256_n_be, 32); break;
		case 10: memset(d, 0, 32); break;
		}
		psa_algorithm_t alg = det_algs[i % 3];
		size_t hn = hlens[i % 7];
		psa_status_t f = mbedtls_psa_ecdsa_sign_hash(&ap, d, 32, alg, hash, hn, s1, 64, &n1);
		psa_status_t b = xo_builtin_psa_ecdsa_sign_hash(&ap, d, 32, alg, hash, hn, s2, 64, &n2);
		CHECK(f == b, "sign status (round %d): fast %d, library %d", i, (int)f, (int)b);
		if (f != PSA_SUCCESS || b != PSA_SUCCESS) {
			CHECK(i >= 8 && i <= 10, "sign failed (round %d)", i);
			continue;
		}
		CHECK(n1 == 64 && n2 == 64 && memcmp(s1, s2, 64) == 0, "signatures differ (round %d, hash %zu bytes)", i, hn);
		CHECK(xo_builtin_ecp_export_public_key(&ap, d, 32, pub, 65, &pn) == PSA_SUCCESS && pn == 65, "public key");
		CHECK(both_verify(d, pub, alg, hash, hn, s1, 64, "good", i) == PSA_SUCCESS, "good signature refused (round %d)", i);
		// altered hash, r, s; short signature; r or s 0, n, n-1
		uint8_t bad[64], h2[64];
		memcpy(h2, hash, 64);
		if (hn) {
			h2[0] ^= 1; // (bytes past the 32nd are not used)
			CHECK(both_verify(d, pub, alg, h2, hn, s1, 64, "other hash", i) == PSA_ERROR_INVALID_SIGNATURE, "other hash accepted (round %d)", i);
		}
		for (int j = 0; j < 9; j++) {
			memcpy(bad, s1, 64);
			switch (j) {
			case 0: bad[5] ^= 0x10; break;
			case 1: bad[40] ^= 0x01; break;
			case 2: memset(bad, 0, 32); break;
			case 3: memset(bad + 32, 0, 32); break;
			case 4: memcpy(bad, p256_n_be, 32); break;
			case 5: memcpy(bad + 32, p256_n_be, 32); break;
			case 6: n_minus(bad + 32, 1); break;
			case 7: memset(bad, 0xff, 64); break;
			case 8: n_minus(bad, 1); break;
			}
			CHECK(both_verify(d, pub, alg, hash, hn, bad, 64, "altered", i * 10 + j) == PSA_ERROR_INVALID_SIGNATURE, "altered signature accepted (round %d, %d)", i, j);
		}
		both_verify(d, pub, alg, hash, hn, s1, 63, "short", i);
		// a randomized signature of the fast path verifies with the library
		psa_algorithm_t ralg = PSA_ALG_ECDSA(PSA_ALG_SHA_256);
		CHECK(mbedtls_psa_ecdsa_sign_hash(&ap, d, 32, ralg, hash, hn, s1, 64, &n1) == PSA_SUCCESS, "randomized sign (round %d)", i);
		CHECK(both_verify(d, pub, ralg, hash, hn, s1, 64, "randomized", i) == PSA_SUCCESS, "randomized signature refused (round %d)", i);
	}
	// a public key off the curve: both refuse it the same way
	uint8_t pub[65] = {4}, sig[64], hash[32] = {1};
	memset(sig, 0x11, 64);
	pub[64] = 1;
	both_verify(NULL, pub, det_algs[0], hash, 32, sig, 64, "off the curve", 0);
}

static int unhexs(const char *s, uint8_t *out, size_t max) {
	size_t n = strlen(s) / 2;
	if (n > max) return -1;
	unhex(s, out, n);
	return (int)n;
}

static void puthex(const uint8_t *b, size_t n) {
	for (size_t i = 0; i < n; i++) printf("%02x", b[i]);
}

// serve answers lines on stdin:
//   S <hash bits: 256, 384, 512> <d hex> <hash hex>  -> "<fast sig> <library sig>"
//   V <public key hex> <hash hex> <signature hex or -> -> "<fast status> <library status>"
// ("-" stands for an empty hash or signature; failures print E<status>.)
static int serve(void) {
	static char line[1 << 16], op[4], a[1 << 14], b[1 << 14], c[1 << 14];
	static uint8_t d[1 << 13], h[1 << 13], sig[1 << 13];
	psa_key_attributes_t ap, apub;
	p256_attrs(&ap, 1);
	p256_attrs(&apub, 0);
	while (fgets(line, sizeof(line), stdin)) {
		uint8_t s1[64], s2[64];
		size_t n1 = 0, n2 = 0;
		if (sscanf(line, "%3s %16383s %16383s %16383s", op, a, b, c) < 3) continue;
		if (op[0] == 'S') {
			int bits = atoi(a);
			psa_algorithm_t alg = det_algs[bits == 384 ? 1 : bits == 512 ? 2 : 0];
			int dn = unhexs(b, d, sizeof(d));
			int hn = strcmp(c, "-") == 0 ? 0 : unhexs(c, h, sizeof(h));
			psa_status_t f = mbedtls_psa_ecdsa_sign_hash(&ap, d, dn, alg, h, hn, s1, 64, &n1);
			psa_status_t l = xo_builtin_psa_ecdsa_sign_hash(&ap, d, dn, alg, h, hn, s2, 64, &n2);
			if (f == PSA_SUCCESS) puthex(s1, 64); else printf("E%d", (int)f);
			printf(" ");
			if (l == PSA_SUCCESS) puthex(s2, 64); else printf("E%d", (int)l);
			printf("\n");
		} else if (op[0] == 'V') {
			int pn = unhexs(a, d, sizeof(d));
			int hn = strcmp(b, "-") == 0 ? 0 : unhexs(b, h, sizeof(h));
			int sn = strcmp(c, "-") == 0 ? 0 : unhexs(c, sig, sizeof(sig));
			psa_status_t f = mbedtls_psa_ecdsa_verify_hash(&apub, d, pn, det_algs[0], h, hn, sig, sn);
			psa_status_t l = xo_builtin_psa_ecdsa_verify_hash(&apub, d, pn, det_algs[0], h, hn, sig, sn);
			printf("%d %d\n", (int)f, (int)l);
		}
		fflush(stdout);
	}
	return 0;
}

static double now_us(void) {
	struct timespec ts;
	clock_gettime(CLOCK_MONOTONIC, &ts);
	return ts.tv_sec * 1e6 + ts.tv_nsec / 1e3;
}

// bench times n deterministic P-256 signatures and verifications of the
// fast path and of the library's own functions on one core, and n
// signatures through psa_sign_hash (key store, locks, dispatch).
static int bench(int n) {
	psa_key_attributes_t ap, apub;
	p256_attrs(&ap, 1);
	p256_attrs(&apub, 0);
	uint8_t d[32], hash[32], sig[64], pub[65];
	size_t sn, pn;
	psa_generate_random(d, 32);
	d[0] &= 0x7f;
	psa_generate_random(hash, 32);
	xo_builtin_ecp_export_public_key(&ap, d, 32, pub, 65, &pn);
	psa_algorithm_t alg = det_algs[0];
	double t0 = now_us();
	mbedtls_psa_ecdsa_sign_hash(&ap, d, 32, alg, hash, 32, sig, 64, &sn); // builds the table
	double tt = now_us();
	for (int i = 0; i < n; i++) { hash[0] = (uint8_t)i; mbedtls_psa_ecdsa_sign_hash(&ap, d, 32, alg, hash, 32, sig, 64, &sn); }
	double t1 = now_us();
	for (int i = 0; i < n; i++) { hash[0] = (uint8_t)i; xo_builtin_psa_ecdsa_sign_hash(&ap, d, 32, alg, hash, 32, sig, 64, &sn); }
	double t2 = now_us();
	for (int i = 0; i < n; i++) mbedtls_psa_ecdsa_verify_hash(&apub, pub, 65, alg, hash, 32, sig, 64);
	double t3 = now_us();
	for (int i = 0; i < n; i++) xo_builtin_psa_ecdsa_verify_hash(&apub, pub, 65, alg, hash, 32, sig, 64);
	double t4 = now_us();
	psa_key_attributes_t a = PSA_KEY_ATTRIBUTES_INIT;
	psa_set_key_type(&a, PSA_KEY_TYPE_ECC_KEY_PAIR(PSA_ECC_FAMILY_SECP_R1));
	psa_set_key_bits(&a, 256);
	psa_set_key_usage_flags(&a, PSA_KEY_USAGE_SIGN_HASH);
	psa_set_key_algorithm(&a, alg);
	psa_key_id_t k;
	psa_import_key(&a, d, 32, &k);
	double t5 = now_us();
	for (int i = 0; i < n; i++) { hash[0] = (uint8_t)i; psa_sign_hash(k, alg, hash, 32, sig, 64, &sn); }
	double t6 = now_us();
	printf("first sign (table) %.0f us; sign: fast %.1f us, library %.1f us; verify: fast %.1f us, library %.1f us; psa_sign_hash %.1f us\n",
		tt - t0, (t1 - tt) / n, (t2 - t1) / n, (t3 - t2) / n, (t4 - t3) / n, (t6 - t5) / n);
	return 0;
}

int main(int argc, char **argv) {
	if (psa_crypto_init() != PSA_SUCCESS) {
		printf("psa_crypto_init failed\n");
		return 1;
	}
	if (argc > 1 && strcmp(argv[1], "serve") == 0) {
		return serve();
	}
	if (argc > 2 && strcmp(argv[1], "bench") == 0) {
		return bench(atoi(argv[2]));
	}
	int rounds = argc > 1 ? atoi(argv[1]) : 200;
	rfc6979();
	rfc7748();
	x25519_random(rounds);
	inverses(rounds);
	sign_verify(rounds / 4);
	ecdsa_p256(rounds * 2);
	if (fails) {
		return 1;
	}
	printf("ok\n");
	return 0;
}
