// xo_crypto.c: what stays C of std/crypto on the native backends
// (decision 0083), included by xo_std.c: crypto.equal and Ed25519, which
// must be constant time (the zero-C audit's B09). The results are those
// of std/rt's crypto.go (Go's crypto/subtle and crypto/ed25519).
// std/llrt/testdata/crypto_diff.c checks them against Go (internal/llvm's
// TestCryptoDiff) and the RFC 8032 vectors. The Rand capability is
// std/native/llrt/rand.xo (B08); its C below stays for the seed.
//
// Static helpers are prefixed cr_ (gotchas/llrt-static-names-in-tests.md).

// ---------------------------------------------------------------------------
// Seed compatibility (B08, decision 0216): Rand is std/native/llrt/rand.xo,
// which the SSA builders call since B08, but the prebuilt seed
// (0.9.0-9b6cd014) still lowers Rand to the C functions below, and it
// builds this checkout's xo with this runtime
// (gotchas/runtime-c-the-seed-still-calls.md). No program a later xo
// builds calls them; delete this block (and xo_win.c's xo_w_entropy)
// once the seed is a release with B08.

#if defined(__APPLE__)
#include <sys/random.h>
#elif defined(__linux__)
#include <sys/random.h>
#endif

// ---------------------------------------------------------------------------
// Random bytes (the operating system's generator, as crypto/rand)

// cr_entropy fills p with n bytes from getentropy (macOS) or getrandom
// (Linux); faults when the system has none.
static void cr_entropy(uint8_t *p, int64_t n) {
	while (n > 0) {
		size_t k = n > 256 ? 256 : (size_t)n;
#if defined(__linux__)
		ssize_t got = getrandom(p, k, 0);
		if (got < 0) {
			if (errno == EINTR) {
				continue;
			}
			fault_text("the system random number generator failed", "");
		}
		k = (size_t)got;
#elif defined(_WIN32)
		if (xo_w_entropy(p, k) != 0) {
			fault_text("the system random number generator failed", "");
		}
#else
		if (getentropy(p, k) != 0) {
			fault_text("the system random number generator failed", "");
		}
#endif
		p += k;
		n -= (int64_t)k;
	}
}

static uint64_t tw_rand_u64(void);

static uint64_t cr_rand_u64(void) {
	if (tw.on) {
		// a test's Rand is seeded (std/rt's seededRand)
		return tw_rand_u64();
	}
	uint8_t b[8];
	cr_entropy(b, 8);
	uint64_t v = 0;
	for (int i = 0; i < 8; i++) {
		v |= (uint64_t)b[i] << (8 * i);
	}
	return v;
}

// xo_rand_bytes is `rand.bytes(n)`: n random bytes (none when n <= 0).
xo_rstr *xo_rand_bytes(int64_t n, int64_t *len) {
	if (n < 0) {
		n = 0;
	}
	xo_rstr *s = rs_new(n);
	if (tw.on) {
		// seeded: one draw per byte, as std/rt's seededRand.M_bytes
		for (int64_t i = 0; i < n; i++) {
			s->data[i] = (char)(uint8_t)tw_rand_u64();
		}
	} else {
		cr_entropy((uint8_t *)s->data, n);
	}
	*len = n;
	return s;
}

// xo_rand_int is `rand.int(lo, hi)`: uniform in [lo, hi) (std/rt's
// randRange); faults on an empty range.
int64_t xo_rand_int(int64_t lo, int64_t hi, const char *loc, int64_t ln) {
	if (hi <= lo) {
		char m[128];
		snprintf(m, sizeof m, "rand.int(%lld, %lld): the range is empty", (long long)lo, (long long)hi);
		fault_at(m, loc, ln);
	}
	uint64_t span = (uint64_t)hi - (uint64_t)lo;
	uint64_t limit = UINT64_MAX - (UINT64_MAX % span);
	for (;;) {
		uint64_t v = cr_rand_u64();
		if (v < limit) {
			return (int64_t)((uint64_t)lo + v % span);
		}
	}
}

// xo_rand_float is `rand.float()`: uniform in [0, 1) with 53 bits.
double xo_rand_float(void) { return (double)(cr_rand_u64() >> 11) / (double)(1ULL << 53); }

