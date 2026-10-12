// xo_crypto.c: std/crypto and the Rand capability on the native backends
// (decision 0083), included by xo_std.c. The results are those of std/rt's
// crypto.go (Go's crypto/sha256, crypto/hmac, crypto/pbkdf2,
// crypto/subtle, encoding/base64, and the Argon2id and BLAKE2b vendored
// from golang.org/x/crypto, decision 0059): SHA-256 (FIPS 180-4), HMAC
// (RFC 2104), PBKDF2 (RFC 8018), BLAKE2b (RFC 7693), and Argon2 (RFC 9106)
// written here from the specifications and x/crypto's portable code.
// std/llrt/testdata/crypto_diff.c checks them against Go
// (internal/llvm's TestCryptoDiff) and the RFC test vectors.
//
// Static helpers are prefixed cr_ (gotchas/llrt-static-names-in-tests.md).

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

// ---------------------------------------------------------------------------
// SHA-256

typedef struct {
	uint32_t h[8];
	uint8_t buf[64];
	uint64_t len; // bytes hashed so far
	int n;        // bytes in buf
} cr_sha256;

static const uint32_t cr_sha_k[64] = {
	0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
	0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
	0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
	0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
	0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
	0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
	0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
	0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
};

static uint32_t cr_ror32(uint32_t x, int n) { return (x >> n) | (x << (32 - n)); }

static void cr_sha_block(uint32_t h[8], const uint8_t *p) {
	uint32_t w[64];
	for (int i = 0; i < 16; i++) {
		w[i] = (uint32_t)p[4 * i] << 24 | (uint32_t)p[4 * i + 1] << 16 | (uint32_t)p[4 * i + 2] << 8 | (uint32_t)p[4 * i + 3];
	}
	for (int i = 16; i < 64; i++) {
		uint32_t s0 = cr_ror32(w[i - 15], 7) ^ cr_ror32(w[i - 15], 18) ^ (w[i - 15] >> 3);
		uint32_t s1 = cr_ror32(w[i - 2], 17) ^ cr_ror32(w[i - 2], 19) ^ (w[i - 2] >> 10);
		w[i] = w[i - 16] + s0 + w[i - 7] + s1;
	}
	uint32_t a = h[0], b = h[1], c = h[2], d = h[3], e = h[4], f = h[5], g = h[6], hh = h[7];
	for (int i = 0; i < 64; i++) {
		uint32_t t1 = hh + (cr_ror32(e, 6) ^ cr_ror32(e, 11) ^ cr_ror32(e, 25)) + ((e & f) ^ (~e & g)) + cr_sha_k[i] + w[i];
		uint32_t t2 = (cr_ror32(a, 2) ^ cr_ror32(a, 13) ^ cr_ror32(a, 22)) + ((a & b) ^ (a & c) ^ (b & c));
		hh = g;
		g = f;
		f = e;
		e = d + t1;
		d = c;
		c = b;
		b = a;
		a = t1 + t2;
	}
	h[0] += a;
	h[1] += b;
	h[2] += c;
	h[3] += d;
	h[4] += e;
	h[5] += f;
	h[6] += g;
	h[7] += hh;
}

static void cr_sha_init(cr_sha256 *s) {
	static const uint32_t iv[8] = {0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19};
	memcpy(s->h, iv, sizeof iv);
	s->len = 0;
	s->n = 0;
}

static void cr_sha_write(cr_sha256 *s, const uint8_t *p, int64_t n) {
	s->len += (uint64_t)n;
	if (s->n > 0) {
		int64_t k = 64 - s->n < n ? 64 - s->n : n;
		memcpy(s->buf + s->n, p, (size_t)k);
		s->n += (int)k;
		p += k;
		n -= k;
		if (s->n == 64) {
			cr_sha_block(s->h, s->buf);
			s->n = 0;
		}
	}
	while (n >= 64) {
		cr_sha_block(s->h, p);
		p += 64;
		n -= 64;
	}
	if (n > 0) {
		memcpy(s->buf, p, (size_t)n);
		s->n = (int)n;
	}
}

static void cr_sha_sum(const cr_sha256 *s0, uint8_t out[32]) {
	cr_sha256 s = *s0;
	uint64_t bits = s.len * 8;
	uint8_t pad[72] = {0x80};
	int64_t padn = (s.n < 56 ? 56 - s.n : 120 - s.n);
	cr_sha_write(&s, pad, padn);
	uint8_t lb[8];
	for (int i = 0; i < 8; i++) {
		lb[i] = (uint8_t)(bits >> (56 - 8 * i));
	}
	cr_sha_write(&s, lb, 8);
	for (int i = 0; i < 8; i++) {
		out[4 * i] = (uint8_t)(s.h[i] >> 24);
		out[4 * i + 1] = (uint8_t)(s.h[i] >> 16);
		out[4 * i + 2] = (uint8_t)(s.h[i] >> 8);
		out[4 * i + 3] = (uint8_t)s.h[i];
	}
}

// ---------------------------------------------------------------------------
// HMAC-SHA256 and PBKDF2-HMAC-SHA256

typedef struct {
	cr_sha256 in, out; // states after the inner and outer padded key
} cr_hmac;

