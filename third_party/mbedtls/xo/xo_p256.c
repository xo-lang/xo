// xo_p256.c: ECDSA P-256 signing and verification for the TLS library
// (decision 0132), in front of Mbed TLS's own through the interposition
// of decision 0117: psa_crypto_ecp.c is compiled with
// -Dmbedtls_psa_ecdsa_sign_hash=xo_builtin_psa_ecdsa_sign_hash (and the
// same for verify_hash), and this file defines the original names.
//
// Field and scalar arithmetic: fiat-crypto's formally verified Montgomery
// code (third_party/fiat-crypto, included below). Points: projective
// coordinates with the complete addition formulas of Renes, Costello,
// and Batina ("Complete addition formulas for prime order elliptic
// curves", 2015, a = -3), as Go's crypto/internal/fips140/nistec does,
// so there is no special case for doubling or the point at infinity.
// k*G: a table of 43 x 32 affine multiples of G (built once, 88 KB),
// signed windows of 6 bits (Booth), every lookup reads all 32 entries
// with masks. Q*u: a 16 entry table, windows of 5 bits. Inverses:
// Fermat with fixed addition chains (the exponents are public).
//
// Signatures are byte identical to the library's: deterministic ECDSA
// draws k from Mbed TLS's own HMAC_DRBG seeded with d || bits2octets(h)
// (RFC 6979), the randomized one from the PSA random generator, with the
// library's rejection sampling (mbedtls_mpi_core_random). Whatever the
// fast path does not cover (other curves, sizes, r or s zero, a key out
// of range, a random generator error) goes to the library's function.
//
// Constant time: no branch and no memory index depends on d, k, or the
// nonce's inverse. Selections are fiat's masked selects (behind its
// empty asm value barrier), table lookups read every entry, window
// positions are public. The only data dependent branches are on public
// values (r == 0, s == 0, the rejection of a drawn k, as in the library).
// TestTLSAccel checks the outputs; decision 0132 says how the absence of
// secret dependent branches was reviewed.

#include "tf_psa_crypto_common.h"
#include <psa/crypto.h>
#include <string.h>
#include <stdlib.h>
#include "psa_crypto_core.h"
#include "psa_crypto_ecp.h"
#include "psa_util_internal.h"
#include "mbedtls/private/hmac_drbg.h"
#include "mbedtls/md.h"

#if defined(MBEDTLS_PSA_BUILTIN_ALG_ECDSA) || \
    defined(MBEDTLS_PSA_BUILTIN_ALG_DETERMINISTIC_ECDSA)

psa_status_t xo_builtin_psa_ecdsa_sign_hash(
    const psa_key_attributes_t *attributes,
    const uint8_t *key_buffer, size_t key_buffer_size,
    psa_algorithm_t alg, const uint8_t *hash, size_t hash_length,
    uint8_t *signature, size_t signature_size, size_t *signature_length);
psa_status_t xo_builtin_psa_ecdsa_verify_hash(
    const psa_key_attributes_t *attributes,
    const uint8_t *key_buffer, size_t key_buffer_size,
    psa_algorithm_t alg, const uint8_t *hash, size_t hash_length,
    const uint8_t *signature, size_t signature_length);

#if defined(__SIZEOF_INT128__) && defined(MBEDTLS_THREADING_PTHREAD)
#define XO_P256_FAST 1
#include <pthread.h>
#include "p256_64.c"
#include "p256_scalar_64.c"

typedef uint64_t fe[4];                  // Montgomery domain, little endian limbs
typedef struct { fe x, y, z; } p256_pt;  // projective (X:Y:Z), x = X/Z; infinity (0:1:0)
typedef struct { fe x, y; } p256_aff;    // affine, never infinity

