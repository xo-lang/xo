// xo_bytes.c: sized integers, Rune, Bytes, the builders, and files for the
// native backends (decisions 0079 and 0081), included by xo_std.c. The
// messages are std/rt's (arith.go, str.go, caps.go), so a fault reads the
// same on every backend.

// n_int_name is the Xo name of the integer type of bits and uns.
static const char *n_int_name(int64_t bits, int64_t uns) {
	switch (bits) {
	case 8:
		return uns ? "UInt8" : "Int8";
	case 16:
		return uns ? "UInt16" : "Int16";
	case 32:
		return uns ? "UInt32" : "Int32";
	}
	return uns ? "UInt64" : "Int";
}

// n_int_str is the decimal text of v, a value of the type of bits and uns.
static void n_int_str(char *buf, size_t n, int64_t v, int64_t bits, int64_t uns) {
	if (uns && bits == 64) {
		snprintf(buf, n, "%llu", (unsigned long long)v);
	} else {
		snprintf(buf, n, "%lld", (long long)v);
	}
}

// n_int_fits reports whether v (an exact result) is a value of the type
// of bits and uns (narrower than 64 bits).
static int n_int_fits(int64_t v, int64_t bits, int64_t uns) {
	if (bits >= 64) {
		return 1;
	}
	if (uns) {
		return v >= 0 && v < ((int64_t)1 << bits);
	}
	int64_t lim = (int64_t)1 << (bits - 1);
	return v >= -lim && v < lim;
}

XO_NORETURN static void n_int_overflow(int64_t a, const char *op, int64_t b, int64_t bits, int64_t uns, const char *loc, int64_t ln) {
	char x[32], y[32], m[160];
	n_int_str(x, sizeof x, a, bits, uns);
	n_int_str(y, sizeof y, b, bits, uns);
	snprintf(m, sizeof m, "integer overflow: %s %s %s does not fit %s", x, op, y, n_int_name(bits, uns));
	fault_at(m, loc, ln);
}

// xo_int_op is checked arithmetic on a sized integer type (std/rt's AddS,
// AddU, ..., Shl, Shr, NegS, NegU): op 0 +, 1 -, 2 *, 3 /, 4 %, 5 <<,
// 6 >>, 7 unary minus of a.
int64_t xo_int_op(int64_t op, int64_t a, int64_t b, int64_t bits, int64_t uns, const char *loc, int64_t ln) {
	static const char *ops[] = {"+", "-", "*", "/", "%", "<<", ">>"};
	char x[32], m[160];
	if (op == 7) {
		int bad = uns ? a != 0 : (bits == 64 ? a == INT64_MIN : !n_int_fits(-a, bits, 0));
		if (bad) {
			n_int_str(x, sizeof x, a, bits, uns);
			snprintf(m, sizeof m, "integer overflow: -(%s) does not fit %s", x, n_int_name(bits, uns));
			fault_at(m, loc, ln);
		}
		return -a;
	}
	if (op == 5 || op == 6) {
		if (b < 0 || b >= bits) {
			snprintf(m, sizeof m, "shift amount %lld out of range for %s", (long long)b, n_int_name(bits, uns));
			fault_at(m, loc, ln);
		}
		if (op == 6) {
			return uns ? (int64_t)((uint64_t)a >> b) : a >> b;
		}
		uint64_t mask = bits == 64 ? ~(uint64_t)0 : (((uint64_t)1 << bits) - 1);
		uint64_t c = ((uint64_t)a << b) & mask;
		int64_t r = (int64_t)c;
		if (!uns && bits < 64 && ((c >> (bits - 1)) & 1)) {
			r = (int64_t)(c | ~mask); // sign extend
		}
		int64_t back = uns ? (int64_t)((uint64_t)r >> b) : r >> b;
		if (back != a) {
			n_int_overflow(a, "<<", b, bits, uns, loc, ln);
		}
		return r;
	}
	if ((op == 3 || op == 4) && b == 0) {
		n_int_str(x, sizeof x, a, bits, uns);
		snprintf(m, sizeof m, "division by zero: %s %s 0", x, op == 3 ? "/" : "%");
		fault_at(m, loc, ln);
	}
	if (uns && bits == 64) {
		uint64_t ua = (uint64_t)a, ub = (uint64_t)b, r;
		switch (op) {
		case 0:
			if (__builtin_add_overflow(ua, ub, &r)) {
				n_int_overflow(a, "+", b, bits, uns, loc, ln);
			}
			return (int64_t)r;
		case 1:
			if (ub > ua) {
				n_int_overflow(a, "-", b, bits, uns, loc, ln);
			}
			return (int64_t)(ua - ub);
		case 2:
			if (__builtin_mul_overflow(ua, ub, &r)) {
				n_int_overflow(a, "*", b, bits, uns, loc, ln);
			}
			return (int64_t)r;
		case 3:
			return (int64_t)(ua / ub);
		default:
			return (int64_t)(ua % ub);
		}
	}
	// a narrow type: the exact result fits in 64 bits, except an unsigned
	// 32 bit product, which is computed unsigned
	int64_t r;
	switch (op) {
	case 0:
		r = a + b;
		break;
	case 1:
		r = a - b;
		break;
	case 2:
		if (uns) {
			uint64_t ur;
			if (__builtin_mul_overflow((uint64_t)a, (uint64_t)b, &ur) || ur > (((uint64_t)1 << bits) - 1)) {
				n_int_overflow(a, "*", b, bits, uns, loc, ln);
			}
			return (int64_t)ur;
		}
		r = a * b;
		break;
	case 3:
		r = a / b;
		break;
	default:
		return a % b;
	}
	if (!n_int_fits(r, bits, uns)) {
		n_int_overflow(a, ops[op], b, bits, uns, loc, ln);
	}
	return r;
}

