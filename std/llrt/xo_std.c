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

// xo_rs_append is s + t (consumes s; appended in place when unique):
// obj.xo's (zero-C audit B13), as the builders of xo_bytes.c.
int64_t xo_x_obj_rs_append(int64_t a, int64_t an, int64_t b, int64_t bn, int64_t lenp);
xo_rstr *xo_rs_append(xo_rstr *a, int64_t an, xo_rstr *b, int64_t bn, int64_t *len) {
	return (xo_rstr *)(intptr_t)xo_x_obj_rs_append(XO_ADDR(a), an, XO_ADDR(b), bn, XO_ADDR(len));
}

// ---------------------------------------------------------------------------
// Numbers

// parse_int is strconv.ParseInt(s, 10, 64): an optional sign and decimal
// digits, in range (cabi.xo).
static inline int n_parse_int(const char *p, int64_t n, int64_t *out) { return (int)xo_x_parse_int(XO_ADDR(p), n, XO_ADDR(out)); }

// ---------------------------------------------------------------------------
// Duration and Time (std.md 2.6)





// ---------------------------------------------------------------------------
// Conversions for std/json: durations, RFC 3339 times


// n_days_from_civil is the days since 1970-01-01 of a date.
static inline int64_t n_days_from_civil(int64_t y, int64_t m, int64_t d) { return xo_x_days_from_civil(y, m, d); }


// Sized integers, Rune, Bytes, builders, files (decisions 0079, 0081)
#include "xo_bytes.c"

// the faulting stubs of the Xo layers' wasm arms (decision 0206)
#include "xo_sys.c"

// HTTP/1.1 (std.md 4)
#include "xo_http.c"

// the Fs operations besides read (decision 0102), compiled only into
// programs that call them (XO_FS, internal/build ioFlags)
#ifdef XO_FS
#include "xo_fs.c"
#endif

// standard input reads (decisions 0088, 0102), likewise (XO_STDIN)
#ifdef XO_STDIN
#include "xo_stdin.c"
#endif

// Proc.run and FakeProc.on_run (decision 0107), compiled only into
// programs that call them (XO_PROC)
#ifdef XO_PROC
#include "xo_proc.c"
#endif

// std/time zones (decision 0094): only in programs that use std/time,
// which the build compiles with XO_TIME and their zone data (xo_tzdata.c).
// The zone data and the kept zones are Xo (std/native/llrt/tzhost.xo,
// work item B27); the leak check frees the kept zones through
// cabi_time.xo's export (xo_rt.c's end of main, under XO_TIME: no static
// constructor, which selfhost/macho's ELF link does not run).