// p and n as plain limbs
static const uint64_t p256_p[4] = {
    0xffffffffffffffffULL, 0x00000000ffffffffULL, 0x0000000000000000ULL, 0xffffffff00000001ULL
};
static const uint64_t p256_n[4] = {
    0xf3b9cac2fc632551ULL, 0xbce6faada7179e84ULL, 0xffffffffffffffffULL, 0xffffffff00000000ULL
};
static const uint8_t p256_b_be[32] = {
    0x5a, 0xc6, 0x35, 0xd8, 0xaa, 0x3a, 0x93, 0xe7, 0xb3, 0xeb, 0xbd, 0x55, 0x76, 0x98, 0x86, 0xbc,
    0x65, 0x1d, 0x06, 0xb0, 0xcc, 0x53, 0xb0, 0xf6, 0x3b, 0xce, 0x3c, 0x3e, 0x27, 0xd2, 0x60, 0x4b,
};
static const uint8_t p256_gx_be[32] = {
    0x6b, 0x17, 0xd1, 0xf2, 0xe1, 0x2c, 0x42, 0x47, 0xf8, 0xbc, 0xe6, 0xe5, 0x63, 0xa4, 0x40, 0xf2,
    0x77, 0x03, 0x7d, 0x81, 0x2d, 0xeb, 0x33, 0xa0, 0xf4, 0xa1, 0x39, 0x45, 0xd8, 0x98, 0xc2, 0x96,
};
static const uint8_t p256_gy_be[32] = {
    0x4f, 0xe3, 0x42, 0xe2, 0xfe, 0x1a, 0x7f, 0x9b, 0x8e, 0xe7, 0xeb, 0x4a, 0x7c, 0x0f, 0x9e, 0x16,
    0x2b, 0xce, 0x33, 0x57, 0x6b, 0x31, 0x5e, 0xce, 0xcb, 0xb6, 0x40, 0x68, 0x37, 0xbf, 0x51, 0xf5,
};

static fe p256_b;                          // b in the Montgomery domain
static p256_aff p256_gtab[43][32];         // [j][i] = (i+1) * 2^(6j) * G
static int p256_ready;
static pthread_once_t p256_once = PTHREAD_ONCE_INIT;

// ---- limbs and bytes ----

static void limbs_from_be(uint64_t out[4], const uint8_t in[32])
{
    for (int i = 0; i < 4; i++) {
        uint64_t w = 0;
        for (int j = 0; j < 8; j++) {
            w = (w << 8) | in[(3 - i) * 8 + j];
        }
        out[i] = w;
    }
}

static void limbs_to_be(uint8_t out[32], const uint64_t in[4])
{
    for (int i = 0; i < 4; i++) {
        for (int j = 0; j < 8; j++) {
            out[(3 - i) * 8 + j] = (uint8_t) (in[i] >> (56 - 8 * j));
        }
    }
}

// limbs_lt is 1 when a < b, else 0, in constant time.
static fiat_p256_uint1 limbs_lt(const uint64_t a[4], const uint64_t b[4])
{
    uint64_t t;
    fiat_p256_uint1 c = 0;
    for (int i = 0; i < 4; i++) {
        fiat_p256_subborrowx_u64(&t, &c, c, a[i], b[i]);
    }
    return c;
}

// limbs_reduce sets a to a - m when a >= m (a < 2m), in constant time.
static void limbs_reduce(uint64_t a[4], const uint64_t m[4])
{
    uint64_t t[4];
    fiat_p256_uint1 c = 0;
    for (int i = 0; i < 4; i++) {
        fiat_p256_subborrowx_u64(&t[i], &c, c, a[i], m[i]);
    }
    fiat_p256_selectznz(a, c, t, a);
}

static uint64_t limbs_nonzero(const uint64_t a[4])
{
    return (a[0] | a[1] | a[2] | a[3]) != 0;
}

// ct_eq is 1 when a == b (both below 2^63), else 0, without a branch.
static fiat_p256_uint1 ct_eq(uint64_t a, uint64_t b)
{
    uint64_t x = fiat_p256_value_barrier_u64(a ^ b);
    return (fiat_p256_uint1) (1 ^ ((x | (0 - x)) >> 63));
}

// ---- field ----

static void fe_from_be(fe out, const uint8_t in[32])
{
    uint64_t t[4];
    limbs_from_be(t, in);
    fiat_p256_to_montgomery(out, t);
}

static void fe_to_limbs(uint64_t out[4], const fe a)
{
    fiat_p256_from_montgomery(out, a);
}

static void fe_sqn(fe out, const fe a, int n)
{
    fiat_p256_square(out, a);
    for (int i = 1; i < n; i++) {
        fiat_p256_square(out, out);
    }
}

// fe_inv sets out = a^(p-2) = 1/a (0 for 0): Go's addition chain
// (fiat/p256_invert.go, 12 multiplications, 255 squarings).
static void fe_inv(fe out, const fe x)
{
    fe z, t0, t1;
    fiat_p256_square(z, x);
    fiat_p256_mul(z, x, z);
    fiat_p256_square(z, z);
    fiat_p256_mul(z, x, z);
    fe_sqn(t0, z, 3);
    fiat_p256_mul(t0, z, t0);
    fe_sqn(t1, t0, 6);
    fiat_p256_mul(t0, t0, t1);
    fe_sqn(t0, t0, 3);
    fiat_p256_mul(z, z, t0);
    fiat_p256_square(t0, z);
    fiat_p256_mul(t0, x, t0);
    fe_sqn(t1, t0, 16);
    fiat_p256_mul(t0, t0, t1);
    fe_sqn(t0, t0, 15);
    fiat_p256_mul(z, z, t0);
    fe_sqn(t0, t0, 17);
    fiat_p256_mul(t0, x, t0);
    fe_sqn(t0, t0, 143);
    fiat_p256_mul(t0, z, t0);
    fe_sqn(t0, t0, 47);
    fiat_p256_mul(z, z, t0);
    fe_sqn(z, z, 2);
    fiat_p256_mul(out, x, z);
}

