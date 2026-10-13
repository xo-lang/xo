// xo_std.c: the intrinsics of the native std implementation (decision
// 0074), included at the end of xo_rt.c. std/native/rt.xo declares them;
// internal/ssa lowers a call of `rt.name` there to xo_name here. A Str
// operand arrives as its bytes and length (an rs_* function gets the
// string object and consumes its first one); a Str result is a new
// string object, its length stored through the last parameter.
//
// Semantics follow std/rt (Go's strings, strconv, and time packages), so
// the native backends print what the Go backend prints.

#ifdef XO_UNICODE_C
#include "xo_unicode.c"
#endif

// ---------------------------------------------------------------------------
// Helpers

// mkstr is a new string object holding the n bytes at p.
static xo_rstr *nstr(const char *p, int64_t n, int64_t *len) {
	xo_rstr *s = rs_new(n);
	if (n > 0) {
		memcpy(s->data, p, (size_t)n);
	}
	*len = n;
	return s;
}

// xo_cffi_str copies n bytes at p into a new string: the Str results of
// the C adapters of `use c` (internal/cbind, decision 0116).
xo_rstr *xo_cffi_str(const char *p, int64_t n, int64_t *len) { return nstr(p, n, len); }

// A growable byte buffer for building results (malloc'd).
typedef struct {
	char *p;
	int64_t n, cap;
} nbuf;

static void nbuf_grow(nbuf *b, int64_t need) {
	if (b->n + need <= b->cap) {
		return;
	}
	int64_t c = b->cap < 64 ? 64 : b->cap * 2;
	while (c < b->n + need) {
		c *= 2;
	}
	b->p = realloc(b->p, (size_t)c);
	if (!b->p) {
		xo_oom();
	}
	b->cap = c;
}

static void nbuf_put(nbuf *b, const char *p, int64_t n) {
	if (n <= 0) {
		return; // b->p may still be NULL (UBSan: zero offset to null)
	}
	nbuf_grow(b, n);
	memcpy(b->p + b->n, p, (size_t)n);
	b->n += n;
}

static void nbuf_byte(nbuf *b, char c) {
	nbuf_grow(b, 1);
	b->p[b->n++] = c;
}

static void nbuf_str(nbuf *b, const char *s) { nbuf_put(b, s, (int64_t)strlen(s)); }

// buf_take turns the buffer into a string object and frees it.
static xo_rstr *nbuf_take(nbuf *b, int64_t *len) {
	xo_rstr *s = nstr(b->p, b->n, len);
	free(b->p);
	b->p = NULL;
	b->n = b->cap = 0;
	return s;
}

// utf8_decode decodes the rune at p (n bytes available) like Go's
// utf8.DecodeRune: an invalid or short sequence is U+FFFD of size 1.
static int32_t n_utf8_decode(const unsigned char *p, int64_t n, int *size) {
	unsigned c = p[0];
	if (c < 0x80) {
		*size = 1;
		return (int32_t)c;
	}
	int k;
	unsigned lo = 0x80, hi = 0xBF;
	int32_t r;
	if (c >= 0xC2 && c <= 0xDF) {
		k = 1;
		r = c & 0x1F;
	} else if (c >= 0xE0 && c <= 0xEF) {
		k = 2;
		r = c & 0x0F;
		if (c == 0xE0) {
			lo = 0xA0;
		} else if (c == 0xED) {
			hi = 0x9F;
		}
	} else if (c >= 0xF0 && c <= 0xF4) {
		k = 3;
		r = c & 0x07;
		if (c == 0xF0) {
			lo = 0x90;
		} else if (c == 0xF4) {
			hi = 0x8F;
		}
	} else {
		*size = 1;
		return 0xFFFD;
	}
	if (n < k + 1 || p[1] < lo || p[1] > hi) {
		*size = 1;
		return 0xFFFD;
	}
	r = (r << 6) | (p[1] & 0x3F);
	for (int j = 2; j <= k; j++) {
		if (p[j] < 0x80 || p[j] > 0xBF) {
			*size = 1;
			return 0xFFFD;
		}
		r = (r << 6) | (p[j] & 0x3F);
	}
	*size = k + 1;
	return r;
}

// utf8_encode writes r (a valid rune) to p and returns its size.
static int n_utf8_encode(char *p, int32_t r) {
	if (r < 0x80) {
		p[0] = (char)r;
		return 1;
	}
	if (r < 0x800) {
		p[0] = (char)(0xC0 | (r >> 6));
		p[1] = (char)(0x80 | (r & 0x3F));
		return 2;
	}
	if (r < 0x10000) {
		p[0] = (char)(0xE0 | (r >> 12));
		p[1] = (char)(0x80 | ((r >> 6) & 0x3F));
		p[2] = (char)(0x80 | (r & 0x3F));
		return 3;
	}
	p[0] = (char)(0xF0 | (r >> 18));
	p[1] = (char)(0x80 | ((r >> 12) & 0x3F));
	p[2] = (char)(0x80 | ((r >> 6) & 0x3F));
	p[3] = (char)(0x80 | (r & 0x3F));
	return 4;
}

// is_space is Go's unicode.IsSpace.
static int n_is_space(int32_t r) {
	switch (r) {
	case '\t':
	case '\n':
	case '\v':
	case '\f':
	case '\r':
	case ' ':
	case 0x85:
	case 0xA0:
	case 0x1680:
	case 0x2028:
	case 0x2029:
	case 0x202F:
	case 0x205F:
	case 0x3000:
		return 1;
	}
	return r >= 0x2000 && r <= 0x200A;
}