// xo_int_conv converts v (of the type of fbits and funs) to the type of
// tbits and tuns, faulting out of range (std/rt's Conv).
int64_t xo_int_conv(int64_t v, int64_t fbits, int64_t funs, int64_t tbits, int64_t tuns, const char *loc, int64_t ln) {
	int ok;
	if (funs && fbits == 64 && v < 0) {
		// a UInt64 above the Int range fits only a UInt64
		ok = tuns && tbits == 64;
	} else if (tuns && tbits == 64) {
		ok = v >= 0;
	} else {
		ok = n_int_fits(v, tbits, tuns);
	}
	if (!ok) {
		char x[32], m[160];
		n_int_str(x, sizeof x, v, fbits, funs);
		snprintf(m, sizeof m, "%s is out of range for %s", x, n_int_name(tbits, tuns));
		fault_at(m, loc, ln);
	}
	return v;
}

static int n_rune_valid(int64_t r) { return r >= 0 && r <= 0x10FFFF && (r < 0xD800 || r > 0xDFFF); }

// xo_int_in_range reports whether the Int v is a value of the type of bits
// and uns (`Int32.try`).
int64_t xo_int_in_range(int64_t v, int64_t bits, int64_t uns) {
	if (uns && bits == 64) {
		return v >= 0;
	}
	return n_int_fits(v, (int)bits, (int)uns);
}

// xo_rune_valid reports whether v is a Unicode scalar value (`Rune.try`).
int64_t xo_rune_valid(int64_t v) { return n_rune_valid(v); }

// xo_rune_of is `Rune(n)` (decision 0079).
int64_t xo_rune_of(int64_t v, int64_t bits, int64_t uns, const char *loc, int64_t ln) {
	if ((uns && bits == 64 && v < 0) || !n_rune_valid(v)) {
		char x[32], m[160];
		n_int_str(x, sizeof x, v, bits, uns);
		snprintf(m, sizeof m, "%s is not a Unicode scalar value, so it is not a Rune", x);
		fault_at(m, loc, ln);
	}
	return v;
}

double xo_u64_to_float(int64_t v) { return (double)(uint64_t)v; }

// Float32 values are doubles holding float32 values (internal/ssa
// Module.Float32): conversions and arithmetic round through float.
double xo_f32_round(double v) { return (double)(float)v; }
double xo_i64_to_f32(int64_t v) { return (double)(float)v; }

// xo_rs_append_u64 is s + the text of a UInt64.
xo_rstr *xo_rs_append_u64(xo_rstr *a, int64_t an, int64_t v, int64_t *len) {
	char t[32];
	int n = snprintf(t, sizeof t, "%llu", (unsigned long long)v);
	*len = an + n;
	return rs_append(a, an, t, n);
}