// ---- points (formulas from eprint 2015/1060, as Go's nistec) ----

static void pt_identity(p256_pt *p)
{
    memset(p, 0, sizeof(*p));
    fiat_p256_set_one(p->y);
}

// pt_add sets q = p1 + p2 (Algorithm 4); the points may overlap.
static void pt_add(p256_pt *q, const p256_pt *p1, const p256_pt *p2)
{
    fe t0, t1, t2, t3, t4, x3, y3, z3;
    fiat_p256_mul(t0, p1->x, p2->x);
    fiat_p256_mul(t1, p1->y, p2->y);
    fiat_p256_mul(t2, p1->z, p2->z);
    fiat_p256_add(t3, p1->x, p1->y);
    fiat_p256_add(t4, p2->x, p2->y);
    fiat_p256_mul(t3, t3, t4);
    fiat_p256_add(t4, t0, t1);
    fiat_p256_sub(t3, t3, t4);
    fiat_p256_add(t4, p1->y, p1->z);
    fiat_p256_add(x3, p2->y, p2->z);
    fiat_p256_mul(t4, t4, x3);
    fiat_p256_add(x3, t1, t2);
    fiat_p256_sub(t4, t4, x3);
    fiat_p256_add(x3, p1->x, p1->z);
    fiat_p256_add(y3, p2->x, p2->z);
    fiat_p256_mul(x3, x3, y3);
    fiat_p256_add(y3, t0, t2);
    fiat_p256_sub(y3, x3, y3);
    fiat_p256_mul(z3, p256_b, t2);
    fiat_p256_sub(x3, y3, z3);
    fiat_p256_add(z3, x3, x3);
    fiat_p256_add(x3, x3, z3);
    fiat_p256_sub(z3, t1, x3);
    fiat_p256_add(x3, t1, x3);
    fiat_p256_mul(y3, p256_b, y3);
    fiat_p256_add(t1, t2, t2);
    fiat_p256_add(t2, t1, t2);
    fiat_p256_sub(y3, y3, t2);
    fiat_p256_sub(y3, y3, t0);
    fiat_p256_add(t1, y3, y3);
    fiat_p256_add(y3, t1, y3);
    fiat_p256_add(t1, t0, t0);
    fiat_p256_add(t0, t1, t0);
    fiat_p256_sub(t0, t0, t2);
    fiat_p256_mul(t1, t4, y3);
    fiat_p256_mul(t2, t0, y3);
    fiat_p256_mul(y3, x3, z3);
    fiat_p256_add(y3, y3, t2);
    fiat_p256_mul(x3, t3, x3);
    fiat_p256_sub(x3, x3, t1);
    fiat_p256_mul(z3, t4, z3);
    fiat_p256_mul(t1, t3, t0);
    fiat_p256_add(z3, z3, t1);
    memcpy(q->x, x3, sizeof(fe));
    memcpy(q->y, y3, sizeof(fe));
    memcpy(q->z, z3, sizeof(fe));
}

// pt_double sets q = 2p (Algorithm 6); the points may overlap.
static void pt_double(p256_pt *q, const p256_pt *p)
{
    fe t0, t1, t2, t3, x3, y3, z3;
    fiat_p256_square(t0, p->x);
    fiat_p256_square(t1, p->y);
    fiat_p256_square(t2, p->z);
    fiat_p256_mul(t3, p->x, p->y);
    fiat_p256_add(t3, t3, t3);
    fiat_p256_mul(z3, p->x, p->z);
    fiat_p256_add(z3, z3, z3);
    fiat_p256_mul(y3, p256_b, t2);
    fiat_p256_sub(y3, y3, z3);
    fiat_p256_add(x3, y3, y3);
    fiat_p256_add(y3, x3, y3);
    fiat_p256_sub(x3, t1, y3);
    fiat_p256_add(y3, t1, y3);
    fiat_p256_mul(y3, x3, y3);
    fiat_p256_mul(x3, x3, t3);
    fiat_p256_add(t3, t2, t2);
    fiat_p256_add(t2, t2, t3);
    fiat_p256_mul(z3, p256_b, z3);
    fiat_p256_sub(z3, z3, t2);
    fiat_p256_sub(z3, z3, t0);
    fiat_p256_add(t3, z3, z3);
    fiat_p256_add(z3, z3, t3);
    fiat_p256_add(t3, t0, t0);
    fiat_p256_add(t0, t3, t0);
    fiat_p256_sub(t0, t0, t2);
    fiat_p256_mul(t0, t0, z3);
    fiat_p256_add(y3, y3, t0);
    fiat_p256_mul(t0, p->y, p->z);
    fiat_p256_add(t0, t0, t0);
    fiat_p256_mul(z3, t0, z3);
    fiat_p256_sub(x3, x3, z3);
    fiat_p256_mul(z3, t0, t1);
    fiat_p256_add(z3, z3, z3);
    fiat_p256_add(z3, z3, z3);
    memcpy(q->x, x3, sizeof(fe));
    memcpy(q->y, y3, sizeof(fe));
    memcpy(q->z, z3, sizeof(fe));
}

