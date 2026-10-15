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

// the fault of an index out of range on Bytes (decisions 0079, 0081)
#include "xo_bytes.c"

// HTTP/1.1 (std.md 4)
#include "xo_http.c"

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