// n_rune_utf8 writes the UTF-8 encoding of r (U+FFFD when it is not a
// scalar value) to p and returns its size (utf8.AppendRune).
static int n_rune_utf8(char *p, int64_t r) { return n_utf8_encode(p, n_rune_valid(r) ? (int32_t)r : 0xFFFD); }

// xo_rs_append_rune is s + the character r (its Display).
xo_rstr *xo_rs_append_rune(xo_rstr *a, int64_t an, int64_t r, int64_t *len) {
	char t[4];
	int n = n_rune_utf8(t, r);
	*len = an + n;
	return rs_append(a, an, t, n);
}

// xo_rs_append_rune_quote is s + the Debug text of r (std/rt quoteRune).
xo_rstr *xo_rs_append_rune_quote(xo_rstr *a, int64_t an, int64_t r, int64_t *len) {
	char t[8];
	int n = 0;
	t[n++] = '\'';
	switch (r) {
	case '\'':
	case '\\':
		t[n++] = '\\';
		t[n++] = (char)r;
		break;
	case '\n':
		t[n++] = '\\';
		t[n++] = 'n';
		break;
	case '\t':
		t[n++] = '\\';
		t[n++] = 't';
		break;
	default:
		n += n_rune_utf8(t + n, r);
	}
	t[n++] = '\'';
	*len = an + n;
	return rs_append(a, an, t, n);
}

// xo_rs_push_byte is a BytesBuilder's write_byte: s + the byte x.
xo_rstr *xo_rs_push_byte(xo_rstr *a, int64_t an, int64_t x, int64_t *len) {
	char c = (char)x;
	*len = an + 1;
	return rs_append(a, an, &c, 1);
}

// xo_rs_push_rune is a BytesBuilder's write_rune: s + the UTF-8 of r.
xo_rstr *xo_rs_push_rune(xo_rstr *a, int64_t an, int64_t r, int64_t *len) { return xo_rs_append_rune(a, an, r, len); }

// xo_rs_with_cap is an empty string with room for n bytes
// (BytesBuilder.with_capacity).
xo_rstr *xo_rs_with_cap(int64_t n, int64_t *len) {
	*len = 0;
	return rs_new(n < 16 ? 16 : n);
}

// n_slice_check faults unless lo..hi is a range of a value of length n
// (std/rt's StrSlice, BytesSlice).
static void n_slice_check(int64_t lo, int64_t hi, int64_t n, const char *what, const char *loc, int64_t ln) {
	if (lo < 0 || hi > n || lo > hi) {
		char m[160];
		snprintf(m, sizeof m, "slice %lld..%lld out of range for %s of length %lld", (long long)lo, (long long)hi, what, (long long)n);
		fault_at(m, loc, ln);
	}
}

// xo_bytes_slice_at is `b[lo..hi]` on Bytes.
xo_rstr *xo_bytes_slice_at(const char *p, int64_t n, int64_t lo, int64_t hi, const char *loc, int64_t ln, int64_t *len) {
	n_slice_check(lo, hi, n, "Bytes", loc, ln);
	return nstr(p + lo, hi - lo, len);
}

// xo_str_slice_at is `s[lo..hi]` on a Str: the offsets must fall on UTF-8
// boundaries.
xo_rstr *xo_str_slice_at(const char *p, int64_t n, int64_t lo, int64_t hi, const char *loc, int64_t ln, int64_t *len) {
	n_slice_check(lo, hi, n, "Str", loc, ln);
	const unsigned char *s = (const unsigned char *)p;
	if ((lo < n && (s[lo] & 0xC0) == 0x80) || (hi < n && (s[hi] & 0xC0) == 0x80)) {
		char m[160];
		snprintf(m, sizeof m, "slice %lld..%lld of Str splits a UTF-8 sequence", (long long)lo, (long long)hi);
		fault_at(m, loc, ln);
	}
	return nstr(p + lo, hi - lo, len);
}

// xo_rune_is_space is unicode.IsSpace (the rest of the Rune classes are
// Xo since decision 0180: std/native/llrt/unicode.xo).
int64_t xo_rune_is_space(int64_t r) { return n_is_space((int32_t)r); }