// pt_add_affine sets q = p1 + p2 (Algorithm 5), or q = p1 when inf is 1
// (p2 then holds anything); the points may overlap.
static void pt_add_affine(p256_pt *q, const p256_pt *p1, const p256_aff *p2, fiat_p256_uint1 inf)
{
    fe t0, t1, t2, t3, t4, x3, y3, z3;
    fiat_p256_mul(t0, p1->x, p2->x);
    fiat_p256_mul(t1, p1->y, p2->y);
    fiat_p256_add(t3, p2->x, p2->y);
    fiat_p256_add(t4, p1->x, p1->y);
    fiat_p256_mul(t3, t3, t4);
    fiat_p256_add(t4, t0, t1);
    fiat_p256_sub(t3, t3, t4);
    fiat_p256_mul(t4, p2->y, p1->z);
    fiat_p256_add(t4, t4, p1->y);
    fiat_p256_mul(y3, p2->x, p1->z);
    fiat_p256_add(y3, y3, p1->x);
    fiat_p256_mul(z3, p256_b, p1->z);
    fiat_p256_sub(x3, y3, z3);
    fiat_p256_add(z3, x3, x3);
    fiat_p256_add(x3, x3, z3);
    fiat_p256_sub(z3, t1, x3);
    fiat_p256_add(x3, t1, x3);
    fiat_p256_mul(y3, p256_b, y3);
    fiat_p256_add(t1, p1->z, p1->z);
    fiat_p256_add(t2, t1, p1->z);
    fiat_p256_sub(y3, y3, t2);
    fiat_p256_sub(y3, y3, t0);
    fiat_p256_add(t1, y3, y3);
    fiat_p256_add(y3, t1, y3);
    fiat_p256_add(t1, t0, t0);
    fiat_p256_add(t0, t1, t0);
    fiat_p256_sub(t0, t0, t2);
    fiat_p256_mul(t1, t4, y3);
    fiat_p256_mul(t2, t0, y3);
    fiat_p256_mul(y3, x3, z3);
    fiat_p256_add(y3, y3, t2);
    fiat_p256_mul(x3, t3, x3);
    fiat_p256_sub(x3, x3, t1);
    fiat_p256_mul(z3, t4, z3);
    fiat_p256_mul(t1, t3, t0);
    fiat_p256_add(z3, z3, t1);
    fiat_p256_selectznz(q->x, inf, x3, p1->x);
    fiat_p256_selectznz(q->y, inf, y3, p1->y);
    fiat_p256_selectznz(q->z, inf, z3, p1->z);
}

// fe_cneg sets y = -y when neg is 1.
static void fe_cneg(fe y, fiat_p256_uint1 neg)
{
    fe m;
    fiat_p256_opp(m, y);
    fiat_p256_selectznz(y, neg, y, m);
}

// ---- windows (Go's boothW5 and boothW6): a signed digit of |d| <= 16
// (32), sign 1 for negative ----

static void booth(uint64_t in, int w, uint64_t *d, fiat_p256_uint1 *sign)
{
    uint64_t s = ~((in >> w) - 1);
    uint64_t t = ((uint64_t) 1 << (w + 1)) - in - 1;
    t = (t & s) | (in & ~s);
    *d = (t >> 1) + (t & 1);
    *sign = (fiat_p256_uint1) (s & 1);
}

// rsh is the 64 low bits of s >> n (n public, below 256).
static uint64_t rsh(const uint64_t s[4], int n)
{
    int i = n / 64;
    n %= 64;
    uint64_t r = s[i] >> n;
    if (i + 1 < 4 && n != 0) {
        r |= s[i + 1] << (64 - n);
    }
    return r;
}

