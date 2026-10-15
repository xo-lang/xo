// crypto_ref.c: the C of the hashes that decision 0198 deleted from
// std/llrt/xo_crypto.c (SHA-256, HMAC, PBKDF2, BLAKE2b, Argon2, unpadded
// base64, verify_password; the runtime's are std/native/llrt/digest.xo),
// kept only as test data so crypto_diff.c, and internal/llvm's
// TestCryptoDiff that runs it in -short, keep passing until decision
// 0215 deletes that Go test; then this file goes too. No build compiles
// it into a runtime. Included by crypto_diff.c after ../xo_rt.c.

// b64_alpha and b64_val were xo_std.c's, which only this C used after
// the runtime's base64 moved to Xo (num.xo, digest.xo).
// cr_ror64 was xo_crypto.c's, for SHA-512 and this C; SHA-512 moved to
// digest.xo (the zero-C audit's B08).
static uint64_t cr_ror64(uint64_t x, int n) { return (x >> n) | (x << (64 - n)); }

// cr_le64 and cr_put_le64 were xo_crypto.c's, which Ed25519 moved out of
// (decision 0222).
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

// ref_equal was xo_crypto.c's xo_crypto_equal (crypto.equal), which moved
// to std/native/llrt/ct.xo (decision 0222); only this C uses it.
static int64_t ref_equal(const char *a, int64_t an, const char *b, int64_t bn) {
	if (an != bn) {
		return 0;
	}
	volatile uint8_t d = 0;
	for (int64_t i = 0; i < an; i++) {
		d |= (uint8_t)(a[i] ^ b[i]);
	}
	return d == 0;
}

static const char b64_alpha[] = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

// b64_val is the value of a base64 digit (the standard alphabet), or -1
// (cabi.xo's export of it was deleted with its last runtime caller).
static inline int b64_val(unsigned c) {
	if (c >= 'A' && c <= 'Z') {
		return (int)(c - 'A');
	}
	if (c >= 'a' && c <= 'z') {
		return (int)(c - 'a' + 26);
	}
	if (c >= '0' && c <= '9') {
		return (int)(c - '0' + 52);
	}
	return c == '+' ? 62 : c == '/' ? 63 : -1;
}

// nbuf_byte appends c (it was xo_std.c's, which only this C used).
static void nbuf_byte(nbuf *b, char c) { nbuf_put(b, &c, 1); }

// nbuf_take turns the buffer into a string object and frees it (it was
// xo_std.c's, which only this C used).
static xo_rstr *nbuf_take(nbuf *b, int64_t *len) {
	xo_rstr *s = nstr(b->p, b->n, len);
	free(b->p);
	b->p = NULL;
	b->n = b->cap = 0;
	return s;
}

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
		ok = (int)ref_equal((const char *)got, want.n, want.p, want.n);
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
		ok = (int)ref_equal((const char *)got, want.n, want.p, want.n);
		free(got);
	}
	free(salt.p);
	free(want.p);
	return ok;
}