// The C path of the Unicode operations (decision 0180): compiled only
// with -DXO_UNICODE_C, which a build adds when its code calls them
// (XO_UNICODE=c selects std/native/llrt/unicode_c.xo); the default is
// std/native/llrt/unicode.xo, in Xo.
#ifdef XO_UNICODE_C
// to_case maps r like Go's unicode.ToLower (lower) or unicode.ToUpper.
static int32_t n_to_case(int32_t r, int lower) {
	if (r < 0x80) {
		if (lower && r >= 'A' && r <= 'Z') {
			return r + 32;
		}
		if (!lower && r >= 'a' && r <= 'z') {
			return r - 32;
		}
		return r;
	}
	int lo = 0, hi = (int)(sizeof xo_case_ranges / sizeof xo_case_ranges[0]);
	while (lo < hi) {
		int m = lo + (hi - lo) / 2;
		const xo_case_range *c = &xo_case_ranges[m];
		if (r < c->lo) {
			hi = m;
		} else if (r > c->hi) {
			lo = m + 1;
		} else {
			int32_t d = lower ? c->low : c->up;
			if (d == XO_UPPER_LOWER) {
				return c->lo + (((r - c->lo) & ~1) | (lower ? 1 : 0));
			}
			return r + d;
		}
	}
	return r;
}

// n_case_into appends strings.ToLower (lower) or ToUpper of the n bytes
// at p to b (strings.Map: an invalid byte becomes U+FFFD).
static void n_case_into(nbuf *b, const char *p, int64_t n, int lower) {
	const unsigned char *s = (const unsigned char *)p;
	for (int64_t i = 0; i < n;) {
		int size;
		int32_t r = n_utf8_decode(s + i, n - i, &size);
		char t[4];
		int k = n_utf8_encode(t, n_to_case(r, lower));
		nbuf_put(b, t, k);
		i += size;
	}
}

// map_case is strings.ToLower or strings.ToUpper.
static xo_rstr *n_map_case(const char *p, int64_t n, int lower, int64_t *len) {
	int ascii = 1;
	for (int64_t i = 0; i < n; i++) {
		if ((unsigned char)p[i] >= 0x80) {
			ascii = 0;
			break;
		}
	}
	if (ascii) {
		xo_rstr *s = nstr(p, n, len);
		for (int64_t i = 0; i < n; i++) {
			char c = s->data[i];
			if (lower && c >= 'A' && c <= 'Z') {
				s->data[i] = (char)(c + 32);
			} else if (!lower && c >= 'a' && c <= 'z') {
				s->data[i] = (char)(c - 32);
			}
		}
		return s;
	}
	nbuf b = {0};
	n_case_into(&b, p, n, lower);
	return nbuf_take(&b, len);
}

// xo_str_eq_lower is `a.to_lower() == b.to_lower()` without making the
// lowered strings: ASCII bytes compare lowered in place; from the first
// non-ASCII byte of either (a rune boundary of both, since everything
// before is ASCII) the rests are lowered and compared (std/rt's
// Str_eq_lower on the Go backend).
int64_t xo_str_eq_lower(const char *p, int64_t n, const char *q, int64_t m) {
	int64_t i = 0;
	for (; i < n && i < m; i++) {
		unsigned a = (unsigned char)p[i], c = (unsigned char)q[i];
		if (a >= 0x80 || c >= 0x80) {
			break;
		}
		a += (a >= 'A' && a <= 'Z') ? 32 : 0;
		c += (c >= 'A' && c <= 'Z') ? 32 : 0;
		if (a != c) {
			return 0;
		}
	}
	if (i == n && i == m) {
		return 1;
	}
	if (i == n || i == m) {
		return 0; // a nonempty rest never lowers to nothing
	}
	nbuf x = {0}, y = {0};
	n_case_into(&x, p + i, n - i, 1);
	n_case_into(&y, q + i, m - i, 1);
	int64_t eq = x.n == y.n && (x.n == 0 || memcmp(x.p, y.p, (size_t)x.n) == 0);
	free(x.p);
	free(y.p);
	return eq;
}

xo_rstr *xo_str_lower(const char *p, int64_t n, int64_t *len) { return n_map_case(p, n, 1, len); }

xo_rstr *xo_str_upper(const char *p, int64_t n, int64_t *len) { return n_map_case(p, n, 0, len); }
#endif

// ---------------------------------------------------------------------------
// Strings (std.md 2.2)

static int64_t n_mem_index(const char *p, int64_t n, const char *q, int64_t m) {
	if (m == 0) {
		return 0;
	}
	if (m > n) {
		return -1;
	}
	const char *end = p + n - m;
	for (const char *s = p; s <= end; s++) {
		s = memchr(s, q[0], (size_t)(end - s + 1));
		if (!s) {
			return -1;
		}
		if (memcmp(s, q, (size_t)m) == 0) {
			return s - p;
		}
	}
	return -1;
}

int64_t xo_str_contains(const char *p, int64_t n, const char *q, int64_t m) { return n_mem_index(p, n, q, m) >= 0; }

int64_t xo_str_starts_with(const char *p, int64_t n, const char *q, int64_t m) {
	return m <= n && memcmp(p, q, (size_t)m) == 0;
}

int64_t xo_str_ends_with(const char *p, int64_t n, const char *q, int64_t m) {
	return m <= n && memcmp(p + n - m, q, (size_t)m) == 0;
}

int64_t xo_str_find(const char *p, int64_t n, const char *q, int64_t m, int64_t from) {
	if (from < 0) {
		from = 0;
	}
	if (from > n) {
		return -1;
	}
	int64_t i = n_mem_index(p + from, n - from, q, m);
	return i < 0 ? -1 : i + from;
}

xo_rstr *xo_str_slice(const char *p, int64_t n, int64_t lo, int64_t hi, int64_t *len) {
	if (lo < 0) {
		lo = 0;
	}
	if (hi > n) {
		hi = n;
	}
	if (hi < lo) {
		hi = lo;
	}
	return nstr(p + lo, hi - lo, len);
}