// base_mult sets p = s * G for s < n (plain limbs).
static void base_mult(p256_pt *p, const uint64_t s[4])
{
    int index = 251;
    uint64_t d;
    fiat_p256_uint1 sign;
    p256_aff t;
    booth(rsh(s, index), 6, &d, &sign);
    for (;;) {
        // t = gtab[(index+1)/6][d-1], reading every entry
        const p256_aff *tab = p256_gtab[(index + 1) / 6];
        memset(&t, 0, sizeof(t));
        for (uint64_t i = 1; i <= 32; i++) {
            fiat_p256_uint1 c = ct_eq(i, d);
            fiat_p256_selectznz(t.x, c, t.x, tab[i - 1].x);
            fiat_p256_selectznz(t.y, c, t.y, tab[i - 1].y);
        }
        fe_cneg(t.y, sign);
        if (index == 251) {
            // p = t, or infinity when d is 0
            p256_pt id;
            pt_identity(&id);
            fiat_p256_uint1 z = ct_eq(d, 0);
            fiat_p256_set_one(p->z);
            memcpy(p->x, t.x, sizeof(fe));
            memcpy(p->y, t.y, sizeof(fe));
            fiat_p256_selectznz(p->x, z, p->x, id.x);
            fiat_p256_selectznz(p->y, z, p->y, id.y);
            fiat_p256_selectznz(p->z, z, p->z, id.z);
        } else {
            pt_add_affine(p, p, &t, ct_eq(d, 0));
        }
        if (index < 5) {
            break;
        }
        index -= 6;
        if (index >= 0) {
            booth(rsh(s, index) & 0x7f, 6, &d, &sign);
        } else {
            booth((s[0] << 1) & 0x7f, 6, &d, &sign);
        }
    }
}

// point_mult sets p = s * q for s < n (plain limbs).
static void point_mult(p256_pt *p, const p256_pt *q, const uint64_t s[4])
{
    p256_pt table[16], t;
    table[0] = *q;
    for (int i = 1; i < 16; i += 2) {
        pt_double(&table[i], &table[i / 2]);
        if (i + 1 < 16) {
            pt_add(&table[i + 1], &table[i], q);
        }
    }
    int index = 254;
    uint64_t d;
    fiat_p256_uint1 sign;
    booth(rsh(s, index), 5, &d, &sign);
    for (;;) {
        pt_identity(&t);
        for (uint64_t i = 1; i <= 16; i++) {
            fiat_p256_uint1 c = ct_eq(i, d);
            fiat_p256_selectznz(t.x, c, t.x, table[i - 1].x);
            fiat_p256_selectznz(t.y, c, t.y, table[i - 1].y);
            fiat_p256_selectznz(t.z, c, t.z, table[i - 1].z);
        }
        if (index == 254) {
            *p = t;
        } else {
            fe_cneg(t.y, sign);
            pt_add(p, p, &t);
        }
        if (index < 4) {
            break;
        }
        index -= 5;
        for (int k = 0; k < 5; k++) {
            pt_double(p, p);
        }
        if (index >= 0) {
            booth(rsh(s, index) & 0x3f, 5, &d, &sign);
        } else {
            booth((s[0] << 1) & 0x3f, 5, &d, &sign);
        }
    }
}

// pt_affine_x sets x to the affine x of p as plain limbs; it returns 0
// for the point at infinity.
static int pt_affine_x(uint64_t x[4], const p256_pt *p)
{
    fe zi, t;
    fe_inv(zi, p->z);
    fiat_p256_mul(t, p->x, zi);
    fe_to_limbs(x, t);
    uint64_t nz;
    fiat_p256_nonzero(&nz, p->z);
    return nz != 0;
}

// p256_init builds the generator table: 43 x 32 points in projective
// coordinates, then one batched inversion (Montgomery's trick) for all
// the Z coordinates.
static void p256_init(void)
{
    enum { N = 43 * 32 };
    p256_pt *pts = malloc(N * sizeof(p256_pt));
    fe *acc = malloc(N * sizeof(fe));
    if (pts == NULL || acc == NULL) {
        free(pts);
        free(acc);
        return;
    }
    fe_from_be(p256_b, p256_b_be);
    p256_pt base;
    fe_from_be(base.x, p256_gx_be);
    fe_from_be(base.y, p256_gy_be);
    fiat_p256_set_one(base.z);
    for (int j = 0; j < 43; j++) {
        pts[j * 32] = base;
        for (int i = 1; i < 32; i++) {
            pt_add(&pts[j * 32 + i], &pts[j * 32 + i - 1], &base);
        }
        for (int k = 0; k < 6; k++) {
            pt_double(&base, &base);
        }
    }
    // acc[i] = z0 * ... * zi; inv = 1 / acc[N-1]; walk back
    memcpy(acc[0], pts[0].z, sizeof(fe));
    for (int i = 1; i < N; i++) {
        fiat_p256_mul(acc[i], acc[i - 1], pts[i].z);
    }
    fe inv, zi;
    fe_inv(inv, acc[N - 1]);
    for (int i = N - 1; i >= 0; i--) {
        if (i > 0) {
            fiat_p256_mul(zi, inv, acc[i - 1]);
            fiat_p256_mul(inv, inv, pts[i].z);
        } else {
            memcpy(zi, inv, sizeof(fe));
        }
        p256_aff *a = &p256_gtab[i / 32][i % 32];
        fiat_p256_mul(a->x, pts[i].x, zi);
        fiat_p256_mul(a->y, pts[i].y, zi);
    }
    free(pts);
    free(acc);
    p256_ready = 1;
}