// xo_rand_bool is `rand.bool()`.
int64_t xo_rand_bool(void) { return (int64_t)(cr_rand_u64() & 1); }


// SHA-256, HMAC, PBKDF2, BLAKE2b, Argon2, unpadded base64, and
// verify_password are std/native/llrt/digest.xo (decisions 0193 and
// 0195; decision 0198 deleted their C path), SHA-512 too (B08).
// crypto.equal and Ed25519 stay C (constant time).

static uint64_t cr_le64(const uint8_t *p) {
	uint64_t v = 0;
	for (int i = 0; i < 8; i++) {
		v |= (uint64_t)p[i] << (8 * i);
	}
	return v;
}

static void cr_put_le64(uint8_t *p, uint64_t v) {
	for (int i = 0; i < 8; i++) {
		p[i] = (uint8_t)(v >> (8 * i));
	}
}

// xo_crypto_equal is `crypto.equal(a, b)`: subtle.ConstantTimeCompare,
// constant time in the contents (not the lengths).
int64_t xo_crypto_equal(const char *a, int64_t an, const char *b, int64_t bn) {
	if (an != bn) {
		return 0;
	}
	volatile uint8_t d = 0;
	for (int64_t i = 0; i < an; i++) {
		d |= (uint8_t)(a[i] ^ b[i]);
	}
	return d == 0;
}

// ---------------------------------------------------------------------------
// Ed25519 (RFC 8032, decision 0189), as Go's
// crypto/ed25519: a 32 byte seed is the private key; verification
// rejects a non canonical S, accepts the encodings of A that Go's
// edwards25519 accepts (y not reduced, x = 0 with the sign bit), and
// compares the encoding of [S]B - [k]A with R byte for byte. Field
// elements are five 51 bit limbs modulo p = 2^255 - 19; points are
// extended twisted Edwards coordinates (X:Y:Z:T) with the complete
// addition law (it doubles too); scalar multiplication is a double and
// always add ladder with masked selection (constant time in the scalar).

// SHA-512 is std/native/llrt/digest.xo (the zero-C audit's B08), through
// the C ABI export of std/native/llrt/cabi_crypto.xo, which a program
// carries when it uses std/crypto. A program without std/crypto never
// reaches Ed25519 but links this C: the weak definition here keeps it
// linking, and faults if it ever ran.
__attribute__((weak)) void xo_x_sha512_3(int64_t a, int64_t an, int64_t b, int64_t bn, int64_t c, int64_t cn, int64_t out) {
	(void)a, (void)an, (void)b, (void)bn, (void)c, (void)cn, (void)out;
	fault_text("SHA-512 without std/crypto's exports", "");
}

// SHA-512 of up to three byte strings one after the other.
static void cr_sha512_3(const uint8_t *a, int64_t an, const uint8_t *b, int64_t bn, const uint8_t *c, int64_t cn, uint8_t out[64]) {
	xo_x_sha512_3(XO_ADDR(a), an, XO_ADDR(b), bn, XO_ADDR(c), cn, XO_ADDR(out));
}

typedef unsigned __int128 cr_u128;
typedef struct {
	uint64_t v[5];
} cr_fe;

#define CR_M51 ((1ULL << 51) - 1)

static void cr_fe_carry(cr_fe *h) {
	uint64_t c;
	for (int i = 0; i < 4; i++) {
		c = h->v[i] >> 51;
		h->v[i] &= CR_M51;
		h->v[i + 1] += c;
	}
	c = h->v[4] >> 51;
	h->v[4] &= CR_M51;
	h->v[0] += 19 * c;
	c = h->v[0] >> 51;
	h->v[0] &= CR_M51;
	h->v[1] += c;
}

static void cr_fe_set(cr_fe *h, uint64_t x) {
	h->v[0] = x;
	h->v[1] = h->v[2] = h->v[3] = h->v[4] = 0;
}

static void cr_fe_add(cr_fe *h, const cr_fe *f, const cr_fe *g) {
	for (int i = 0; i < 5; i++) {
		h->v[i] = f->v[i] + g->v[i];
	}
	cr_fe_carry(h);
}