int64_t xo_str_byte(const char *p, int64_t n, int64_t i) {
	if (i < 0 || i >= n) {
		return -1;
	}
	return (unsigned char)p[i];
}

xo_rstr *xo_str_of_byte(int64_t b, int64_t *len) {
	char c = (char)b;
	return nstr(&c, 1, len);
}

#ifdef XO_STRINGS_C
// the C path of the Str operations (decision 0193): only
// XO_LLRT_C=strings builds call it (std/native/llrt/strings_c.xo)
// xo_str_trim is strings.TrimSpace (mode 0), TrimLeftFunc (1), or
// TrimRightFunc (2) with unicode.IsSpace.
xo_rstr *xo_str_trim(const char *p, int64_t n, int64_t mode, int64_t *len) {
	const unsigned char *s = (const unsigned char *)p;
	int64_t lo = 0, hi = n;
	if (mode != 2) {
		while (lo < hi) {
			int size;
			int32_t r = n_utf8_decode(s + lo, hi - lo, &size);
			if (!n_is_space(r) || (r == 0xFFFD && size == 1)) {
				break;
			}
			lo += size;
		}
	}
	if (mode != 1) {
		while (hi > lo) {
			// the rune ending at hi
			int64_t st = hi - 1;
			while (st > lo && hi - st < 4 && (s[st] & 0xC0) == 0x80) {
				st--;
			}
			int size;
			int32_t r = n_utf8_decode(s + st, hi - st, &size);
			if (st + size != hi) {
				// not a whole rune: an invalid byte, never space
				break;
			}
			if (!n_is_space(r) || (r == 0xFFFD && size == 1)) {
				break;
			}
			hi = st;
		}
	}
	return nstr(p + lo, hi - lo, len);
}
#endif



int64_t xo_str_rune_count(const char *p, int64_t n) {
	const unsigned char *s = (const unsigned char *)p;
	int64_t c = 0;
	for (int64_t i = 0; i < n;) {
		if (s[i] < 0x80) {
			i++;
		} else {
			int size;
			n_utf8_decode(s + i, n - i, &size);
			i += size;
		}
		c++;
	}
	return c;
}

// str_list_new is an empty List[Str] with room for cap elements.
static xo_rlist *n_str_list_new(int64_t cap) { return xo_rl_new(cap < 4 ? 4 : cap, 16); }

// str_list_add appends the n bytes at p to a List[Str] made by
// str_list_new (growing it).
static xo_rlist *n_str_list_add(xo_rlist *l, const char *p, int64_t n) {
	if (l->len == l->cap) {
		l = xo_rl_grow(l, 16);
	}
	int64_t len;
	xo_rstr *s = nstr(p, n, &len);
	((xo_rstr **)rl_data(l))[2 * l->len] = s;
	((int64_t *)rl_data(l))[2 * l->len + 1] = n;
	l->len++;
	return l;
}

// xo_str_split is strings.Split: an empty separator splits into UTF-8
// sequences (an invalid byte alone).
xo_rlist *xo_str_split(const char *p, int64_t n, const char *q, int64_t m) {
	xo_rlist *l = n_str_list_new(4);
	if (m == 0) {
		const unsigned char *s = (const unsigned char *)p;
		for (int64_t i = 0; i < n;) {
			int size;
			n_utf8_decode(s + i, n - i, &size);
			l = n_str_list_add(l, p + i, size);
			i += size;
		}
		return l;
	}
	int64_t start = 0;
	for (;;) {
		int64_t i = n_mem_index(p + start, n - start, q, m);
		if (i < 0) {
			break;
		}
		l = n_str_list_add(l, p + start, i);
		start += i + m;
	}
	return n_str_list_add(l, p + start, n - start);
}


// xo_str_lines splits on \n and strips a trailing \r; a final newline
// gives no empty last line.
xo_rlist *xo_str_lines(const char *p, int64_t n) {
	xo_rlist *l = n_str_list_new(4);
	int64_t start = 0;
	while (start < n) {
		const char *nl = memchr(p + start, '\n', (size_t)(n - start));
		int64_t end = nl ? nl - p : n;
		int64_t e = end;
		if (e > start && p[e - 1] == '\r') {
			e--;
		}
		l = n_str_list_add(l, p + start, e - start);
		start = end + 1;
	}
	return l;
}


#ifdef XO_STRINGS_C
// the C path of the Str operations (decision 0193): only
// XO_LLRT_C=strings builds call it (std/native/llrt/strings_c.xo)
// xo_str_replace is strings.ReplaceAll: an empty old inserts new before
// every UTF-8 sequence and at the end.
xo_rstr *xo_str_replace(const char *p, int64_t n, const char *o, int64_t on, const char *w, int64_t wn, int64_t *len) {
	nbuf b = {0};
	nbuf_grow(&b, n);
	if (on == 0) {
		const unsigned char *s = (const unsigned char *)p;
		nbuf_put(&b, w, wn);
		for (int64_t i = 0; i < n;) {
			int size;
			n_utf8_decode(s + i, n - i, &size);
			nbuf_put(&b, p + i, size);
			nbuf_put(&b, w, wn);
			i += size;
		}
		return nbuf_take(&b, len);
	}
	int64_t start = 0;
	for (;;) {
		int64_t i = n_mem_index(p + start, n - start, o, on);
		if (i < 0) {
			break;
		}
		nbuf_put(&b, p + start, i);
		nbuf_put(&b, w, wn);
		start += i + on;
	}
	nbuf_put(&b, p + start, n - start);
	return nbuf_take(&b, len);
}
#endif