// ---- scalars modulo n (Montgomery domain of fiat's p256_scalar) ----

static void sc_sqn(fe out, const fe a, int n)
{
    fiat_p256_scalar_square(out, a);
    for (int i = 1; i < n; i++) {
        fiat_p256_scalar_square(out, out);
    }
}

// sc_inv sets out = x^(n-2) (Montgomery in and out): Go's chain
// (nistec/p256_ordinv.go, 38 multiplications, 254 squarings).
static void sc_inv(fe out, const fe one_x)
{
    fe _1, _11, _101, _111, _1111, _10101, _101111, x, t;
    memcpy(_1, one_x, sizeof(fe));
    fiat_p256_scalar_square(x, _1);
    fiat_p256_scalar_mul(_11, x, _1);
    fiat_p256_scalar_mul(_101, x, _11);
    fiat_p256_scalar_mul(_111, x, _101);
    fiat_p256_scalar_square(x, _101);
    fiat_p256_scalar_mul(_1111, _101, x);
    fiat_p256_scalar_square(t, x);
    fiat_p256_scalar_mul(_10101, t, _1);
    fiat_p256_scalar_square(x, _10101);
    fiat_p256_scalar_mul(_101111, _101, x);
    fiat_p256_scalar_mul(x, _10101, x);
    sc_sqn(t, x, 2);
    fiat_p256_scalar_mul(t, t, _11);
    sc_sqn(x, t, 8);
    fiat_p256_scalar_mul(x, x, t);
    sc_sqn(t, x, 16);
    fiat_p256_scalar_mul(t, t, x);
    sc_sqn(x, t, 64);
    fiat_p256_scalar_mul(x, x, t);
    sc_sqn(x, x, 32);
    fiat_p256_scalar_mul(x, x, t);
    static const int sqrs[26] = {
        6, 5, 4, 5, 5, 4, 3, 3, 5, 9, 6, 2, 5, 6, 5, 4, 5, 5, 3, 10, 2, 5, 5, 3, 7, 6
    };
    const uint64_t *muls[26] = {
        _101111, _111, _11, _1111, _10101, _101, _101, _101, _111, _101111,
        _1111, _1, _1, _1111, _111, _111, _111, _101, _11, _101111,
        _11, _11, _11, _1, _10101, _1111
    };
    for (int i = 0; i < 26; i++) {
        sc_sqn(x, x, sqrs[i]);
        fiat_p256_scalar_mul(x, x, muls[i]);
    }
    memcpy(out, x, sizeof(fe));
}

// hash_to_e sets e = the leftmost 256 bits of the hash, minus n when at
// least n (Mbed TLS's derive_mpi for P-256).
static void hash_to_e(uint64_t e[4], const uint8_t *hash, size_t hash_length)
{
    uint8_t b[32] = { 0 };
    size_t use = hash_length > 32 ? 32 : hash_length;
    memcpy(b + 32 - use, hash, use);
    limbs_from_be(e, b);
    limbs_reduce(e, p256_n);
}

