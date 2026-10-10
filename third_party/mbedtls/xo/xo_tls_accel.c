// xo_tls_accel.c: faster paths the Xo runtime puts in front of the
// library's own functions (decision 0117). The vendored sources stay
// unmodified: the library file that defines a function replaced here is
// compiled with -D<name>=xo_builtin_<...> (FileDefines in mbedtls.go), so
// its definition gets another name, and this file defines the original
// name, handling the fast case and calling the renamed function for
// everything else.

#include "tf_psa_crypto_common.h"
#include <psa/crypto.h>
#include <string.h>
#include "psa_crypto_core.h"
#include "psa_crypto_ecp.h"

#if defined(MBEDTLS_PSA_BUILTIN_KEY_TYPE_ECC_KEY_PAIR_EXPORT) || \
    defined(MBEDTLS_PSA_BUILTIN_KEY_TYPE_ECC_PUBLIC_KEY)
#if defined(MBEDTLS_ECDH_VARIANT_EVEREST_ENABLED)
#include "tf-psa-crypto/private/everest/x25519.h"
#endif

psa_status_t xo_builtin_ecp_export_public_key(
    const psa_key_attributes_t *attributes,
    const uint8_t *key_buffer, size_t key_buffer_size,
    uint8_t *data, size_t data_size, size_t *data_length);

// mbedtls_psa_ecp_export_public_key: the public key of an X25519 key pair
// (every TLS 1.3 handshake makes one for its key share) through HACL*'s
// Curve25519 (Everest), which the library uses for the key agreement but
// not here: its generic Montgomery ladder on bignums took about 40 percent
// of a server handshake's CPU (2026-10-10). X25519(k, 9) with the scalar
// clamped as RFC 7748 says, the same bytes as the ladder's (TestTLSAccel).
psa_status_t mbedtls_psa_ecp_export_public_key(
    const psa_key_attributes_t *attributes,
    const uint8_t *key_buffer, size_t key_buffer_size,
    uint8_t *data, size_t data_size, size_t *data_length)
{
#if defined(MBEDTLS_ECDH_VARIANT_EVEREST_ENABLED)
    if (attributes->type == PSA_KEY_TYPE_ECC_KEY_PAIR(PSA_ECC_FAMILY_MONTGOMERY) &&
        attributes->bits == 255 && key_buffer_size == MBEDTLS_X25519_KEY_SIZE_BYTES) {
        static const uint8_t base[MBEDTLS_X25519_KEY_SIZE_BYTES] = { 9 };
        if (data_size < MBEDTLS_X25519_KEY_SIZE_BYTES) {
            return PSA_ERROR_BUFFER_TOO_SMALL;
        }
        mbedtls_x25519_scalarmult(data, key_buffer, base);
        *data_length = MBEDTLS_X25519_KEY_SIZE_BYTES;
        return PSA_SUCCESS;
    }
#endif
    return xo_builtin_ecp_export_public_key(attributes, key_buffer, key_buffer_size,
                                            data, data_size, data_length);
}
#endif

#if defined(MBEDTLS_BIGNUM_C) && defined(MBEDTLS_THREADING_PTHREAD)
#include <pthread.h>
#include "mbedtls/private/bignum.h"
#include "bignum_internal.h"

int xo_builtin_mpi_gcd_modinv_odd(mbedtls_mpi *G, mbedtls_mpi *I,
                                  const mbedtls_mpi *A, const mbedtls_mpi *N);

// The prime modulus and the group order of P-256, their exponents for
// Fermat's inverse (m - 2), and R^2 mod m (exp_mod's cache, filled here
// once so later calls only read it).
static const char *const inv_hex[2] = {
    "FFFFFFFF00000001000000000000000000000000FFFFFFFFFFFFFFFFFFFFFFFF",
    "FFFFFFFF00000000FFFFFFFFFFFFFFFFBCE6FAADA7179E84F3B9CAC2FC632551",
};
static mbedtls_mpi inv_m[2], inv_e[2], inv_rr[2];
static int inv_ready;
static pthread_once_t inv_once = PTHREAD_ONCE_INIT;