xo_rstr *xo_str_repeat(const char *p, int64_t n, int64_t k, int64_t *len) {
	if (k <= 0 || n == 0) {
		return nstr(p, 0, len);
	}
	if (n > INT64_MAX / k) {
		fault_text("strings: Repeat output length overflow", "?");
	}
	xo_rstr *s = rs_new(n * k);
	for (int64_t i = 0; i < k; i++) {
		memcpy(s->data + i * n, p, (size_t)n);
	}
	*len = n * k;
	return s;
}

xo_rstr *xo_str_join(xo_rlist *l, const char *sep, int64_t sn, int64_t *len) {
	int64_t total = 0;
	for (int64_t i = 0; i < l->len; i++) {
		total += ((int64_t *)rl_data(l))[2 * i + 1] + (i > 0 ? sn : 0);
	}
	xo_rstr *s = rs_new(total);
	int64_t o = 0;
	for (int64_t i = 0; i < l->len; i++) {
		if (i > 0) {
			memcpy(s->data + o, sep, (size_t)sn);
			o += sn;
		}
		xo_rstr *e = ((xo_rstr **)rl_data(l))[2 * i];
		int64_t en = ((int64_t *)rl_data(l))[2 * i + 1];
		memcpy(s->data + o, e->data, (size_t)en);
		o += en;
	}
	*len = total;
	return s;
}

// xo_rs_append is s + t (consumes s; appended in place when unique).
xo_rstr *xo_rs_append(xo_rstr *a, int64_t an, xo_rstr *b, int64_t bn, int64_t *len) {
	*len = an + bn;
	return rs_append(a, an, b->data, bn);
}

// ---------------------------------------------------------------------------
// Numbers

// parse_int is strconv.ParseInt(s, 10, 64): an optional sign and decimal
// digits, in range.
static int n_parse_int(const char *p, int64_t n, int64_t *out) {
	int64_t i = 0;
	int neg = 0;
	if (n == 0) {
		return 0;
	}
	if (p[0] == '+' || p[0] == '-') {
		neg = p[0] == '-';
		i++;
	}
	if (i == n) {
		return 0;
	}
	uint64_t v = 0;
	uint64_t lim = neg ? (uint64_t)INT64_MAX + 1 : (uint64_t)INT64_MAX;
	for (; i < n; i++) {
		if (p[i] < '0' || p[i] > '9') {
			return 0;
		}
		uint64_t d = (uint64_t)(p[i] - '0');
		if (v > (lim - d) / 10) {
			return 0;
		}
		v = v * 10 + d;
	}
	*out = neg ? (int64_t)(0 - v) : (int64_t)v;
	return 1;
}

#ifdef XO_TEXT_C
// the C path of the number and time text (decision 0193): only
// XO_LLRT_C=text builds call it (std/native/llrt/text_c.xo)
int64_t xo_int_parse_ok(const char *p, int64_t n) {
	int64_t v;
	return n_parse_int(p, n, &v);
}

int64_t xo_int_parse(const char *p, int64_t n) {
	int64_t v = 0;
	n_parse_int(p, n, &v);
	return v;
}
#endif


static int n_lower_eq(const char *p, int64_t n, const char *w) {
	int64_t m = (int64_t)strlen(w);
	if (n != m) {
		return 0;
	}
	for (int64_t i = 0; i < n; i++) {
		char c = p[i];
		if (c >= 'A' && c <= 'Z') {
			c = (char)(c + 32);
		}
		if (c != w[i]) {
			return 0;
		}
	}
	return 1;
}

// parse_float is strconv.ParseFloat(s, 64) for decimal forms, inf,
// infinity, and nan (no hex floats, no underscores): 0 when malformed or
// out of range.
static int n_parse_float(const char *p, int64_t n, double *out) {
	if (n == 0 || n > 4096) {
		return 0;
	}
	int64_t i = 0;
	int neg = 0;
	if (p[0] == '+' || p[0] == '-') {
		neg = p[0] == '-';
		i++;
	}
	if (n_lower_eq(p + i, n - i, "inf") || n_lower_eq(p + i, n - i, "infinity")) {
		*out = neg ? -INFINITY : INFINITY;
		return 1;
	}
	if (i == 0 && n_lower_eq(p, n, "nan")) {
		*out = NAN;
		return 1;
	}
	int64_t digits = 0;
	while (i < n && p[i] >= '0' && p[i] <= '9') {
		i++;
		digits++;
	}
	if (i < n && p[i] == '.') {
		i++;
		while (i < n && p[i] >= '0' && p[i] <= '9') {
			i++;
			digits++;
		}
	}
	if (digits == 0) {
		return 0;
	}
	if (i < n && (p[i] == 'e' || p[i] == 'E')) {
		i++;
		if (i < n && (p[i] == '+' || p[i] == '-')) {
			i++;
		}
		int64_t ed = 0;
		while (i < n && p[i] >= '0' && p[i] <= '9') {
			i++;
			ed++;
		}
		if (ed == 0) {
			return 0;
		}
	}
	if (i != n) {
		return 0;
	}
	char *c = cstr_of(p, n);
	double v = strtod(c, NULL);
	free(c);
	if (isinf(v)) {
		return 0;
	}
	*out = v;
	return 1;
}

#ifdef XO_NUM_C
// the C path of the number text (decision 0193 step 3): only
// XO_LLRT_C=num builds call it (std/native/llrt/num_c.xo)
int64_t xo_float_parse_ok(const char *p, int64_t n) {
	double v;
	return n_parse_float(p, n, &v);
}

double xo_float_parse(const char *p, int64_t n) {
	double v = 0;
	n_parse_float(p, n, &v);
	return v;
}

#endif

// ---------------------------------------------------------------------------
// Duration and Time (std.md 2.6)