// xo_bytes_decode is utf8.DecodeRune of the bytes from i: the rune times 8
// plus its size (U+FFFD and 0 at the end).
int64_t xo_bytes_decode(const char *p, int64_t n, int64_t i) {
	if (i >= n) {
		return (int64_t)0xFFFD << 3;
	}
	int size;
	int32_t r = n_utf8_decode((const unsigned char *)p + i, n - i, &size);
	return ((int64_t)r << 3) | size;
}

// xo_bytes_rune_count is utf8.RuneCount (an invalid byte is one rune).
int64_t xo_bytes_rune_count(const char *p, int64_t n) { return xo_str_rune_count(p, n); }

// xo_bytes_hex is the lowercase hex of the bytes (Bytes.to_hex).
xo_rstr *xo_bytes_hex(const char *p, int64_t n, int64_t *len) {
	static const char hex[] = "0123456789abcdef";
	xo_rstr *s = rs_new(2 * n);
	for (int64_t i = 0; i < n; i++) {
		s->data[2 * i] = hex[(unsigned char)p[i] >> 4];
		s->data[2 * i + 1] = hex[(unsigned char)p[i] & 15];
	}
	*len = 2 * n;
	return s;
}

// xo_int_wrap is Int arithmetic that wraps like Go's int64 (op 0 +,
// 1 -, 2 *): Int.wrapping_add, wrapping_sub, wrapping_mul.
int64_t xo_int_wrap(int64_t op, int64_t a, int64_t b) {
	uint64_t x = (uint64_t)a, y = (uint64_t)b;
	switch (op) {
	case 0:
		return (int64_t)(x + y);
	case 1:
		return (int64_t)(x - y);
	}
	return (int64_t)(x * y);
}

// xo_int_fits reports whether a op b fits an Int (op 0 +, 1 -, 2 *,
// 3 /: b is not 0 and not MinInt / -1): the checked_ and saturating_
// methods.
int64_t xo_int_fits(int64_t op, int64_t a, int64_t b) {
	int64_t r;
	switch (op) {
	case 0:
		return !__builtin_add_overflow(a, b, &r);
	case 1:
		return !__builtin_sub_overflow(a, b, &r);
	case 2:
		return !__builtin_mul_overflow(a, b, &r);
	}
	return b != 0 && !(b == -1 && a == INT64_MIN);
}

static int n_hex_val(char c) {
	if (c >= '0' && c <= '9') {
		return c - '0';
	}
	if (c >= 'a' && c <= 'f') {
		return c - 'a' + 10;
	}
	if (c >= 'A' && c <= 'F') {
		return c - 'A' + 10;
	}
	return -1;
}

// xo_bytes_hex_ok reports whether s is hexadecimal that
// hex.DecodeString accepts (an even number of hex digits, either case).
int64_t xo_bytes_hex_ok(const char *p, int64_t n) {
	if (n % 2 != 0) {
		return 0;
	}
	for (int64_t i = 0; i < n; i++) {
		if (n_hex_val(p[i]) < 0) {
			return 0;
		}
	}
	return 1;
}

// xo_bytes_from_hex decodes hexadecimal that xo_bytes_hex_ok accepted
// (Bytes.from_hex).
xo_rstr *xo_bytes_from_hex(const char *p, int64_t n, int64_t *len) {
	xo_rstr *s = rs_new(n / 2);
	for (int64_t i = 0; i + 1 < n; i += 2) {
		s->data[i / 2] = (char)(n_hex_val(p[i]) << 4 | n_hex_val(p[i + 1]));
	}
	*len = n / 2;
	return s;
}

// The outcome of the last file operation of this thread (fs.read blocks
// the thread, and its status is read before the task can move): std/rt's
// FsErr tag (0 for success), and the error's path and message.
static __thread int64_t n_fs_status;
static __thread char *n_fs_path, *n_fs_err;