// h = f - g, adding 4p so no limb goes below zero (inputs carried).
static void cr_fe_sub(cr_fe *h, const cr_fe *f, const cr_fe *g) {
	h->v[0] = f->v[0] + 0x1FFFFFFFFFFFB4ULL - g->v[0];
	for (int i = 1; i < 5; i++) {
		h->v[i] = f->v[i] + 0x1FFFFFFFFFFFFCULL - g->v[i];
	}
	cr_fe_carry(h);
}

static void cr_fe_neg(cr_fe *h, const cr_fe *f) {
	cr_fe z;
	cr_fe_set(&z, 0);
	cr_fe_sub(h, &z, f);
}

static void cr_fe_mul(cr_fe *h, const cr_fe *f, const cr_fe *g) {
	uint64_t f0 = f->v[0], f1 = f->v[1], f2 = f->v[2], f3 = f->v[3], f4 = f->v[4];
	uint64_t g0 = g->v[0], g1 = g->v[1], g2 = g->v[2], g3 = g->v[3], g4 = g->v[4];
	uint64_t g1_19 = 19 * g1, g2_19 = 19 * g2, g3_19 = 19 * g3, g4_19 = 19 * g4;
	cr_u128 r0 = (cr_u128)f0 * g0 + (cr_u128)f1 * g4_19 + (cr_u128)f2 * g3_19 + (cr_u128)f3 * g2_19 + (cr_u128)f4 * g1_19;
	cr_u128 r1 = (cr_u128)f0 * g1 + (cr_u128)f1 * g0 + (cr_u128)f2 * g4_19 + (cr_u128)f3 * g3_19 + (cr_u128)f4 * g2_19;
	cr_u128 r2 = (cr_u128)f0 * g2 + (cr_u128)f1 * g1 + (cr_u128)f2 * g0 + (cr_u128)f3 * g4_19 + (cr_u128)f4 * g3_19;
	cr_u128 r3 = (cr_u128)f0 * g3 + (cr_u128)f1 * g2 + (cr_u128)f2 * g1 + (cr_u128)f3 * g0 + (cr_u128)f4 * g4_19;
	cr_u128 r4 = (cr_u128)f0 * g4 + (cr_u128)f1 * g3 + (cr_u128)f2 * g2 + (cr_u128)f3 * g1 + (cr_u128)f4 * g0;
	r1 += (uint64_t)(r0 >> 51);
	uint64_t h0 = (uint64_t)r0 & CR_M51;
	r2 += (uint64_t)(r1 >> 51);
	uint64_t h1 = (uint64_t)r1 & CR_M51;
	r3 += (uint64_t)(r2 >> 51);
	uint64_t h2 = (uint64_t)r2 & CR_M51;
	r4 += (uint64_t)(r3 >> 51);
	uint64_t h3 = (uint64_t)r3 & CR_M51;
	uint64_t c = (uint64_t)(r4 >> 51);
	uint64_t h4 = (uint64_t)r4 & CR_M51;
	h0 += 19 * c;
	h1 += h0 >> 51;
	h0 &= CR_M51;
	h->v[0] = h0;
	h->v[1] = h1;
	h->v[2] = h2;
	h->v[3] = h3;
	h->v[4] = h4;
}

static void cr_fe_sq(cr_fe *h, const cr_fe *f) { cr_fe_mul(h, f, f); }

// h = f^(2^k).
static void cr_fe_sqn(cr_fe *h, const cr_fe *f, int k) {
	cr_fe_sq(h, f);
	for (int i = 1; i < k; i++) {
		cr_fe_sq(h, h);
	}
}

// z^(2^250 - 1) and z^11 (ref10's chain), shared by inversion and the
// square root exponent.
static void cr_fe_pow250(cr_fe *z250, cr_fe *z11, const cr_fe *z) {
	cr_fe z2, z9, t, a, b, c;
	cr_fe_sq(&z2, z);
	cr_fe_sqn(&t, &z2, 2);
	cr_fe_mul(&z9, &t, z);
	cr_fe_mul(z11, &z9, &z2);
	cr_fe_sq(&t, z11);
	cr_fe_mul(&a, &t, &z9); // 2^5 - 1
	cr_fe_sqn(&t, &a, 5);
	cr_fe_mul(&b, &t, &a); // 2^10 - 1
	cr_fe_sqn(&t, &b, 10);
	cr_fe_mul(&c, &t, &b); // 2^20 - 1
	cr_fe_sqn(&t, &c, 20);
	cr_fe_mul(&t, &t, &c); // 2^40 - 1
	cr_fe_sqn(&t, &t, 10);
	cr_fe_mul(&a, &t, &b); // 2^50 - 1
	cr_fe_sqn(&t, &a, 50);
	cr_fe_mul(&b, &t, &a); // 2^100 - 1
	cr_fe_sqn(&t, &b, 100);
	cr_fe_mul(&t, &t, &b); // 2^200 - 1
	cr_fe_sqn(&t, &t, 50);
	cr_fe_mul(z250, &t, &a); // 2^250 - 1
}