static void cr_hmac_init(cr_hmac *m, const uint8_t *key, int64_t kn) {
	uint8_t k[64] = {0};
	if (kn > 64) {
		cr_sha256 s;
		cr_sha_init(&s);
		cr_sha_write(&s, key, kn);
		cr_sha_sum(&s, k);
	} else if (kn > 0) {
		memcpy(k, key, (size_t)kn);
	}
	uint8_t pad[64];
	for (int i = 0; i < 64; i++) {
		pad[i] = k[i] ^ 0x36;
	}
	cr_sha_init(&m->in);
	cr_sha_write(&m->in, pad, 64);
	for (int i = 0; i < 64; i++) {
		pad[i] = k[i] ^ 0x5c;
	}
	cr_sha_init(&m->out);
	cr_sha_write(&m->out, pad, 64);
}

// cr_hmac_sum is the MAC of the n bytes at p under the key of m (m is
// not changed, so it serves many messages).
static void cr_hmac_sum(const cr_hmac *m, const uint8_t *p, int64_t n, const uint8_t *p2, int64_t n2, uint8_t out[32]) {
	cr_sha256 s = m->in;
	cr_sha_write(&s, p, n);
	if (n2 > 0) {
		cr_sha_write(&s, p2, n2);
	}
	uint8_t inner[32];
	cr_sha_sum(&s, inner);
	s = m->out;
	cr_sha_write(&s, inner, 32);
	cr_sha_sum(&s, out);
}

// cr_pbkdf2 is PBKDF2-HMAC-SHA256 (crypto/pbkdf2.Key).
static void cr_pbkdf2(const uint8_t *pw, int64_t pn, const uint8_t *salt, int64_t sn, int64_t iter, uint8_t *out, int64_t keylen) {
	cr_hmac m;
	cr_hmac_init(&m, pw, pn);
	for (uint32_t block = 1; keylen > 0; block++) {
		uint8_t ib[4] = {(uint8_t)(block >> 24), (uint8_t)(block >> 16), (uint8_t)(block >> 8), (uint8_t)block};
		uint8_t u[32], t[32];
		cr_hmac_sum(&m, salt, sn, ib, 4, u);
		memcpy(t, u, 32);
		for (int64_t i = 1; i < iter; i++) {
			cr_hmac_sum(&m, u, 32, NULL, 0, u);
			for (int j = 0; j < 32; j++) {
				t[j] ^= u[j];
			}
		}
		int64_t k = keylen < 32 ? keylen : 32;
		memcpy(out, t, (size_t)k);
		out += k;
		keylen -= k;
	}
}

// ---------------------------------------------------------------------------
// BLAKE2b (RFC 7693), unkeyed, digest sizes 1 to 64

typedef struct {
	uint64_t h[8];
	uint64_t t[2];
	uint8_t buf[128];
	int n;
	int size;
} cr_b2b;

static const uint64_t cr_b2b_iv[8] = {
	0x6a09e667f3bcc908ULL, 0xbb67ae8584caa73bULL, 0x3c6ef372fe94f82bULL, 0xa54ff53a5f1d36f1ULL,
	0x510e527fade682d1ULL, 0x9b05688c2b3e6c1fULL, 0x1f83d9abfb41bd6bULL, 0x5be0cd19137e2179ULL,
};

