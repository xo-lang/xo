// xo_bytes.c: sized integers, Rune, Bytes, the builders, and files for the
// native backends (decisions 0079 and 0081), included by xo_std.c. The
// messages are std/rt's (arith.go, str.go, caps.go), so a fault reads the
// same on every backend.

static int n_rune_valid(int64_t r) { return r >= 0 && r <= 0x10FFFF && (r < 0xD800 || r > 0xDFFF); }

// The C versions of the intrinsics below and further down are compiled
// only with -DXO_INTS_C, which the drivers add when the generated code
// calls one. The flag kept its name when decision 0198 deleted the C
// path of ints (XO_LLRT_C=ints) it also guarded.
#ifdef XO_INTS_C
// the C versions of the conversions of UInt64 and Float32 (the native
// backends lower rt.u64_to_float, rt.f32_round, and rt.i64_to_f32
// inline, decision 0195; compiled only when called)
double xo_u64_to_float(int64_t v) { return (double)(uint64_t)v; }

// Float32 values are doubles holding float32 values (internal/ssa
// Module.Float32): conversions and arithmetic round through float.
double xo_f32_round(double v) { return (double)(float)v; }
double xo_i64_to_f32(int64_t v) { return (double)(float)v; }
#endif

// n_rune_utf8 writes the UTF-8 encoding of r (U+FFFD when it is not a
// scalar value) to p and returns its size (utf8.AppendRune).
static int n_rune_utf8(char *p, int64_t r) { return n_utf8_encode(p, n_rune_valid(r) ? (int32_t)r : 0xFFFD); }

// The builders are obj.xo's (zero-C audit B13), which arm64 generated
// code calls directly; LLVM generated code and the seed call these.
int64_t xo_x_obj_rs_push_byte(int64_t a, int64_t an, int64_t x, int64_t lenp);
int64_t xo_x_obj_rs_push_word(int64_t a, int64_t an, int64_t w, int64_t n, int64_t lenp);
int64_t xo_x_obj_rs_push_words(int64_t a, int64_t an, int64_t w0, int64_t w1, int64_t w2, int64_t w3, int64_t lens, int64_t lenp);
int64_t xo_x_obj_rs_append_bytes(int64_t a, int64_t an, int64_t b, int64_t bn, int64_t lo, int64_t hi, int64_t mode, int64_t lenp);
int64_t xo_x_obj_rs_with_cap(int64_t n, int64_t lenp);

// xo_rs_push_byte is a BytesBuilder's write_byte: s + the byte x.
xo_rstr *xo_rs_push_byte(xo_rstr *a, int64_t an, int64_t x, int64_t *len) {
	return (xo_rstr *)(intptr_t)xo_x_obj_rs_push_byte(XO_ADDR(a), an, x, XO_ADDR(len));
}

// xo_rs_push_word is s + the n (0 to 8) low bytes of w, the lowest
// first: short text assembled in an Int by Xo runtime code
// (std/native/llrt/text.xo, decision 0193).
xo_rstr *xo_rs_push_word(xo_rstr *a, int64_t an, int64_t w, int64_t n, int64_t *len) {
	return (xo_rstr *)(intptr_t)xo_x_obj_rs_push_word(XO_ADDR(a), an, w, n, XO_ADDR(len));
}

// xo_rs_push_words is s + four pieces of up to 8 bytes each, w0 to w3,
// their lengths in the nibbles of lens (decision 0193).
xo_rstr *xo_rs_push_words(xo_rstr *a, int64_t an, int64_t w0, int64_t w1, int64_t w2, int64_t w3, int64_t lens, int64_t *len) {
	return (xo_rstr *)(intptr_t)xo_x_obj_rs_push_words(XO_ADDR(a), an, w0, w1, w2, w3, lens, XO_ADDR(len));
}

// xo_rs_push_rune is a BytesBuilder's write_rune: s + the UTF-8 of r.
xo_rstr *xo_rs_push_rune(xo_rstr *a, int64_t an, int64_t r, int64_t *len) {
	char t[4];
	int n = n_rune_utf8(t, r);
	*len = an + n;
	return rs_append(a, an, t, n);
}