// h = z^(p - 2) = 1/z.
static void cr_fe_inv(cr_fe *h, const cr_fe *z) {
	cr_fe z250, z11, t;
	cr_fe_pow250(&z250, &z11, z);
	cr_fe_sqn(&t, &z250, 5);
	cr_fe_mul(h, &t, &z11);
}

// h = z^((p - 5) / 8) = z^(2^252 - 3).
static void cr_fe_pow22523(cr_fe *h, const cr_fe *z) {
	cr_fe z250, z11, t;
	cr_fe_pow250(&z250, &z11, z);
	cr_fe_sqn(&t, &z250, 2);
	cr_fe_mul(h, &t, z);
}

// The canonical little endian encoding of f (reduced below p).
static void cr_fe_bytes(uint8_t s[32], const cr_fe *f) {
	cr_fe h = *f;
	cr_fe_carry(&h);
	cr_fe_carry(&h);
	uint64_t q = (h.v[0] + 19) >> 51;
	q = (h.v[1] + q) >> 51;
	q = (h.v[2] + q) >> 51;
	q = (h.v[3] + q) >> 51;
	q = (h.v[4] + q) >> 51;
	h.v[0] += 19 * q;
	for (int i = 0; i < 4; i++) {
		h.v[i + 1] += h.v[i] >> 51;
		h.v[i] &= CR_M51;
	}
	h.v[4] &= CR_M51;
	uint64_t t[4] = {
		h.v[0] | h.v[1] << 51,
		h.v[1] >> 13 | h.v[2] << 38,
		h.v[2] >> 26 | h.v[3] << 25,
		h.v[3] >> 39 | h.v[4] << 12,
	};
	for (int i = 0; i < 4; i++) {
		cr_put_le64(s + 8 * i, t[i]);
	}
}

// h = the 255 low bits of s (not reduced: a value of p or more is kept,
// as edwards25519's SetBytes does).
static void cr_fe_from_bytes(cr_fe *h, const uint8_t s[32]) {
	uint64_t w0 = cr_le64(s), w1 = cr_le64(s + 8), w2 = cr_le64(s + 16), w3 = cr_le64(s + 24);
	h->v[0] = w0 & CR_M51;
	h->v[1] = (w0 >> 51 | w1 << 13) & CR_M51;
	h->v[2] = (w1 >> 38 | w2 << 26) & CR_M51;
	h->v[3] = (w2 >> 25 | w3 << 39) & CR_M51;
	h->v[4] = (w3 >> 12) & CR_M51;
}

static int cr_fe_is_zero(const cr_fe *f) {
	uint8_t s[32];
	cr_fe_bytes(s, f);
	uint8_t d = 0;
	for (int i = 0; i < 32; i++) {
		d |= s[i];
	}
	return d == 0;
}

static int cr_fe_is_odd(const cr_fe *f) {
	uint8_t s[32];
	cr_fe_bytes(s, f);
	return s[0] & 1;
}

typedef struct {
	cr_fe x, y, z, t;
} cr_ge;

static cr_fe cr_ed_d, cr_ed_d2, cr_ed_sqrtm1;
static cr_ge cr_ed_base;
static pthread_once_t cr_ed_once = PTHREAD_ONCE_INIT;

static void cr_ge_identity(cr_ge *p) {
	cr_fe_set(&p->x, 0);
	cr_fe_set(&p->y, 1);
	cr_fe_set(&p->z, 1);
	cr_fe_set(&p->t, 0);
}