// fmt_frac writes v / 10^prec with trailing zeros of the fraction
// removed, as Go's time.Duration.String does (fmtFrac and fmtInt).
static int n_fmt_duration(char *out, int64_t d) {
	// Go's time.Duration.String
	char buf[32];
	int w = (int)sizeof buf;
	uint64_t u = (uint64_t)d;
	int neg = d < 0;
	if (neg) {
		u = 0 - u;
	}
	if (u < 1000000000ULL) {
		// less than a second: ns, µs, or ms, with a fraction
		int prec;
		w--;
		buf[w] = 's';
		w--;
		if (u == 0) {
			memcpy(out, "0s", 2);
			return 2;
		} else if (u < 1000ULL) {
			prec = 0;
			buf[w] = 'n';
		} else if (u < 1000000ULL) {
			prec = 3;
			// U+00B5 'µ' micro sign, 0xC2 0xB5
			w--;
			memcpy(buf + w, "\xC2\xB5", 2);
		} else {
			prec = 6;
			buf[w] = 'm';
		}
		// fmtFrac
		int print = 0;
		for (int i = 0; i < prec; i++) {
			int digit = (int)(u % 10);
			print = print || digit != 0;
			if (print) {
				w--;
				buf[w] = (char)(digit + '0');
			}
			u /= 10;
		}
		if (print) {
			w--;
			buf[w] = '.';
		}
		// fmtInt
		if (u == 0) {
			w--;
			buf[w] = '0';
		} else {
			while (u > 0) {
				w--;
				buf[w] = (char)(u % 10 + '0');
				u /= 10;
			}
		}
	} else {
		w--;
		buf[w] = 's';
		int print = 0;
		for (int i = 0; i < 9; i++) {
			int digit = (int)(u % 10);
			print = print || digit != 0;
			if (print) {
				w--;
				buf[w] = (char)(digit + '0');
			}
			u /= 10;
		}
		if (print) {
			w--;
			buf[w] = '.';
		}
		// u is now whole seconds
		uint64_t secs = u % 60;
		u /= 60;
		if (secs == 0) {
			w--;
			buf[w] = '0';
		} else {
			while (secs > 0) {
				w--;
				buf[w] = (char)(secs % 10 + '0');
				secs /= 10;
			}
		}
		if (u > 0) {
			w--;
			buf[w] = 'm';
			uint64_t mins = u % 60;
			u /= 60;
			if (mins == 0) {
				w--;
				buf[w] = '0';
			} else {
				while (mins > 0) {
					w--;
					buf[w] = (char)(mins % 10 + '0');
					mins /= 10;
				}
			}
			if (u > 0) {
				w--;
				buf[w] = 'h';
				while (u > 0) {
					w--;
					buf[w] = (char)(u % 10 + '0');
					u /= 10;
				}
			}
		}
	}
	if (neg) {
		w--;
		buf[w] = '-';
	}
	int k = (int)sizeof buf - w;
	memcpy(out, buf + w, (size_t)k);
	return k;
}

#ifdef XO_TEXT_C
// the C path of the number and time text (decision 0193): only
// XO_LLRT_C=text builds call it (std/native/llrt/text_c.xo)
// xo_rs_append_duration is s + the text of a Duration (time.Duration's
// String).
xo_rstr *xo_rs_append_duration(xo_rstr *a, int64_t an, int64_t d, int64_t *len) {
	char t[40];
	int k = n_fmt_duration(t, d);
	*len = an + k;
	return rs_append(a, an, t, k);
}
#endif


// n_fmt_time writes a Time of t nanoseconds since the Unix epoch as RFC
// 3339 in UTC with the nanoseconds' trailing zeros removed (Go's
// time.RFC3339Nano).
static int n_fmt_time(char *out, int64_t t) {
	int64_t secs = t / 1000000000, ns = t % 1000000000;
	if (ns < 0) {
		ns += 1000000000;
		secs--;
	}
	// days since the epoch to a civil date (Howard Hinnant's algorithm)
	int64_t days = secs / 86400, rem = secs % 86400;
	if (rem < 0) {
		rem += 86400;
		days--;
	}
	int64_t z = days + 719468;
	int64_t era = (z >= 0 ? z : z - 146096) / 146097;
	int64_t doe = z - era * 146097;
	int64_t yoe = (doe - doe / 1460 + doe / 36524 - doe / 146096) / 365;
	int64_t y = yoe + era * 400;
	int64_t doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
	int64_t mp = (5 * doy + 2) / 153;
	int64_t d = doy - (153 * mp + 2) / 5 + 1;
	int64_t m = mp < 10 ? mp + 3 : mp - 9;
	if (m <= 2) {
		y++;
	}
	int k = snprintf(out, 48, "%04lld-%02lld-%02lldT%02lld:%02lld:%02lld", (long long)y, (long long)m, (long long)d,
	                 (long long)(rem / 3600), (long long)(rem / 60 % 60), (long long)(rem % 60));
	if (ns) {
		char f[16];
		snprintf(f, sizeof f, "%09lld", (long long)ns);
		int fl = 9;
		while (f[fl - 1] == '0') {
			fl--;
		}
		out[k++] = '.';
		memcpy(out + k, f, (size_t)fl);
		k += fl;
	}
	out[k++] = 'Z';
	out[k] = 0;
	return k;
}

#ifdef XO_TEXT_C
// the C path of the number and time text (decision 0193): only
// XO_LLRT_C=text builds call it (std/native/llrt/text_c.xo)
// xo_rs_append_time is s + the text of a Time (n_fmt_time).
xo_rstr *xo_rs_append_time(xo_rstr *a, int64_t an, int64_t t, int64_t *len) {
	char b[64];
	int k = n_fmt_time(b, t);
	*len = an + k;
	return rs_append(a, an, b, k);
}
#endif