static void inv_init(void)
{
    mbedtls_mpi one, t;
    mbedtls_mpi_init(&one);
    mbedtls_mpi_init(&t);
    int ok = mbedtls_mpi_lset(&one, 1) == 0;
    for (int i = 0; i < 2; i++) {
        mbedtls_mpi_init(&inv_m[i]);
        mbedtls_mpi_init(&inv_e[i]);
        mbedtls_mpi_init(&inv_rr[i]);
        ok = ok && mbedtls_mpi_read_string(&inv_m[i], 16, inv_hex[i]) == 0 &&
             mbedtls_mpi_sub_int(&inv_e[i], &inv_m[i], 2) == 0 &&
             mbedtls_mpi_exp_mod_unsafe(&t, &one, &inv_e[i], &inv_m[i], &inv_rr[i]) == 0;
    }
    mbedtls_mpi_free(&one);
    mbedtls_mpi_free(&t);
    inv_ready = ok;
}

// mbedtls_mpi_gcd_modinv_odd: the inverses of ECDSA and of the curve
// arithmetic (I = A^-1 mod N, no GCD wanted) modulo P-256's prime or
// order, as A^(N-2) mod N (Fermat; both are prime). The library's
// constant time binary GCD took about a third of a P-256 signature
// (2026-10-10); its Montgomery exponentiation takes half the time. The
// exponent is public and the multiplications are constant time, so the
// secret A leaks no more than through the GCD. Where A has no inverse
// (0 or N) the result is 0, which the contract allows ("indeterminate").
int mbedtls_mpi_gcd_modinv_odd(mbedtls_mpi *G, mbedtls_mpi *I,
                               const mbedtls_mpi *A, const mbedtls_mpi *N)
{
    if (G == NULL && I != NULL && A != N && A->s > 0) {
        pthread_once(&inv_once, inv_init);
        for (int i = 0; inv_ready && i < 2; i++) {
            if (mbedtls_mpi_cmp_mpi(N, &inv_m[i]) == 0) {
                return mbedtls_mpi_exp_mod_unsafe(I, A, &inv_e[i], N, &inv_rr[i]);
            }
        }
    }
    return xo_builtin_mpi_gcd_modinv_odd(G, I, A, N);
}
#endif

#if defined(MBEDTLS_THREADING_PTHREAD)
#include <pthread.h>
#include "threading_internal.h"

// A pthread mutex that spins briefly before it blocks. Every PSA call
// takes the library's one key slot mutex (about 25 times per handshake),
// each time for a few hundred nanoseconds; with a thread per CPU serving
// handshakes, the plain lock put a contending thread to sleep in the
// kernel and woke it with another system call (about a quarter of the
// worker threads' time was __psynch_mutexwait, 2026-10-10). The mutex is
// still a pthread mutex, so the library's condition variables work with
// it unchanged.
static int xo_tls_mutex_lock(mbedtls_platform_mutex_t *m)
{
    for (int i = 0; i < 100; i++) {
        if (pthread_mutex_trylock(m) == 0) {
            return 0;
        }
#if defined(__aarch64__)
        __asm__ volatile ("yield");
#elif defined(__x86_64__)
        __asm__ volatile ("pause");
#endif
    }
    return pthread_mutex_lock(m) == 0 ? 0 : MBEDTLS_ERR_THREADING_USAGE_ERROR;
}

// xo_tls_threading_init installs the lock; xo_tls.c calls it once, before
// psa_crypto_init, inside its pthread_once (so every later lock call of
// any thread sees it).
void xo_tls_threading_init(void)
{
    mbedtls_mutex_lock_ptr = xo_tls_mutex_lock;
}
#else
void xo_tls_threading_init(void)
{
}
#endif