// xo_rs_append_bytes is s + the bytes lo..hi of b, their ASCII letters
// lowered (mode 1) or raised (mode 2) (std/native/llrt/unicode.xo,
// decision 0180).
xo_rstr *xo_rs_append_bytes(xo_rstr *a, int64_t an, xo_rstr *b, int64_t bn, int64_t lo, int64_t hi, int64_t mode, int64_t *len) {
	return (xo_rstr *)(intptr_t)xo_x_obj_rs_append_bytes(XO_ADDR(a), an, XO_ADDR(b), bn, lo, hi, mode, XO_ADDR(len));
}

// xo_rs_with_cap is an empty string with room for n bytes
// (BytesBuilder.with_capacity).
xo_rstr *xo_rs_with_cap(int64_t n, int64_t *len) { return (xo_rstr *)(intptr_t)xo_x_obj_rs_with_cap(n, XO_ADDR(len)); }

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

// The C versions of the runtime intrinsics (decisions 0193 and 0195):
// both native backends lower rt.int_wrap with a constant op,
// rt.u64_mul_hi, the bits of a Float, and rt.bytes_word inline, and
// Int's checked_ and saturating_ methods are std/native/llrt/ints.xo,
// so no default build calls these: they are compiled only with
// -DXO_INTS_C, which the drivers add when the generated code calls one
// (a non-constant op of int_wrap).
#ifdef XO_INTS_C
// xo_int_wrap is Int arithmetic that wraps like Go's int64 (op 0 +,
// 1 -, 2 *): Int.wrapping_add, wrapping_sub, wrapping_mul; and the
// shifts of a word by b mod 64 (op 3 left, 4 right logical) for the
// number text in Xo (std/native/llrt/num.xo, decision 0193). The native
// backends lower a constant op inline; this is the call otherwise.
int64_t xo_int_wrap(int64_t op, int64_t a, int64_t b) {
	uint64_t x = (uint64_t)a, y = (uint64_t)b;
	switch (op) {
	case 0:
		return (int64_t)(x + y);
	case 1:
		return (int64_t)(x - y);
	case 3:
		return (int64_t)(x << (y & 63));
	case 4:
		return (int64_t)(x >> (y & 63));
	case 5:
		// rotate right (decision 0195)
		return (int64_t)((x >> (y & 63)) | (x << ((64 - (y & 63)) & 63)));
	case 6: {
		// the low 32 bits rotated right, zero extended
		uint32_t w = (uint32_t)x;
		return (int64_t)(uint32_t)((w >> (y & 31)) | (w << ((32 - (y & 31)) & 31)));
	}
	case 7:
		return (int64_t)__builtin_bswap64(x);
	}
	return (int64_t)(x * y);
}

// xo_u64_mul_hi is the high word of the 128 bit product of a and b as
// unsigned words; xo_float_bits and xo_float_from_bits are a double's
// IEEE 754 bits and back (std/native/llrt/num.xo, decision 0193; the
// native backends lower them inline).
int64_t xo_u64_mul_hi(int64_t a, int64_t b) { return (int64_t)(((unsigned __int128)(uint64_t)a * (uint64_t)b) >> 64); }

int64_t xo_float_bits(double f) {
	int64_t b;
	memcpy(&b, &f, 8);
	return b;
}

// xo_bytes_word is the 8 bytes of p from i, little endian (the caller
// checked the range; the native backends lower it inline).
int64_t xo_bytes_word(const char *p, int64_t n, int64_t i) {
	(void)n;
	int64_t w;
	memcpy(&w, p + i, 8);
	return w;
}

double xo_float_from_bits(int64_t b) {
	double f;
	memcpy(&f, &b, 8);
	return f;
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
#endif

// xo_fault_bytes_index is `b[i]` out of range on Bytes (std/rt BytesAt).
// (A forwarder to cabi.xo's export for older xo's code, decision 0228.)
void xo_x_fault_raise_bytes_index(int64_t i, int64_t n, int64_t loc);
XO_NORETURN void xo_fault_bytes_index(int64_t i, int64_t n, const char *loc) {
	xo_x_fault_raise_bytes_index(i, n, XO_ADDR(loc));
	__builtin_unreachable();
}

// Constants of collection types (decision 0081) are consts.xo's (built
// once, immortal until exit); generated code and entry.xo's services
// record call these, which forward to cabi.xo's exports.
int64_t xo_x_const_once(int64_t fn, int64_t drop, int64_t id);
void xo_x_consts_free(void);

xo_rlist *xo_const_once(int64_t fn, int64_t drop, int64_t id) { return (xo_rlist *)(intptr_t)xo_x_const_once(fn, drop, id); }

void xo_consts_free(void) { xo_x_consts_free(); }