#ifdef XO_TEXT_C
// the C path of the number and time text (decision 0193): only
// XO_LLRT_C=text builds call it (std/native/llrt/text_c.xo)
// xo_rs_append_quote is s + t in double quotes with the escapes of the Go
// backend's Debug of a Str (std/rt show.go quote): \" \\ \n \t \r \0,
// \u{FFFD} for U+FFFD and invalid bytes, \xhh for other control bytes.
xo_rstr *xo_rs_append_quote(xo_rstr *a, int64_t an, xo_rstr *b, int64_t bn, int64_t *len) {
	nbuf o = {0};
	nbuf_byte(&o, '"');
	const unsigned char *s = (const unsigned char *)b->data;
	for (int64_t i = 0; i < bn;) {
		int size;
		int32_t r = n_utf8_decode(s + i, bn - i, &size);
		switch (r) {
		case '"':
			nbuf_str(&o, "\\\"");
			break;
		case '\\':
			nbuf_str(&o, "\\\\");
			break;
		case '\n':
			nbuf_str(&o, "\\n");
			break;
		case '\t':
			nbuf_str(&o, "\\t");
			break;
		case '\r':
			nbuf_str(&o, "\\r");
			break;
		case 0:
			nbuf_str(&o, "\\0");
			break;
		case 0xFFFD:
			nbuf_str(&o, "\\u{FFFD}");
			break;
		default:
			if (r < 0x20 || r == 0x7f) {
				char h[8];
				snprintf(h, sizeof h, "\\x%x%x", (unsigned)r >> 4, (unsigned)r & 15);
				nbuf_str(&o, h);
			} else {
				nbuf_put(&o, (const char *)s + i, size);
			}
		}
		i += size;
	}
	nbuf_byte(&o, '"');
	*len = an + o.n;
	xo_rstr *res = rs_append(a, an, o.p, o.n);
	free(o.p);
	return res;
}
#endif


// xo_exit flushes standard output and exits with status code (main
// returned an error: status 1, after "error: <text>" on standard error).
static void tw_exit(int64_t code);

void xo_exit(int64_t code) {
	if (tw.on) {
		// returns only inside testing.run_main, to unwind main
		tw_exit(code);
		return;
	}
	xo_flush();
	exit((int)code);
}

// ---------------------------------------------------------------------------
// Conversions for std/json: base64, durations, RFC 3339 times

static const char b64_alpha[] = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

#ifdef XO_JSONTEXT_C
// the C path of std/json's text (decision 0193 step 4): only
// XO_LLRT_C=jsontext builds call it (std/native/llrt/jsontext_c.xo)
// xo_rs_append_b64_quoted is s + the standard base64 of t (padded) in
// double quotes (a Bytes in JSON).
xo_rstr *xo_rs_append_b64_quoted(xo_rstr *a, int64_t an, xo_rstr *b, int64_t bn, int64_t *len) {
	nbuf o = {0};
	nbuf_byte(&o, '"');
	const unsigned char *s = (const unsigned char *)b->data;
	for (int64_t i = 0; i < bn; i += 3) {
		unsigned v = (unsigned)s[i] << 16;
		if (i + 1 < bn) {
			v |= (unsigned)s[i + 1] << 8;
		}
		if (i + 2 < bn) {
			v |= s[i + 2];
		}
		char q[4] = {b64_alpha[v >> 18 & 63], b64_alpha[v >> 12 & 63], i + 1 < bn ? b64_alpha[v >> 6 & 63] : '=', i + 2 < bn ? b64_alpha[v & 63] : '='};
		nbuf_put(&o, q, 4);
	}
	nbuf_byte(&o, '"');
	*len = an + o.n;
	xo_rstr *r = rs_append(a, an, o.p, o.n);
	free(o.p);
	return r;
}
#endif

#ifdef XO_NUM_C
// the C path of the number text (decision 0193 step 3): only
// XO_LLRT_C=num builds call it (std/native/llrt/num_c.xo)
// xo_b64_encode is the standard base64 of the bytes, padded
// (Bytes.to_base64, base64.StdEncoding.EncodeToString).
xo_rstr *xo_b64_encode(const char *p, int64_t n, int64_t *len) {
	nbuf o = {0};
	const unsigned char *s = (const unsigned char *)p;
	for (int64_t i = 0; i < n; i += 3) {
		unsigned v = (unsigned)s[i] << 16;
		if (i + 1 < n) {
			v |= (unsigned)s[i + 1] << 8;
		}
		if (i + 2 < n) {
			v |= s[i + 2];
		}
		char q[4] = {b64_alpha[v >> 18 & 63], b64_alpha[v >> 12 & 63], i + 1 < n ? b64_alpha[v >> 6 & 63] : '=', i + 2 < n ? b64_alpha[v & 63] : '='};
		nbuf_put(&o, q, 4);
	}
	return nbuf_take(&o, len);
}
#endif

static __attribute__((unused)) int b64_val(unsigned c) {
	if (c >= 'A' && c <= 'Z') {
		return (int)(c - 'A');
	}
	if (c >= 'a' && c <= 'z') {
		return (int)(c - 'a' + 26);
	}
	if (c >= '0' && c <= '9') {
		return (int)(c - '0' + 52);
	}
	if (c == '+') {
		return 62;
	}
	if (c == '/') {
		return 63;
	}
	return -1;
}