// p256_sign signs hash with the private key d (32 bytes, big endian)
// into sig (r || s). md_info selects deterministic ECDSA (RFC 6979
// through HMAC_DRBG); NULL draws k from the PSA random generator. It
// returns 0, or -1 where the library's function must do it instead.
static int p256_sign(const uint8_t d_be[32], const uint8_t *hash, size_t hash_length,
                     const mbedtls_md_info_t *md_info, uint8_t sig[64])
{
    uint64_t d[4], e[4], k[4], r[4], s[4];
    fe dm, em, km, rm, sm, kinv;
    uint8_t seed[64], kb[32];
    int ret = -1;
    mbedtls_hmac_drbg_context drbg;
    p256_pt R;

    limbs_from_be(d, d_be);
    // 1 <= d < n, else the library reports the error
    if (!(limbs_nonzero(d) & limbs_lt(d, p256_n))) {
        return -1;
    }
    hash_to_e(e, hash, hash_length);
    mbedtls_hmac_drbg_init(&drbg);
    if (md_info != NULL) {
        memcpy(seed, d_be, 32);
        limbs_to_be(seed + 32, e);
        if (mbedtls_hmac_drbg_seed_buf(&drbg, md_info, seed, 64) != 0) {
            goto done;
        }
    }
    // k: mbedtls_mpi_core_random's rejection sampling (at most 29 draws,
    // 1 <= k < n), from the DRBG or the PSA generator
    int ok = 0;
    for (int tries = 0; tries < 29 && !ok; tries++) {
        int rc = md_info != NULL ? mbedtls_hmac_drbg_random(&drbg, kb, 32)
                                 : mbedtls_psa_get_random(MBEDTLS_PSA_RANDOM_STATE, kb, 32);
        if (rc != 0) {
            goto done;
        }
        limbs_from_be(k, kb);
        ok = (int) (limbs_nonzero(k) & limbs_lt(k, p256_n));
    }
    if (!ok) {
        goto done;
    }
    base_mult(&R, k);
    if (!pt_affine_x(r, &R)) {
        goto done;
    }
    limbs_reduce(r, p256_n);
    if (!limbs_nonzero(r)) {
        goto done; // the library draws the next k
    }
    // s = (e + r d) / k mod n
    fiat_p256_scalar_to_montgomery(dm, d);
    fiat_p256_scalar_to_montgomery(em, e);
    fiat_p256_scalar_to_montgomery(km, k);
    fiat_p256_scalar_to_montgomery(rm, r);
    sc_inv(kinv, km);
    fiat_p256_scalar_mul(sm, rm, dm);
    fiat_p256_scalar_add(sm, sm, em);
    fiat_p256_scalar_mul(sm, sm, kinv);
    fiat_p256_scalar_from_montgomery(s, sm);
    if (!limbs_nonzero(s)) {
        goto done;
    }
    limbs_to_be(sig, r);
    limbs_to_be(sig + 32, s);
    ret = 0;
done:
    mbedtls_hmac_drbg_free(&drbg);
    mbedtls_platform_zeroize(seed, sizeof(seed));
    mbedtls_platform_zeroize(kb, sizeof(kb));
    mbedtls_platform_zeroize(d, sizeof(d));
    mbedtls_platform_zeroize(k, sizeof(k));
    mbedtls_platform_zeroize(dm, sizeof(dm));
    mbedtls_platform_zeroize(km, sizeof(km));
    mbedtls_platform_zeroize(kinv, sizeof(kinv));
    mbedtls_platform_zeroize(&R, sizeof(R));
    return ret;
}

// p256_public sets Q from a public key (65 bytes, 04 || X || Y) or a key
// pair (32 bytes, Q = d G); 0 on success, -1 otherwise.
static int p256_public(p256_pt *Q, const psa_key_attributes_t *attributes,
                       const uint8_t *key, size_t key_size)
{
    if (attributes->type == PSA_KEY_TYPE_ECC_PUBLIC_KEY(PSA_ECC_FAMILY_SECP_R1)) {
        uint64_t x[4], y[4];
        fe lhs, rhs, t;
        if (key_size != 65 || key[0] != 4) {
            return -1;
        }
        limbs_from_be(x, key + 1);
        limbs_from_be(y, key + 33);
        if (!limbs_lt(x, p256_p) || !limbs_lt(y, p256_p)) {
            return -1;
        }
        fiat_p256_to_montgomery(Q->x, x);
        fiat_p256_to_montgomery(Q->y, y);
        fiat_p256_set_one(Q->z);
        // y^2 = x^3 - 3x + b
        fiat_p256_square(rhs, Q->x);
        fiat_p256_mul(rhs, rhs, Q->x);
        fiat_p256_add(t, Q->x, Q->x);
        fiat_p256_add(t, t, Q->x);
        fiat_p256_sub(rhs, rhs, t);
        fiat_p256_add(rhs, rhs, p256_b);
        fiat_p256_square(lhs, Q->y);
        return memcmp(lhs, rhs, sizeof(fe)) == 0 ? 0 : -1;
    }
    if (attributes->type == PSA_KEY_TYPE_ECC_KEY_PAIR(PSA_ECC_FAMILY_SECP_R1) && key_size == 32) {
        uint64_t d[4];
        limbs_from_be(d, key);
        if (!(limbs_nonzero(d) & limbs_lt(d, p256_n))) {
            return -1;
        }
        base_mult(Q, d);
        mbedtls_platform_zeroize(d, sizeof(d));
        return 0;
    }
    return -1;
}