static const uint8_t cr_b2b_sigma[12][16] = {
	{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
	{14, 10, 4, 8, 9, 15, 13, 6, 1, 12, 0, 2, 11, 7, 5, 3},
	{11, 8, 12, 0, 5, 2, 15, 13, 10, 14, 3, 6, 7, 1, 9, 4},
	{7, 9, 3, 1, 13, 12, 11, 14, 2, 6, 5, 10, 4, 0, 15, 8},
	{9, 0, 5, 7, 2, 4, 10, 15, 14, 1, 11, 12, 6, 8, 3, 13},
	{2, 12, 6, 10, 0, 11, 8, 3, 4, 13, 7, 5, 15, 14, 1, 9},
	{12, 5, 1, 15, 14, 13, 4, 10, 0, 7, 6, 3, 9, 2, 8, 11},
	{13, 11, 7, 14, 12, 1, 3, 9, 5, 0, 15, 4, 8, 6, 2, 10},
	{6, 15, 14, 9, 11, 3, 0, 8, 12, 2, 13, 7, 1, 4, 10, 5},
	{10, 2, 8, 4, 7, 6, 1, 5, 15, 11, 9, 14, 3, 12, 13, 0},
	{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
	{14, 10, 4, 8, 9, 15, 13, 6, 1, 12, 0, 2, 11, 7, 5, 3},
};

static uint64_t cr_ror64(uint64_t x, int n) { return (x >> n) | (x << (64 - n)); }

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

static void cr_put_le32(uint8_t *p, uint32_t v) {
	for (int i = 0; i < 4; i++) {
		p[i] = (uint8_t)(v >> (8 * i));
	}
}

static void cr_b2b_compress(cr_b2b *s, const uint8_t *blk, int last) {
	uint64_t m[16], v[16];
	for (int i = 0; i < 16; i++) {
		m[i] = cr_le64(blk + 8 * i);
	}
	for (int i = 0; i < 8; i++) {
		v[i] = s->h[i];
		v[i + 8] = cr_b2b_iv[i];
	}
	v[12] ^= s->t[0];
	v[13] ^= s->t[1];
	if (last) {
		v[14] = ~v[14];
	}
#define CR_G(a, b, c, d, x, y)                     \
	do {                                           \
		v[a] = v[a] + v[b] + (x);                  \
		v[d] = cr_ror64(v[d] ^ v[a], 32);          \
		v[c] = v[c] + v[d];                        \
		v[b] = cr_ror64(v[b] ^ v[c], 24);          \
		v[a] = v[a] + v[b] + (y);                  \
		v[d] = cr_ror64(v[d] ^ v[a], 16);          \
		v[c] = v[c] + v[d];                        \
		v[b] = cr_ror64(v[b] ^ v[c], 63);          \
	} while (0)
	for (int r = 0; r < 12; r++) {
		const uint8_t *sg = cr_b2b_sigma[r];
		CR_G(0, 4, 8, 12, m[sg[0]], m[sg[1]]);
		CR_G(1, 5, 9, 13, m[sg[2]], m[sg[3]]);
		CR_G(2, 6, 10, 14, m[sg[4]], m[sg[5]]);
		CR_G(3, 7, 11, 15, m[sg[6]], m[sg[7]]);
		CR_G(0, 5, 10, 15, m[sg[8]], m[sg[9]]);
		CR_G(1, 6, 11, 12, m[sg[10]], m[sg[11]]);
		CR_G(2, 7, 8, 13, m[sg[12]], m[sg[13]]);
		CR_G(3, 4, 9, 14, m[sg[14]], m[sg[15]]);
	}
#undef CR_G
	for (int i = 0; i < 8; i++) {
		s->h[i] ^= v[i] ^ v[i + 8];
	}
}

static void cr_b2b_init(cr_b2b *s, int size) {
	memcpy(s->h, cr_b2b_iv, sizeof cr_b2b_iv);
	s->h[0] ^= 0x01010000ULL ^ (uint64_t)size;
	s->t[0] = s->t[1] = 0;
	s->n = 0;
	s->size = size;
}

static void cr_b2b_count(cr_b2b *s, uint64_t n) {
	s->t[0] += n;
	if (s->t[0] < n) {
		s->t[1]++;
	}
}

static void cr_b2b_write(cr_b2b *s, const uint8_t *p, int64_t n) {
	while (n > 0) {
		if (s->n == 128) {
			// a full block is compressed only when more data follows (the
			// last block gets the final flag)
			cr_b2b_count(s, 128);
			cr_b2b_compress(s, s->buf, 0);
			s->n = 0;
		}
		int64_t k = 128 - s->n < n ? 128 - s->n : n;
		memcpy(s->buf + s->n, p, (size_t)k);
		s->n += (int)k;
		p += k;
		n -= k;
	}
}

static void cr_b2b_sum(cr_b2b *s, uint8_t *out) {
	cr_b2b_count(s, (uint64_t)s->n);
	memset(s->buf + s->n, 0, (size_t)(128 - s->n));
	cr_b2b_compress(s, s->buf, 1);
	uint8_t full[64];
	for (int i = 0; i < 8; i++) {
		cr_put_le64(full + 8 * i, s->h[i]);
	}
	memcpy(out, full, (size_t)s->size);
}

// ---------------------------------------------------------------------------
// Argon2 (RFC 9106), as x/crypto's argon2 package computes it

enum { CR_ARGON2D = 0, CR_ARGON2I = 1, CR_ARGON2ID = 2 };

typedef struct {
	uint64_t v[128];
} cr_ablock;

// cr_argon2_hash is H' of RFC 9106 3.3: out (outn bytes) from in.
static void cr_argon2_hash(uint8_t *out, int64_t outn, const uint8_t *in, int64_t inn) {
	cr_b2b b;
	uint8_t lb[4], buf[64];
	cr_put_le32(lb, (uint32_t)outn);
	if (outn <= 64) {
		cr_b2b_init(&b, (int)outn);
		cr_b2b_write(&b, lb, 4);
		cr_b2b_write(&b, in, inn);
		cr_b2b_sum(&b, out);
		return;
	}
	cr_b2b_init(&b, 64);
	cr_b2b_write(&b, lb, 4);
	cr_b2b_write(&b, in, inn);
	cr_b2b_sum(&b, buf);
	memcpy(out, buf, 32);
	out += 32;
	outn -= 32;
	while (outn > 64) {
		cr_b2b_init(&b, 64);
		cr_b2b_write(&b, buf, 64);
		cr_b2b_sum(&b, buf);
		memcpy(out, buf, 32);
		out += 32;
		outn -= 32;
	}
	cr_b2b_init(&b, (int)outn);
	cr_b2b_write(&b, buf, 64);
	cr_b2b_sum(&b, out);
}

static void cr_blamka(uint64_t *t, int a, int b, int c, int d) {
#define CR_FBM(x, y) ((x) + (y) + 2 * (uint64_t)(uint32_t)(x) * (uint64_t)(uint32_t)(y))
	t[a] = CR_FBM(t[a], t[b]);
	t[d] = cr_ror64(t[d] ^ t[a], 32);
	t[c] = CR_FBM(t[c], t[d]);
	t[b] = cr_ror64(t[b] ^ t[c], 24);
	t[a] = CR_FBM(t[a], t[b]);
	t[d] = cr_ror64(t[d] ^ t[a], 16);
	t[c] = CR_FBM(t[c], t[d]);
	t[b] = cr_ror64(t[b] ^ t[c], 63);
#undef CR_FBM
}

// cr_round is BLAKE2b's round without the message on 16 words given by
// their indexes in t (x/crypto's blamka).
static void cr_round(uint64_t *t, const int ix[16]) {
	uint64_t v[16];
	for (int i = 0; i < 16; i++) {
		v[i] = t[ix[i]];
	}
	cr_blamka(v, 0, 4, 8, 12);
	cr_blamka(v, 1, 5, 9, 13);
	cr_blamka(v, 2, 6, 10, 14);
	cr_blamka(v, 3, 7, 11, 15);
	cr_blamka(v, 0, 5, 10, 15);
	cr_blamka(v, 1, 6, 11, 12);
	cr_blamka(v, 2, 7, 8, 13);
	cr_blamka(v, 3, 4, 9, 14);
	for (int i = 0; i < 16; i++) {
		t[ix[i]] = v[i];
	}
}

// cr_process_block is G of RFC 9106 3.5: out = (or ^=, when xor is set)
// in1 ^ in2 ^ P(in1 ^ in2).
static void cr_process_block(cr_ablock *out, const cr_ablock *in1, const cr_ablock *in2, int xor) {
	cr_ablock t;
	for (int i = 0; i < 128; i++) {
		t.v[i] = in1->v[i] ^ in2->v[i];
	}
	int ix[16];
	for (int i = 0; i < 128; i += 16) {
		for (int j = 0; j < 16; j++) {
			ix[j] = i + j;
		}
		cr_round(t.v, ix);
	}
	for (int i = 0; i < 16; i += 2) {
		for (int j = 0; j < 8; j++) {
			ix[2 * j] = 16 * j + i;
			ix[2 * j + 1] = 16 * j + i + 1;
		}
		cr_round(t.v, ix);
	}
	if (xor) {
		for (int i = 0; i < 128; i++) {
			out->v[i] ^= in1->v[i] ^ in2->v[i] ^ t.v[i];
		}
	} else {
		for (int i = 0; i < 128; i++) {
			out->v[i] = in1->v[i] ^ in2->v[i] ^ t.v[i];
		}
	}
}

static uint32_t cr_phi(uint64_t rnd, uint64_t m, uint64_t s, uint32_t lane, uint32_t lanes) {
	uint64_t p = rnd & 0xFFFFFFFFULL;
	p = (p * p) >> 32;
	p = (p * m) >> 32;
	return lane * lanes + (uint32_t)((s + m - (p + 1)) % (uint64_t)lanes);
}

static uint32_t cr_index_alpha(uint64_t rnd, uint32_t lanes, uint32_t segments, uint32_t threads, uint32_t n, uint32_t slice, uint32_t lane, uint32_t index) {
	uint32_t ref = (uint32_t)(rnd >> 32) % threads;
	if (n == 0 && slice == 0) {
		ref = lane;
	}
	uint32_t m = 3 * segments, s = ((slice + 1) % 4) * segments;
	if (lane == ref) {
		m += index;
	}
	if (n == 0) {
		m = slice * segments;
		s = 0;
		if (slice == 0 || lane == ref) {
			m += index;
		}
	}
	if (index == 0 || lane == ref) {
		m--;
	}
	return cr_phi(rnd, m, s, ref, lanes);
}

// cr_argon2 derives keylen bytes into out (x/crypto's deriveKey). The
// lanes of one slice are independent, so they run one after another
// here (x/crypto runs them on goroutines) with the same result.
static void cr_argon2(int mode, const uint8_t *pw, int64_t pn, const uint8_t *salt, int64_t sn, const uint8_t *secret, int64_t secn, const uint8_t *ad, int64_t adn,
	uint32_t time, uint32_t memory, uint32_t threads, uint8_t *out, uint32_t keylen) {
	uint8_t h0[72], tmp[4], params[24];
	cr_b2b b;
	cr_b2b_init(&b, 64);
	cr_put_le32(params, threads);
	cr_put_le32(params + 4, keylen);
	cr_put_le32(params + 8, memory);
	cr_put_le32(params + 12, time);
	cr_put_le32(params + 16, 0x13);
	cr_put_le32(params + 20, (uint32_t)mode);
	cr_b2b_write(&b, params, 24);
	cr_put_le32(tmp, (uint32_t)pn);
	cr_b2b_write(&b, tmp, 4);
	cr_b2b_write(&b, pw, pn);
	cr_put_le32(tmp, (uint32_t)sn);
	cr_b2b_write(&b, tmp, 4);
	cr_b2b_write(&b, salt, sn);
	cr_put_le32(tmp, (uint32_t)secn);
	cr_b2b_write(&b, tmp, 4);
	cr_b2b_write(&b, secret, secn);
	cr_put_le32(tmp, (uint32_t)adn);
	cr_b2b_write(&b, tmp, 4);
	cr_b2b_write(&b, ad, adn);
	cr_b2b_sum(&b, h0);

	memory = memory / (4 * threads) * (4 * threads);
	if (memory < 2 * 4 * threads) {
		memory = 2 * 4 * threads;
	}
	cr_ablock *B = calloc(memory, sizeof(cr_ablock));
	if (!B) {
		xo_oom();
	}
	uint32_t lanes = memory / threads, segments = lanes / 4;
	uint8_t block0[1024];
	for (uint32_t lane = 0; lane < threads; lane++) {
		uint32_t j = lane * lanes;
		cr_put_le32(h0 + 68, lane);
		for (uint32_t k = 0; k < 2; k++) {
			cr_put_le32(h0 + 64, k);
			cr_argon2_hash(block0, 1024, h0, 72);
			for (int i = 0; i < 128; i++) {
				B[j + k].v[i] = cr_le64(block0 + 8 * i);
			}
		}
	}
	for (uint32_t n = 0; n < time; n++) {
		for (uint32_t slice = 0; slice < 4; slice++) {
			for (uint32_t lane = 0; lane < threads; lane++) {
				cr_ablock addresses, in, zero;
				memset(&addresses, 0, sizeof addresses);
				memset(&in, 0, sizeof in);
				memset(&zero, 0, sizeof zero);
				int indep = mode == CR_ARGON2I || (mode == CR_ARGON2ID && n == 0 && slice < 2);
				if (indep) {
					in.v[0] = n;
					in.v[1] = lane;
					in.v[2] = slice;
					in.v[3] = memory;
					in.v[4] = time;
					in.v[5] = (uint64_t)mode;
				}
				uint32_t index = 0;
				if (n == 0 && slice == 0) {
					index = 2;
					if (mode == CR_ARGON2I || mode == CR_ARGON2ID) {
						in.v[6]++;
						cr_process_block(&addresses, &in, &zero, 0);
						cr_process_block(&addresses, &addresses, &zero, 0);
					}
				}
				uint32_t offset = lane * lanes + slice * segments + index;
				while (index < segments) {
					uint32_t prev = offset - 1;
					if (index == 0 && slice == 0) {
						prev += lanes;
					}
					uint64_t rnd;
					if (indep) {
						if (index % 128 == 0) {
							in.v[6]++;
							cr_process_block(&addresses, &in, &zero, 0);
							cr_process_block(&addresses, &addresses, &zero, 0);
						}
						rnd = addresses.v[index % 128];
					} else {
						rnd = B[prev].v[0];
					}
					uint32_t ref = cr_index_alpha(rnd, lanes, segments, threads, n, slice, lane, index);
					cr_process_block(&B[offset], &B[prev], &B[ref], 1);
					index++;
					offset++;
				}
			}
		}
	}
	for (uint32_t lane = 0; lane + 1 < threads; lane++) {
		for (int i = 0; i < 128; i++) {
			B[memory - 1].v[i] ^= B[lane * lanes + lanes - 1].v[i];
		}
	}
	for (int i = 0; i < 128; i++) {
		cr_put_le64(block0 + 8 * i, B[memory - 1].v[i]);
	}
	free(B);
	cr_argon2_hash(out, keylen, block0, 1024);
}

// ---------------------------------------------------------------------------
// Base64 without padding (encoding/base64's RawStdEncoding and
// RawURLEncoding)

static const char cr_b64_url[] = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_";

static void cr_b64_raw(nbuf *b, const uint8_t *p, int64_t n, const char *alpha) {
	for (int64_t i = 0; i < n; i += 3) {
		uint32_t v = (uint32_t)p[i] << 16;
		if (i + 1 < n) {
			v |= (uint32_t)p[i + 1] << 8;
		}
		if (i + 2 < n) {
			v |= p[i + 2];
		}
		nbuf_byte(b, alpha[v >> 18 & 63]);
		nbuf_byte(b, alpha[v >> 12 & 63]);
		if (i + 1 < n) {
			nbuf_byte(b, alpha[v >> 6 & 63]);
		}
		if (i + 2 < n) {
			nbuf_byte(b, alpha[v & 63]);
		}
	}
}

// cr_b64_raw_decode is RawStdEncoding.DecodeString: '\r' and '\n' are
// skipped, '=' and other bytes are errors, a final group of one symbol is
// an error, and the unused low bits of a final group are ignored.
static int cr_b64_raw_decode(const char *p, int64_t n, nbuf *out) {
	uint32_t acc = 0;
	int k = 0;
	for (int64_t i = 0; i < n; i++) {
		unsigned char c = (unsigned char)p[i];
		if (c == '\r' || c == '\n') {
			continue;
		}
		int v = c == '=' ? -1 : b64_val(c);
		if (v < 0) {
			return 0;
		}
		acc = acc << 6 | (uint32_t)v;
		if (++k == 4) {
			nbuf_byte(out, (char)(acc >> 16));
			nbuf_byte(out, (char)(acc >> 8));
			nbuf_byte(out, (char)acc);
			acc = 0;
			k = 0;
		}
	}
	if (k == 1) {
		return 0;
	}
	if (k == 2) {
		nbuf_byte(out, (char)(acc >> 4));
	} else if (k == 3) {
		nbuf_byte(out, (char)(acc >> 10));
		nbuf_byte(out, (char)(acc >> 2));
	}
	return 1;
}

// cr_parse_int is strconv.ParseInt(s, 10, 64): an optional sign and at
// least one decimal digit, no other bytes, within int64.
static int cr_parse_int(const char *p, int64_t n, int64_t *v) {
	int64_t i = 0;
	int neg = 0;
	if (n > 0 && (p[0] == '+' || p[0] == '-')) {
		neg = p[0] == '-';
		i = 1;
	}
	if (i == n) {
		return 0;
	}
	uint64_t x = 0, lim = neg ? (uint64_t)1 << 63 : ((uint64_t)1 << 63) - 1;
	for (; i < n; i++) {
		if (p[i] < '0' || p[i] > '9') {
			return 0;
		}
		uint64_t d = (uint64_t)(p[i] - '0');
		if (x > (lim - d) / 10) {
			return 0;
		}
		x = x * 10 + d;
	}
	*v = neg ? (int64_t)(0 - x) : (int64_t)x;
	return 1;
}

// ---------------------------------------------------------------------------
// Intrinsics (std/native/rtcrypto.xo)

// xo_crypto_sha256 is `crypto.sha256(data)`.
xo_rstr *xo_crypto_sha256(const char *p, int64_t n, int64_t *len) {
	cr_sha256 s;
	cr_sha_init(&s);
	cr_sha_write(&s, (const uint8_t *)p, n);
	uint8_t d[32];
	cr_sha_sum(&s, d);
	return nstr((const char *)d, 32, len);
}

// xo_crypto_hmac_sha256 is `crypto.hmac_sha256(key, msg)`.
xo_rstr *xo_crypto_hmac_sha256(const char *k, int64_t kn, const char *p, int64_t n, int64_t *len) {
	cr_hmac m;
	cr_hmac_init(&m, (const uint8_t *)k, kn);
	uint8_t d[32];
	cr_hmac_sum(&m, (const uint8_t *)p, n, NULL, 0, d);
	return nstr((const char *)d, 32, len);
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

// xo_crypto_pbkdf2 is PBKDF2-HMAC-SHA256 of the password and salt.
xo_rstr *xo_crypto_pbkdf2(const char *pw, int64_t pn, const char *salt, int64_t sn, int64_t iter, int64_t keylen, int64_t *len) {
	xo_rstr *s = rs_new(keylen);
	cr_pbkdf2((const uint8_t *)pw, pn, (const uint8_t *)salt, sn, iter, (uint8_t *)s->data, keylen);
	*len = keylen;
	return s;
}

// xo_crypto_argon2id is Argon2id of the password and salt (no secret, no
// associated data), as std/rt's argon2Key(argon2ModeID, ...).
xo_rstr *xo_crypto_argon2id(const char *pw, int64_t pn, const char *salt, int64_t sn, int64_t t, int64_t m, int64_t p, int64_t keylen, int64_t *len) {
	xo_rstr *s = rs_new(keylen);
	cr_argon2(CR_ARGON2ID, (const uint8_t *)pw, pn, (const uint8_t *)salt, sn, NULL, 0, NULL, 0, (uint32_t)t, (uint32_t)m, (uint32_t)p, (uint8_t *)s->data, (uint32_t)keylen);
	*len = keylen;
	return s;
}

// xo_crypto_b64 is the unpadded base64 of the bytes: URL safe when url
// is set (RawURLEncoding), else standard (RawStdEncoding).
xo_rstr *xo_crypto_b64(const char *p, int64_t n, int64_t url, int64_t *len) {
	nbuf b = {0};
	cr_b64_raw(&b, (const uint8_t *)p, n, url ? cr_b64_url : b64_alpha);
	return nbuf_take(&b, len);
}

// cr_split_dollar splits s at every '$' into at most max fields (the
// count of all fields is returned; strings.Split).
static int cr_split_dollar(const char *s, int64_t n, const char **f, int64_t *fn, int max) {
	int k = 0;
	int64_t start = 0;
	for (int64_t i = 0; i <= n; i++) {
		if (i == n || s[i] == '$') {
			if (k < max) {
				f[k] = s + start;
				fn[k] = i - start;
			}
			k++;
			start = i + 1;
		}
	}
	return k;
}

static int cr_eq(const char *p, int64_t n, const char *lit) { return (int64_t)strlen(lit) == n && memcmp(p, lit, (size_t)n) == 0; }

static int cr_argon2id_params_ok(int64_t m, int64_t t, int64_t p) {
	return p >= 1 && p <= 255 && m >= 8 * p && m <= 4 * 1024 * 1024 && t >= 1 && t <= 1000;
}

// cr_verify_argon2id is std/rt's verifyArgon2id.
static int cr_verify_argon2id(const char *pw, int64_t pn, const char *h, int64_t hn) {
	const char *f[6];
	int64_t fl[6];
	if (cr_split_dollar(h, hn, f, fl, 6) != 6 || fl[0] != 0 || !cr_eq(f[1], fl[1], "argon2id") || !cr_eq(f[2], fl[2], "v=19")) {
		return 0;
	}
	// "m=..,t=..,p=.." split at ','
	const char *ps[3];
	int64_t pl[3];
	int k = 0;
	int64_t start = 0;
	for (int64_t i = 0; i <= fl[3]; i++) {
		if (i == fl[3] || f[3][i] == ',') {
			if (k < 3) {
				ps[k] = f[3] + start;
				pl[k] = i - start;
			}
			k++;
			start = i + 1;
		}
	}
	if (k != 3) {
		return 0;
	}
	int64_t vals[3];
	const char *names[3] = {"m=", "t=", "p="};
	for (int i = 0; i < 3; i++) {
		if (pl[i] < 2 || memcmp(ps[i], names[i], 2) != 0 || !cr_parse_int(ps[i] + 2, pl[i] - 2, &vals[i])) {
			return 0;
		}
	}
	if (!cr_argon2id_params_ok(vals[0], vals[1], vals[2])) {
		return 0;
	}
	nbuf salt = {0}, want = {0};
	int ok = cr_b64_raw_decode(f[4], fl[4], &salt) && salt.n >= 8 && cr_b64_raw_decode(f[5], fl[5], &want) && want.n >= 4 && want.n <= 1024;
	if (ok) {
		uint8_t *got = malloc((size_t)want.n);
		if (!got) {
			xo_oom();
		}
		cr_argon2(CR_ARGON2ID, (const uint8_t *)pw, pn, (const uint8_t *)salt.p, salt.n, NULL, 0, NULL, 0, (uint32_t)vals[1], (uint32_t)vals[0], (uint32_t)vals[2], got, (uint32_t)want.n);
		ok = (int)xo_crypto_equal((const char *)got, want.n, want.p, want.n);
		free(got);
	}
	free(salt.p);
	free(want.p);
	return ok;
}

// xo_crypto_verify_password is `crypto.verify_password(password, hash)`
// (std/rt's Crypto_verify_password): Argon2id PHC strings by their
// prefix, else pbkdf2-sha256$<iterations>$<salt>$<key>; a malformed hash
// is false.
int64_t xo_crypto_verify_password(const char *pw, int64_t pn, const char *h, int64_t hn) {
	if (hn >= 10 && memcmp(h, "$argon2id$", 10) == 0) {
		return cr_verify_argon2id(pw, pn, h, hn);
	}
	const char *f[4];
	int64_t fl[4];
	if (cr_split_dollar(h, hn, f, fl, 4) != 4 || !cr_eq(f[0], fl[0], "pbkdf2-sha256")) {
		return 0;
	}
	int64_t iter;
	if (!cr_parse_int(f[1], fl[1], &iter) || iter < 1 || iter > 100000000) {
		return 0;
	}
	nbuf salt = {0}, want = {0};
	int ok = cr_b64_raw_decode(f[2], fl[2], &salt) && cr_b64_raw_decode(f[3], fl[3], &want) && want.n > 0;
	if (ok) {
		uint8_t *got = malloc((size_t)want.n);
		if (!got) {
			xo_oom();
		}
		cr_pbkdf2((const uint8_t *)pw, pn, (const uint8_t *)salt.p, salt.n, iter, got, want.n);
		ok = (int)xo_crypto_equal((const char *)got, want.n, want.p, want.n);
		free(got);
	}
	free(salt.p);
	free(want.p);
	return ok;
}

// ---------------------------------------------------------------------------
// SHA-512 (FIPS 180-4) and Ed25519 (RFC 8032, decision 0189), as Go's
// crypto/ed25519: a 32 byte seed is the private key; verification
// rejects a non canonical S, accepts the encodings of A that Go's
// edwards25519 accepts (y not reduced, x = 0 with the sign bit), and
// compares the encoding of [S]B - [k]A with R byte for byte. Field
// elements are five 51 bit limbs modulo p = 2^255 - 19; points are
// extended twisted Edwards coordinates (X:Y:Z:T) with the complete
// addition law (it doubles too); scalar multiplication is a double and
// always add ladder with masked selection (constant time in the scalar).

typedef struct {
	uint64_t h[8];
	uint8_t buf[128];
	uint64_t len; // bytes hashed so far
	int n;        // bytes in buf
} cr_sha512;

static const uint64_t cr_sha512_k[80] = {
	0x428A2F98D728AE22ULL, 0x7137449123EF65CDULL, 0xB5C0FBCFEC4D3B2FULL, 0xE9B5DBA58189DBBCULL,
	0x3956C25BF348B538ULL, 0x59F111F1B605D019ULL, 0x923F82A4AF194F9BULL, 0xAB1C5ED5DA6D8118ULL,
	0xD807AA98A3030242ULL, 0x12835B0145706FBEULL, 0x243185BE4EE4B28CULL, 0x550C7DC3D5FFB4E2ULL,
	0x72BE5D74F27B896FULL, 0x80DEB1FE3B1696B1ULL, 0x9BDC06A725C71235ULL, 0xC19BF174CF692694ULL,
	0xE49B69C19EF14AD2ULL, 0xEFBE4786384F25E3ULL, 0x0FC19DC68B8CD5B5ULL, 0x240CA1CC77AC9C65ULL,
	0x2DE92C6F592B0275ULL, 0x4A7484AA6EA6E483ULL, 0x5CB0A9DCBD41FBD4ULL, 0x76F988DA831153B5ULL,
	0x983E5152EE66DFABULL, 0xA831C66D2DB43210ULL, 0xB00327C898FB213FULL, 0xBF597FC7BEEF0EE4ULL,
	0xC6E00BF33DA88FC2ULL, 0xD5A79147930AA725ULL, 0x06CA6351E003826FULL, 0x142929670A0E6E70ULL,
	0x27B70A8546D22FFCULL, 0x2E1B21385C26C926ULL, 0x4D2C6DFC5AC42AEDULL, 0x53380D139D95B3DFULL,
	0x650A73548BAF63DEULL, 0x766A0ABB3C77B2A8ULL, 0x81C2C92E47EDAEE6ULL, 0x92722C851482353BULL,
	0xA2BFE8A14CF10364ULL, 0xA81A664BBC423001ULL, 0xC24B8B70D0F89791ULL, 0xC76C51A30654BE30ULL,
	0xD192E819D6EF5218ULL, 0xD69906245565A910ULL, 0xF40E35855771202AULL, 0x106AA07032BBD1B8ULL,
	0x19A4C116B8D2D0C8ULL, 0x1E376C085141AB53ULL, 0x2748774CDF8EEB99ULL, 0x34B0BCB5E19B48A8ULL,
	0x391C0CB3C5C95A63ULL, 0x4ED8AA4AE3418ACBULL, 0x5B9CCA4F7763E373ULL, 0x682E6FF3D6B2B8A3ULL,
	0x748F82EE5DEFB2FCULL, 0x78A5636F43172F60ULL, 0x84C87814A1F0AB72ULL, 0x8CC702081A6439ECULL,
	0x90BEFFFA23631E28ULL, 0xA4506CEBDE82BDE9ULL, 0xBEF9A3F7B2C67915ULL, 0xC67178F2E372532BULL,
	0xCA273ECEEA26619CULL, 0xD186B8C721C0C207ULL, 0xEADA7DD6CDE0EB1EULL, 0xF57D4F7FEE6ED178ULL,
	0x06F067AA72176FBAULL, 0x0A637DC5A2C898A6ULL, 0x113F9804BEF90DAEULL, 0x1B710B35131C471BULL,
	0x28DB77F523047D84ULL, 0x32CAAB7B40C72493ULL, 0x3C9EBE0A15C9BEBCULL, 0x431D67C49C100D4CULL,
	0x4CC5D4BECB3E42B6ULL, 0x597F299CFC657E2AULL, 0x5FCB6FAB3AD6FAECULL, 0x6C44198C4A475817ULL,
};

static uint64_t cr_be64(const uint8_t *p) {
	uint64_t v = 0;
	for (int i = 0; i < 8; i++) {
		v = v << 8 | p[i];
	}
	return v;
}

static void cr_sha512_block(uint64_t h[8], const uint8_t *p) {
	uint64_t w[80];
	for (int i = 0; i < 16; i++) {
		w[i] = cr_be64(p + 8 * i);
	}
	for (int i = 16; i < 80; i++) {
		uint64_t s0 = cr_ror64(w[i - 15], 1) ^ cr_ror64(w[i - 15], 8) ^ (w[i - 15] >> 7);
		uint64_t s1 = cr_ror64(w[i - 2], 19) ^ cr_ror64(w[i - 2], 61) ^ (w[i - 2] >> 6);
		w[i] = w[i - 16] + s0 + w[i - 7] + s1;
	}
	uint64_t a = h[0], b = h[1], c = h[2], d = h[3], e = h[4], f = h[5], g = h[6], hh = h[7];
	for (int i = 0; i < 80; i++) {
		uint64_t s1 = cr_ror64(e, 14) ^ cr_ror64(e, 18) ^ cr_ror64(e, 41);
		uint64_t ch = (e & f) ^ (~e & g);
		uint64_t t1 = hh + s1 + ch + cr_sha512_k[i] + w[i];
		uint64_t s0 = cr_ror64(a, 28) ^ cr_ror64(a, 34) ^ cr_ror64(a, 39);
		uint64_t maj = (a & b) ^ (a & c) ^ (b & c);
		uint64_t t2 = s0 + maj;
		hh = g;
		g = f;
		f = e;
		e = d + t1;
		d = c;
		c = b;
		b = a;
		a = t1 + t2;
	}
	h[0] += a;
	h[1] += b;
	h[2] += c;
	h[3] += d;
	h[4] += e;
	h[5] += f;
	h[6] += g;
	h[7] += hh;
}

static void cr_sha512_init(cr_sha512 *s) {
	static const uint64_t iv[8] = {
		0x6A09E667F3BCC908ULL, 0xBB67AE8584CAA73BULL, 0x3C6EF372FE94F82BULL, 0xA54FF53A5F1D36F1ULL,
		0x510E527FADE682D1ULL, 0x9B05688C2B3E6C1FULL, 0x1F83D9ABFB41BD6BULL, 0x5BE0CD19137E2179ULL,
	};
	memcpy(s->h, iv, sizeof iv);
	s->len = 0;
	s->n = 0;
}

static void cr_sha512_write(cr_sha512 *s, const uint8_t *p, int64_t n) {
	s->len += (uint64_t)n;
	while (n > 0) {
		if (s->n == 0 && n >= 128) {
			cr_sha512_block(s->h, p);
			p += 128;
			n -= 128;
			continue;
		}
		int k = 128 - s->n;
		if (k > n) {
			k = (int)n;
		}
		memcpy(s->buf + s->n, p, (size_t)k);
		s->n += k;
		p += k;
		n -= k;
		if (s->n == 128) {
			cr_sha512_block(s->h, s->buf);
			s->n = 0;
		}
	}
}

static void cr_sha512_sum(const cr_sha512 *s0, uint8_t out[64]) {
	cr_sha512 s = *s0;
	uint64_t bits = s.len * 8;
	uint8_t pad[144] = {0x80};
	int padn = (s.n < 112 ? 112 : 240) - s.n;
	cr_sha512_write(&s, pad, padn);
	uint8_t lenb[16] = {0};
	for (int i = 0; i < 8; i++) {
		lenb[15 - i] = (uint8_t)(bits >> (8 * i));
	}
	cr_sha512_write(&s, lenb, 16);
	for (int i = 0; i < 8; i++) {
		for (int j = 0; j < 8; j++) {
			out[8 * i + j] = (uint8_t)(s.h[i] >> (56 - 8 * j));
		}
	}
}

// SHA-512 of up to three byte strings one after the other.
static void cr_sha512_3(const uint8_t *a, int64_t an, const uint8_t *b, int64_t bn, const uint8_t *c, int64_t cn, uint8_t out[64]) {
	cr_sha512 s;
	cr_sha512_init(&s);
	cr_sha512_write(&s, a, an);
	cr_sha512_write(&s, b, bn);
	cr_sha512_write(&s, c, cn);
	cr_sha512_sum(&s, out);
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