// xo_fs_read is `os.fs.read(path)`: the contents in a list of one, or the
// empty list with the status set (std/rt's mapFsErr).
xo_rlist *xo_fs_read(const char *p, int64_t n) {
	char *path = cstr_of(p, n);
	FILE *f = NULL;
	int err = 0;
	if (tw_is_fake(TW_REAL_FS_READ)) {
		// a test's MemFs starts empty (std.md 8.3)
		err = ENOENT;
	} else {
		f = fopen(n == 0 ? "." : path, "rb");
	}
	nbuf b = {0};
	if (!f && err == 0) {
		err = errno;
	} else if (f) {
		char t[65536];
		size_t k;
		while ((k = fread(t, 1, sizeof t, f)) > 0) {
			nbuf_put(&b, t, (int64_t)k);
		}
		if (ferror(f)) {
			err = errno ? errno : EIO;
		}
		fclose(f);
	}
	if (err != 0) {
		free(b.p);
		int64_t st = 7;
		switch (err) {
		case ENOENT:
			st = 1;
			break;
		case EEXIST:
			st = 2;
			break;
		case ENOTEMPTY:
			st = 3;
			break;
		case ENOTDIR:
			st = 4;
			break;
		case EISDIR:
			st = 5;
			break;
		case EACCES:
		case EPERM:
			st = 6;
			break;
		}
		n_fs_status = st;
		free(n_fs_path);
		free(n_fs_err);
		n_fs_path = path;
		n_fs_err = strdup(strerror(err));
		if (n_fs_err[0] >= 'A' && n_fs_err[0] <= 'Z') {
			n_fs_err[0] = (char)(n_fs_err[0] + 32);
		}
		return &empty_list;
	}
	free(path);
	n_fs_status = 0;
	xo_rlist *l = str_list(b.p ? b.p : "", b.n);
	free(b.p);
	return l;
}

int64_t xo_fs_status(void) { return n_fs_status; }

// xo_fs_msg is the path and the message of the last error.
xo_rlist *xo_fs_msg(void) {
	const char *p = n_fs_path ? n_fs_path : "", *m = n_fs_err ? n_fs_err : "";
	xo_rlist *l = n_str_list_new(2);
	l = n_str_list_add(l, p, (int64_t)strlen(p));
	return n_str_list_add(l, m, (int64_t)strlen(m));
}

// xo_env_args is `env.args()`: the arguments after the program name.
static xo_rlist *tw_env_args(void);

xo_rlist *xo_env_args(void) {
	if (tw_is_fake(TW_REAL_ENV)) {
		return tw_env_args();
	}
	xo_rlist *l = n_str_list_new(xo_argc);
	for (int i = 1; i < xo_argc; i++) {
		l = n_str_list_add(l, xo_argv[i], (int64_t)strlen(xo_argv[i]));
	}
	return l;
}

// xo_fault_bytes_index is `b[i]` out of range on Bytes (std/rt BytesAt).
XO_NORETURN void xo_fault_bytes_index(int64_t i, int64_t n, const char *loc) {
	char m[160];
	snprintf(m, sizeof m, "index %lld out of range for Bytes of length %lld", (long long)i, (long long)n);
	fault_text(m, loc);
}

// Constants of collection types (decision 0081): each is built once, by
// the entry function fn on first use, and kept immortal in its slot (id)
// until exit, when xo_consts_free gives it to drop (an entry function that
// releases a list) so the leak check sees it freed. Two threads may both
// build it; the loser releases its copy.
#define N_MAX_CONSTS 4096

static xo_rlist *n_const_lists[N_MAX_CONSTS];
static void (*n_const_drops[N_MAX_CONSTS])(xo_rlist *);

xo_rlist *xo_const_once(int64_t fn, int64_t drop, int64_t id) {
	xo_rlist *l = __atomic_load_n(&n_const_lists[id], __ATOMIC_ACQUIRE);
	if (l) {
		return l;
	}
	l = ((xo_rlist * (*)(void)) fn)();
	void (*d)(xo_rlist *) = (void (*)(xo_rlist *))drop;
	int64_t rc = l->rc;
	l->rc = XO_RC_IMMORTAL;
	xo_rlist *none = NULL;
	if (__atomic_compare_exchange_n(&n_const_lists[id], &none, l, 0, __ATOMIC_ACQ_REL, __ATOMIC_ACQUIRE)) {
		n_const_drops[id] = d;
		(void)rc;
		return l;
	}
	l->rc = 1;
	d(l);
	return none;
}

// xo_consts_free releases the constants built during the run.
void xo_consts_free(void) {
	for (int i = 0; i < N_MAX_CONSTS; i++) {
		xo_rlist *l = n_const_lists[i];
		if (l && n_const_drops[i]) {
			n_const_lists[i] = NULL;
			l->rc = 1;
			n_const_drops[i](l);
		}
	}
}