// p256_verify checks sig (r || s) of hash under Q: PSA_SUCCESS or
// PSA_ERROR_INVALID_SIGNATURE, as mbedtls_ecdsa_verify decides.
static psa_status_t p256_verify(const p256_pt *Q, const uint8_t *hash, size_t hash_length,
                                const uint8_t sig[64])
{
    uint64_t r[4], s[4], e[4], u1[4], u2[4], x[4];
    fe rm, sm, em, w, t;
    p256_pt A, B;
    limbs_from_be(r, sig);
    limbs_from_be(s, sig + 32);
    if (!limbs_nonzero(r) || !limbs_lt(r, p256_n) || !limbs_nonzero(s) || !limbs_lt(s, p256_n)) {
        return PSA_ERROR_INVALID_SIGNATURE;
    }
    hash_to_e(e, hash, hash_length);
    fiat_p256_scalar_to_montgomery(rm, r);
    fiat_p256_scalar_to_montgomery(sm, s);
    fiat_p256_scalar_to_montgomery(em, e);
    sc_inv(w, sm);
    fiat_p256_scalar_mul(t, em, w);
    fiat_p256_scalar_from_montgomery(u1, t);
    fiat_p256_scalar_mul(t, rm, w);
    fiat_p256_scalar_from_montgomery(u2, t);
    base_mult(&A, u1);
    point_mult(&B, Q, u2);
    pt_add(&A, &A, &B);
    if (!pt_affine_x(x, &A)) {
        return PSA_ERROR_INVALID_SIGNATURE;
    }
    limbs_reduce(x, p256_n);
    return memcmp(x, r, sizeof(x)) == 0 ? PSA_SUCCESS : PSA_ERROR_INVALID_SIGNATURE;
}

static int p256_start(void)
{
    pthread_once(&p256_once, p256_init);
    return p256_ready;
}
#endif /* __SIZEOF_INT128__ && MBEDTLS_THREADING_PTHREAD */

// mbedtls_psa_ecdsa_sign_hash: P-256 signatures, deterministic (TLS's
// MBEDTLS_PK_ALG_ECDSA) or randomized, through the code above; the
// library's function for everything else.
psa_status_t mbedtls_psa_ecdsa_sign_hash(
    const psa_key_attributes_t *attributes,
    const uint8_t *key_buffer, size_t key_buffer_size,
    psa_algorithm_t alg, const uint8_t *hash, size_t hash_length,
    uint8_t *signature, size_t signature_size, size_t *signature_length)
{
#if defined(XO_P256_FAST)
    if (attributes->type == PSA_KEY_TYPE_ECC_KEY_PAIR(PSA_ECC_FAMILY_SECP_R1) &&
        attributes->bits == 256 && key_buffer_size == 32 && signature_size >= 64 &&
        p256_start()) {
        const mbedtls_md_info_t *md_info = NULL;
        int ok = 1;
        if (PSA_ALG_ECDSA_IS_DETERMINISTIC(alg)) {
#if defined(MBEDTLS_PSA_BUILTIN_ALG_DETERMINISTIC_ECDSA)
            md_info = mbedtls_md_info_from_type(
                mbedtls_md_type_from_psa_alg(PSA_ALG_SIGN_GET_HASH(alg)));
            ok = md_info != NULL;
#else
            ok = 0;
#endif
        }
        if (ok && p256_sign(key_buffer, hash, hash_length, md_info, signature) == 0) {
            *signature_length = 64;
            return PSA_SUCCESS;
        }
    }
#endif
    return xo_builtin_psa_ecdsa_sign_hash(attributes, key_buffer, key_buffer_size, alg,
                                          hash, hash_length, signature, signature_size,
                                          signature_length);
}

// mbedtls_psa_ecdsa_verify_hash: P-256 verification (the client's
// certificate chain and handshake signature) through the code above.
psa_status_t mbedtls_psa_ecdsa_verify_hash(
    const psa_key_attributes_t *attributes,
    const uint8_t *key_buffer, size_t key_buffer_size,
    psa_algorithm_t alg, const uint8_t *hash, size_t hash_length,
    const uint8_t *signature, size_t signature_length)
{
#if defined(XO_P256_FAST)
    p256_pt Q;
    if (attributes->bits == 256 && signature_length == 64 && p256_start() &&
        p256_public(&Q, attributes, key_buffer, key_buffer_size) == 0) {
        return p256_verify(&Q, hash, hash_length, signature);
    }
#endif
    return xo_builtin_psa_ecdsa_verify_hash(attributes, key_buffer, key_buffer_size, alg,
                                            hash, hash_length, signature, signature_length);
}
#endif