// r = p + q (add-2008-hwcd-3; complete for a = -1, so it also doubles).
static void cr_ge_add(cr_ge *r, const cr_ge *p, const cr_ge *q) {
	cr_fe a, b, c, d, e, f, g, h, t;
	cr_fe_sub(&a, &p->y, &p->x);
	cr_fe_sub(&t, &q->y, &q->x);
	cr_fe_mul(&a, &a, &t);
	cr_fe_add(&b, &p->y, &p->x);
	cr_fe_add(&t, &q->y, &q->x);
	cr_fe_mul(&b, &b, &t);
	cr_fe_mul(&c, &p->t, &q->t);
	cr_fe_mul(&c, &c, &cr_ed_d2);
	cr_fe_mul(&d, &p->z, &q->z);
	cr_fe_add(&d, &d, &d);
	cr_fe_sub(&e, &b, &a);
	cr_fe_sub(&f, &d, &c);
	cr_fe_add(&g, &d, &c);
	cr_fe_add(&h, &b, &a);
	cr_fe_mul(&r->x, &e, &f);
	cr_fe_mul(&r->y, &g, &h);
	cr_fe_mul(&r->t, &e, &h);
	cr_fe_mul(&r->z, &f, &g);
}

// p = q when bit is 1, unchanged when 0, without a branch on bit.
static void cr_ge_cmov(cr_ge *p, const cr_ge *q, uint64_t bit) {
	uint64_t m = 0 - bit;
	cr_fe *pf[4] = {&p->x, &p->y, &p->z, &p->t};
	const cr_fe *qf[4] = {&q->x, &q->y, &q->z, &q->t};
	for (int i = 0; i < 4; i++) {
		for (int j = 0; j < 5; j++) {
			pf[i]->v[j] ^= m & (pf[i]->v[j] ^ qf[i]->v[j]);
		}
	}
}

// r = [k]p for the 256 bit little endian scalar k.
static void cr_ge_mul(cr_ge *r, const uint8_t k[32], const cr_ge *p) {
	cr_ge acc, sum;
	cr_ge_identity(&acc);
	for (int i = 255; i >= 0; i--) {
		cr_ge_add(&acc, &acc, &acc);
		cr_ge_add(&sum, &acc, p);
		cr_ge_cmov(&acc, &sum, (uint64_t)((k[i >> 3] >> (i & 7)) & 1));
	}
	*r = acc;
}

static void cr_ge_bytes(uint8_t s[32], const cr_ge *p) {
	cr_fe zi, x, y;
	cr_fe_inv(&zi, &p->z);
	cr_fe_mul(&x, &p->x, &zi);
	cr_fe_mul(&y, &p->y, &zi);
	cr_fe_bytes(s, &y);
	s[31] ^= (uint8_t)(cr_fe_is_odd(&x) << 7);
}

// p = the point encoded by s; 0 when s encodes no point of the curve.
static int cr_ge_from_bytes(cr_ge *p, const uint8_t s[32]) {
	cr_fe y, u, v, v3, v7, x, t, chk, one;
	cr_fe_set(&one, 1);
	cr_fe_from_bytes(&y, s);
	cr_fe_sq(&u, &y);
	cr_fe_mul(&v, &u, &cr_ed_d);
	cr_fe_sub(&u, &u, &one);
	cr_fe_add(&v, &v, &one);
	cr_fe_sq(&v3, &v);
	cr_fe_mul(&v3, &v3, &v);
	cr_fe_sq(&v7, &v3);
	cr_fe_mul(&v7, &v7, &v);
	cr_fe_mul(&t, &u, &v7);
	cr_fe_pow22523(&t, &t);
	cr_fe_mul(&x, &u, &v3);
	cr_fe_mul(&x, &x, &t);
	cr_fe_sq(&chk, &x);
	cr_fe_mul(&chk, &chk, &v);
	cr_fe_sub(&t, &chk, &u);
	if (!cr_fe_is_zero(&t)) {
		cr_fe_add(&t, &chk, &u);
		if (!cr_fe_is_zero(&t)) {
			return 0;
		}
		cr_fe_mul(&x, &x, &cr_ed_sqrtm1);
	}
	if (cr_fe_is_odd(&x) != (s[31] >> 7)) {
		cr_fe_neg(&x, &x);
	}
	p->x = x;
	p->y = y;
	cr_fe_carry(&p->y);
	cr_fe_set(&p->z, 1);
	cr_fe_mul(&p->t, &x, &p->y);
	return 1;
}

