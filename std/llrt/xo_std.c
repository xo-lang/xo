// xo_std.c: the intrinsics of the native std implementation (decision
// 0074), included at the end of xo_rt.c. std/native/rt.xo declares them;
// internal/ssa lowers a call of `rt.name` there to xo_name here. A Str
// operand arrives as its bytes and length (an rs_* function gets the
// string object and consumes its first one); a Str result is a new
// string object, its length stored through the last parameter.
//
// Semantics follow std/rt (Go's strings, strconv, and time packages), so
// the native backends print what the Go backend prints.

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

// The UTF-8 helpers of the C code are Xo (std/native/llrt/cabi.xo,
// decision 0207): utf8_decode is utf8.DecodeRune of the n > 0 bytes at
// p, an invalid or short sequence U+FFFD of size 1.
static inline int32_t n_utf8_decode(const unsigned char *p, int64_t n, int *size) {
	int64_t x = xo_x_utf8_decode(XO_ADDR(p), n);
	*size = (int)(x & 7);
	return (int32_t)(x >> 3);
}

// utf8_encode writes r (a valid rune) to p and returns its size.
static inline int n_utf8_encode(char *p, int32_t r) { return (int)xo_x_utf8_encode(XO_ADDR(p), r); }

// is_space is Go's unicode.IsSpace.
static inline int n_is_space(int32_t r) { return (int)xo_x_is_space(r); }

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
// digits, in range (cabi.xo).
static inline int n_parse_int(const char *p, int64_t n, int64_t *out) { return (int)xo_x_parse_int(XO_ADDR(p), n, XO_ADDR(out)); }

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

// ---------------------------------------------------------------------------
// Duration and Time (std.md 2.6)

#ifdef XO_TEXT_C
// the C copy for the C path of text (XO_LLRT_C=text, decision 0193);
// the runtime's own callers use cabi.xo's fmt_duration
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

#endif

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


#ifdef XO_TEXT_C
// the C copy for the C path of text (XO_LLRT_C=text); the runtime's own
// callers use cabi.xo's fmt_time
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

#endif

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
// Conversions for std/json: durations, RFC 3339 times

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
static inline int64_t n_days_from_civil(int64_t y, int64_t m, int64_t d) { return xo_x_days_from_civil(y, m, d); }

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

// the buffers of the Linux system call layer in Xo (decision 0206)
#include "xo_sys.c"

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