#ifdef XO_NUM_C
// n_b64_decode is base64.StdEncoding.DecodeString: padding required,
// \r and \n ignored; 0 when the text is not valid.
static int n_b64_decode(const char *p, int64_t n, nbuf *out) {
	unsigned char q[4];
	int k = 0, pad = 0;
	for (int64_t i = 0; i < n; i++) {
		unsigned c = (unsigned char)p[i];
		if (c == '\r' || c == '\n') {
			continue;
		}
		if (pad) {
			// after padding only more padding of the same quantum
			if (c != '=' || k == 0) {
				return 0;
			}
			q[k++] = 0;
			pad++;
		} else if (c == '=') {
			if (k < 2) {
				return 0;
			}
			pad = 1;
			q[k++] = 0;
		} else {
			int v = b64_val(c);
			if (v < 0) {
				return 0;
			}
			q[k++] = (unsigned char)v;
		}
		if (k == 4) {
			unsigned v = (unsigned)q[0] << 18 | (unsigned)q[1] << 12 | (unsigned)q[2] << 6 | q[3];
			char d[3] = {(char)(v >> 16), (char)(v >> 8), (char)v};
			int keep = pad == 0 ? 3 : pad == 1 ? 2 : 1;
			if (pad == 2 && k == 4) {
				keep = 1;
			}
			nbuf_put(out, d, keep);
			if (pad) {
				// padding ends the text (only \r and \n may follow)
				for (int64_t j = i + 1; j < n; j++) {
					if (p[j] != '\r' && p[j] != '\n') {
						return 0;
					}
				}
				return 1;
			}
			k = 0;
		}
	}
	return k == 0 && !pad;
}

int64_t xo_b64_ok(const char *p, int64_t n) {
	nbuf o = {0};
	int ok = n_b64_decode(p, n, &o);
	free(o.p);
	return ok;
}

xo_rstr *xo_b64_decode(const char *p, int64_t n, int64_t *len) {
	nbuf o = {0};
	n_b64_decode(p, n, &o);
	return nbuf_take(&o, len);
}

#endif

#ifdef XO_TEXT_C
// the C path of the number and time text (decision 0193): only
// XO_LLRT_C=text builds call it (std/native/llrt/text_c.xo)
// n_parse_duration is std/rt's parseDuration: time.ParseDuration of the
// text with surrounding white space removed.
static int n_parse_duration(const char *p0, int64_t n0, int64_t *out) {
	const unsigned char *s = (const unsigned char *)p0;
	int64_t n = n0;
	// strings.TrimSpace
	while (n > 0) {
		int size;
		int32_t r = n_utf8_decode(s, n, &size);
		if (!n_is_space(r) || (r == 0xFFFD && size == 1)) {
			break;
		}
		s += size;
		n -= size;
	}
	while (n > 0) {
		int64_t st = n - 1;
		while (st > 0 && n - st < 4 && (s[st] & 0xC0) == 0x80) {
			st--;
		}
		int size;
		int32_t r = n_utf8_decode(s + st, n - st, &size);
		if (st + size != n || !n_is_space(r) || (r == 0xFFFD && size == 1)) {
			break;
		}
		n = st;
	}
	uint64_t d = 0;
	int neg = 0;
	int64_t i = 0;
	if (n > 0 && (s[0] == '-' || s[0] == '+')) {
		neg = s[0] == '-';
		i++;
	}
	if (n - i == 1 && s[i] == '0') {
		*out = 0;
		return 1;
	}
	if (i == n) {
		return 0;
	}
	const uint64_t lim = (uint64_t)1 << 63;
	while (i < n) {
		uint64_t v = 0, f = 0;
		double scale = 1;
		if (!(s[i] == '.' || (s[i] >= '0' && s[i] <= '9'))) {
			return 0;
		}
		int64_t pl = i;
		while (i < n && s[i] >= '0' && s[i] <= '9') {
			if (v > lim / 10) {
				return 0;
			}
			v = v * 10 + (s[i] - '0');
			if (v > lim) {
				return 0;
			}
			i++;
		}
		int pre = i != pl;
		int post = 0;
		if (i < n && s[i] == '.') {
			i++;
			int64_t pf = i;
			int overflow = 0;
			while (i < n && s[i] >= '0' && s[i] <= '9') {
				if (!overflow) {
					if (f > (lim - 1) / 10) {
						overflow = 1;
					} else {
						uint64_t y = f * 10 + (s[i] - '0');
						if (y > lim) {
							overflow = 1;
						} else {
							f = y;
							scale *= 10;
						}
					}
				}
				i++;
			}
			post = i != pf;
		}
		if (!pre && !post) {
			return 0;
		}
		int64_t us = i;
		while (i < n && !(s[i] == '.' || (s[i] >= '0' && s[i] <= '9'))) {
			i++;
		}
		if (i == us) {
			return 0;
		}
		const char *u = (const char *)s + us;
		int64_t ul = i - us;
		uint64_t unit = 0;
		if (ul == 2 && memcmp(u, "ns", 2) == 0) {
			unit = 1;
		} else if ((ul == 2 && memcmp(u, "us", 2) == 0) || (ul == 3 && memcmp(u, "\xC2\xB5s", 3) == 0) ||
		           (ul == 3 && memcmp(u, "\xCE\xBCs", 3) == 0)) {
			unit = 1000;
		} else if (ul == 2 && memcmp(u, "ms", 2) == 0) {
			unit = 1000000;
		} else if (ul == 1 && u[0] == 's') {
			unit = 1000000000ULL;
		} else if (ul == 1 && u[0] == 'm') {
			unit = 60000000000ULL;
		} else if (ul == 1 && u[0] == 'h') {
			unit = 3600000000000ULL;
		} else {
			return 0;
		}
		if (v > lim / unit) {
			return 0;
		}
		v *= unit;
		if (f > 0) {
			v += (uint64_t)((double)f * ((double)unit / scale));
			if (v > lim) {
				return 0;
			}
		}
		d += v;
		if (d > lim) {
			return 0;
		}
	}
	if (neg) {
		*out = (int64_t)(0 - d);
		return 1;
	}
	if (d > lim - 1) {
		return 0;
	}
	*out = (int64_t)d;
	return 1;
}