// The curve constants: d = -121665/121666, 2d, sqrt(-1) = 2^((p-1)/4),
// and the base point (y = 4/5, x even). Computed once (pthread_once:
// tasks on two workers raced on a plain flag, ThreadSanitizer on Linux,
// 2026-10-12).
static void cr_ed_init_once(void) {
	cr_fe a, b, z250, z11, t;
	cr_fe_set(&a, 121665);
	cr_fe_neg(&a, &a);
	cr_fe_set(&b, 121666);
	cr_fe_inv(&b, &b);
	cr_fe_mul(&cr_ed_d, &a, &b);
	cr_fe_add(&cr_ed_d2, &cr_ed_d, &cr_ed_d);
	cr_fe_set(&a, 2);
	cr_fe_pow250(&z250, &z11, &a);
	cr_fe_sqn(&t, &z250, 3); // 2^(2^253 - 8)
	cr_fe_sq(&b, &a);
	cr_fe_mul(&b, &b, &a); // 2^3
	cr_fe_mul(&cr_ed_sqrtm1, &t, &b);
	uint8_t s[32];
	memset(s, 0x66, 32);
	s[0] = 0x58;
	cr_ge_from_bytes(&cr_ed_base, s);
}

static void cr_ed_init(void) {
	pthread_once(&cr_ed_once, cr_ed_init_once);
}

// The group order L = 2^252 + 27742317777372353535851937790883648493 as
// 64 bit little endian limbs.
static const uint64_t cr_sc_l[4] = {0x5812631a5cf5d3edULL, 0x14def9dea2f79cd6ULL, 0, 0x1000000000000000ULL};

// out = the n little endian 64 bit limbs of x modulo L, bit by bit
// (constant time in x).
static void cr_sc_reduce(uint8_t out[32], const uint64_t *x, int n) {
	uint64_t r[5] = {0};
	for (int i = 64 * n - 1; i >= 0; i--) {
		uint64_t bit = (x[i >> 6] >> (i & 63)) & 1;
		for (int j = 4; j > 0; j--) {
			r[j] = r[j] << 1 | r[j - 1] >> 63;
		}
		r[0] = r[0] << 1 | bit;
		uint64_t t[5], borrow = 0;
		for (int j = 0; j < 5; j++) {
			uint64_t l = j < 4 ? cr_sc_l[j] : 0;
			cr_u128 d = (cr_u128)r[j] - l - borrow;
			t[j] = (uint64_t)d;
			borrow = (uint64_t)(d >> 64) & 1;
		}
		uint64_t m = borrow - 1; // all ones when r >= L
		for (int j = 0; j < 5; j++) {
			r[j] ^= m & (r[j] ^ t[j]);
		}
	}
	for (int j = 0; j < 4; j++) {
		cr_put_le64(out + 8 * j, r[j]);
	}
}

// out = the 64 byte little endian h modulo L.
static void cr_sc_reduce64(uint8_t out[32], const uint8_t h[64]) {
	uint64_t x[8];
	for (int i = 0; i < 8; i++) {
		x[i] = cr_le64(h + 8 * i);
	}
	cr_sc_reduce(out, x, 8);
}

// out = (r + k * a) modulo L for 32 byte little endian scalars.
static void cr_sc_muladd(uint8_t out[32], const uint8_t k[32], const uint8_t a[32], const uint8_t r[32]) {
	uint64_t kk[4], aa[4], x[9] = {0};
	for (int i = 0; i < 4; i++) {
		kk[i] = cr_le64(k + 8 * i);
		aa[i] = cr_le64(a + 8 * i);
	}
	for (int i = 0; i < 4; i++) {
		uint64_t carry = 0;
		for (int j = 0; j < 4; j++) {
			cr_u128 t = (cr_u128)kk[i] * aa[j] + x[i + j] + carry;
			x[i + j] = (uint64_t)t;
			carry = (uint64_t)(t >> 64);
		}
		x[i + 4] += carry;
	}
	uint64_t carry = 0;
	for (int i = 0; i < 9; i++) {
		cr_u128 t = (cr_u128)x[i] + (i < 4 ? cr_le64(r + 8 * i) : 0) + carry;
		x[i] = (uint64_t)t;
		carry = (uint64_t)(t >> 64);
	}
	cr_sc_reduce(out, x, 9);
}

