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

// n_utf8_encode writes r (a valid rune) to p and returns its size
// (cabi.xo utf8_encode; decision 0207).
static inline int n_utf8_encode(char *p, int32_t r) { return (int)xo_x_utf8_encode(XO_ADDR(p), r); }

// ---------------------------------------------------------------------------
// Seed compatibility (zero-C audit end pass): the native prelude's
// llrt_str_slice and llrt_str_repeat (raw.xo) and obj.xo's rs_append
// export replace these in the code of this compiler (selfhost/ssa
// show.xo, selfhost/llvm and selfhost/arm64 call the exports); the seed
// (0.11.0) still calls them. Delete them when the seed has 5dc4de24 and
// this change (gotchas/runtime-c-the-seed-still-calls.md).

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

int64_t xo_x_obj_rs_append(int64_t a, int64_t an, int64_t b, int64_t bn, int64_t lenp);
xo_rstr *xo_rs_append(xo_rstr *a, int64_t an, xo_rstr *b, int64_t bn, int64_t *len) {
	return (xo_rstr *)(intptr_t)xo_x_obj_rs_append(XO_ADDR(a), an, XO_ADDR(b), bn, XO_ADDR(len));
}

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

// standard input reads are std/native/llrt/stdin.xo's on every OS (the
// zero-C audit's B32 part 2 removed xo_stdin.c; XO_STDIN, still passed
// by the driver for programs that read standard input, selects nothing)

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