int64_t xo_duration_parse_ok(const char *p, int64_t n) {
	int64_t d;
	return n_parse_duration(p, n, &d);
}

int64_t xo_duration_parse(const char *p, int64_t n) {
	int64_t d = 0;
	n_parse_duration(p, n, &d);
	return d;
}
#endif


// n_days_from_civil is the days since 1970-01-01 of a date.
static int64_t n_days_from_civil(int64_t y, int64_t m, int64_t d) {
	y -= m <= 2;
	int64_t era = (y >= 0 ? y : y - 399) / 400;
	int64_t yoe = y - era * 400;
	int64_t doy = (153 * (m + (m > 2 ? -3 : 9)) + 2) / 5 + d - 1;
	int64_t doe = yoe * 365 + yoe / 4 - yoe / 100 + doy;
	return era * 146097 + doe - 719468;
}

#ifdef XO_TEXT_C
// the C path of the number and time text (decision 0193): only
// XO_LLRT_C=text builds call it (std/native/llrt/text_c.xo)
static int n_days_in(int m, int64_t y) {
	static const int dm[] = {31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31};
	if (m == 2 && ((y % 4 == 0 && y % 100 != 0) || y % 400 == 0)) {
		return 29;
	}
	return dm[m - 1];
}

// n_parse_time is time.Parse(time.RFC3339Nano, s) for the RFC 3339 forms
// its fast path (parseRFC3339) accepts: ns since the Unix epoch.
static int n_parse_time(const char *p, int64_t n, int64_t *out) {
	const unsigned char *s = (const unsigned char *)p;
	if (n < 19) {
		return 0;
	}
	int ok = 1;
#define NUM(off, len, lo, hi, dst)                          \
	do {                                                    \
		int64_t x_ = 0;                                     \
		for (int k_ = 0; k_ < (len); k_++) {                \
			unsigned c_ = s[(off) + k_];                    \
			if (c_ < '0' || c_ > '9') {                     \
				ok = 0;                                     \
			}                                               \
			x_ = x_ * 10 + (int64_t)(c_ - '0');             \
		}                                                   \
		if (!ok || x_ < (lo) || x_ > (hi)) {                \
			ok = 0;                                         \
		}                                                   \
		(dst) = x_;                                         \
	} while (0)
	int64_t year, month, day, hour, min, sec;
	NUM(0, 4, 0, 9999, year);
	NUM(5, 2, 1, 12, month);
	if (!ok) {
		return 0;
	}
	NUM(8, 2, 1, n_days_in((int)month, year), day);
	NUM(11, 2, 0, 23, hour);
	NUM(14, 2, 0, 59, min);
	NUM(17, 2, 0, 59, sec);
	if (!ok || !(s[4] == '-' && s[7] == '-' && s[10] == 'T' && s[13] == ':' && s[16] == ':')) {
		return 0;
	}
	int64_t i = 19, nsec = 0;
	if (n - i >= 2 && s[i] == '.' && s[i + 1] >= '0' && s[i + 1] <= '9') {
		int64_t j = i + 1;
		while (j < n && s[j] >= '0' && s[j] <= '9') {
			j++;
		}
		int64_t digits = j - i - 1, scale = 1;
		for (int64_t k = 0; k < 9; k++) {
			nsec *= 10;
			if (k < digits) {
				nsec += s[i + 1 + k] - '0';
			}
		}
		(void)scale;
		i = j;
	}
	int64_t zone = 0;
	if (!(n - i == 1 && s[i] == 'Z')) {
		if (n - i != 6) {
			return 0;
		}
		int64_t hr, mm;
		NUM(i + 1, 2, 0, 23, hr);
		NUM(i + 4, 2, 0, 59, mm);
		if (!ok || !((s[i] == '-' || s[i] == '+') && s[i + 3] == ':')) {
			return 0;
		}
		zone = (hr * 60 + mm) * 60;
		if (s[i] == '-') {
			zone = -zone;
		}
	}
#undef NUM
	int64_t secs = n_days_from_civil(year, month, day) * 86400 + hour * 3600 + min * 60 + sec - zone;
	*out = secs * 1000000000 + nsec;
	return 1;
}

int64_t xo_time_parse_ok(const char *p, int64_t n) {
	int64_t t;
	return n_parse_time(p, n, &t);
}

int64_t xo_time_parse(const char *p, int64_t n) {
	int64_t t = 0;
	n_parse_time(p, n, &t);
	return t;
}
#endif


// Sized integers, Rune, Bytes, builders, files (decisions 0079, 0081)
#include "xo_bytes.c"

#ifdef XO_JSONTEXT_C
// JSON (std.md 5): the C path of std/json's text (decision 0193 step 4),
// which only XO_LLRT_C=jsontext builds call (std/native/llrt/
// jsontext_c.xo); std/native/llrt/jsontext.xo is the default
#include "xo_json.c"
#endif

// HTTP/1.1 (std.md 4)
#include "xo_http.c"

// std/crypto and the Rand capability (decision 0083)
#include "xo_crypto.c"

// the Fs operations besides read (decision 0102), compiled only into
// programs that call them (XO_FS, internal/build ioFlags)
#ifdef XO_FS
#include "xo_fs.c"
#endif

// standard input reads (decisions 0088, 0102), likewise (XO_STDIN)
#ifdef XO_STDIN
#include "xo_stdin.c"
#endif

// the test world of test binaries (decision 0083)
#include "xo_testing.c"

// Proc.run and FakeProc.on_run (decision 0107), compiled only into
// programs that call them (XO_PROC)
#ifdef XO_PROC
#include "xo_proc.c"
#endif

// std/time zones (decision 0094): only in programs that use std/time,
// which the build compiles with XO_TIME and their zone data (xo_tzdata.c)
#ifdef XO_TIME
#include "xo_time.c"
#endif
