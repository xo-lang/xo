// tls_accel.c: TestTLSAccel's program. Checks the functions
// third_party/mbedtls/xo/xo_tls_accel.c puts in front of Mbed TLS's own
// (decision 0117) against known answers (RFC 6979 A.2.5, RFC 7748 6.1)
// and against the library's renamed originals on random inputs.
#define MBEDTLS_DECLARE_PRIVATE_IDENTIFIERS
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "psa/crypto.h"
#include "mbedtls/private/bignum.h"

psa_status_t xo_builtin_ecp_export_public_key(const psa_key_attributes_t *attributes,
	const uint8_t *key_buffer, size_t key_buffer_size, uint8_t *data, size_t data_size, size_t *data_length);
psa_status_t mbedtls_psa_ecp_export_public_key(const psa_key_attributes_t *attributes,
	const uint8_t *key_buffer, size_t key_buffer_size, uint8_t *data, size_t data_size, size_t *data_length);
int xo_builtin_mpi_gcd_modinv_odd(mbedtls_mpi *G, mbedtls_mpi *I, const mbedtls_mpi *A, const mbedtls_mpi *N);
int mbedtls_mpi_gcd_modinv_odd(mbedtls_mpi *G, mbedtls_mpi *I, const mbedtls_mpi *A, const mbedtls_mpi *N);

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

int main(int argc, char **argv) {
	int rounds = argc > 1 ? atoi(argv[1]) : 200;
	if (psa_crypto_init() != PSA_SUCCESS) {
		printf("psa_crypto_init failed\n");
		return 1;
	}
	rfc6979();
	rfc7748();
	x25519_random(rounds);
	inverses(rounds);
	sign_verify(rounds / 4);
	if (fails) {
		return 1;
	}
	printf("ok\n");
	return 0;
}