// Whether the 32 byte little endian s is below L.
static int cr_sc_canonical(const uint8_t s[32]) {
	for (int i = 3; i >= 0; i--) {
		uint64_t v = cr_le64(s + 8 * i);
		if (v != cr_sc_l[i]) {
			return v < cr_sc_l[i];
		}
	}
	return 0;
}

// The secret scalar (clamped), the nonce prefix, and the public key of a
// seed.
static void cr_ed_expand(const uint8_t seed[32], uint8_t a[32], uint8_t prefix[32], uint8_t pub[32]) {
	cr_ed_init();
	uint8_t h[64];
	cr_sha512_3(seed, 32, NULL, 0, NULL, 0, h);
	memcpy(a, h, 32);
	a[0] &= 248;
	a[31] &= 127;
	a[31] |= 64;
	memcpy(prefix, h + 32, 32);
	cr_ge pa;
	cr_ge_mul(&pa, a, &cr_ed_base);
	cr_ge_bytes(pub, &pa);
}

static void cr_ed_sign(uint8_t sig[64], const uint8_t seed[32], const uint8_t *msg, int64_t mn) {
	uint8_t a[32], prefix[32], pub[32], h[64], r[32], k[32];
	cr_ed_expand(seed, a, prefix, pub);
	cr_sha512_3(prefix, 32, msg, mn, NULL, 0, h);
	cr_sc_reduce64(r, h);
	cr_ge pr;
	cr_ge_mul(&pr, r, &cr_ed_base);
	cr_ge_bytes(sig, &pr);
	cr_sha512_3(sig, 32, pub, 32, msg, mn, h);
	cr_sc_reduce64(k, h);
	cr_sc_muladd(sig + 32, k, a, r);
}

static int cr_ed_verify(const uint8_t pub[32], const uint8_t *msg, int64_t mn, const uint8_t sig[64]) {
	cr_ed_init();
	if (!cr_sc_canonical(sig + 32)) {
		return 0;
	}
	cr_ge pa, neg, sb, ka, pr;
	if (!cr_ge_from_bytes(&pa, pub)) {
		return 0;
	}
	cr_fe_neg(&neg.x, &pa.x);
	neg.y = pa.y;
	neg.z = pa.z;
	cr_fe_neg(&neg.t, &pa.t);
	uint8_t h[64], k[32], rb[32];
	cr_sha512_3(sig, 32, pub, 32, msg, mn, h);
	cr_sc_reduce64(k, h);
	cr_ge_mul(&sb, sig + 32, &cr_ed_base);
	cr_ge_mul(&ka, k, &neg);
	cr_ge_add(&pr, &sb, &ka);
	cr_ge_bytes(rb, &pr);
	return memcmp(rb, sig, 32) == 0;
}

// xo_crypto_ed25519_public_key is `crypto.ed25519_public_key(seed)`
// (std/native/crypto.xo checks that the seed is 32 bytes).
xo_rstr *xo_crypto_ed25519_public_key(const char *seed, int64_t sn, int64_t *len) {
	(void)sn;
	uint8_t a[32], prefix[32], pub[32];
	cr_ed_expand((const uint8_t *)seed, a, prefix, pub);
	return nstr((const char *)pub, 32, len);
}

// xo_crypto_ed25519_sign is `crypto.ed25519_sign(seed, msg)`.
xo_rstr *xo_crypto_ed25519_sign(const char *seed, int64_t sn, const char *m, int64_t mn, int64_t *len) {
	(void)sn;
	uint8_t sig[64];
	cr_ed_sign(sig, (const uint8_t *)seed, (const uint8_t *)m, mn);
	return nstr((const char *)sig, 64, len);
}

// xo_crypto_ed25519_verify is `crypto.ed25519_verify(public_key, msg,
// sig)`: false for a key or a signature of the wrong length.
int64_t xo_crypto_ed25519_verify(const char *pub, int64_t pn, const char *m, int64_t mn, const char *sig, int64_t gn) {
	if (pn != 32 || gn != 64) {
		return 0;
	}
	return cr_ed_verify((const uint8_t *)pub, (const uint8_t *)m, mn, (const uint8_t *)sig);
}
