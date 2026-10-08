// xo_rt.c: the runtime of the P3 LLVM backend prototype (internal/llvm,
// `xo build --backend=llvm`). It is compiled by clang together with the
// generated LLVM IR (with -flto, so the small helpers inline into Xo code).
//
// Contents:
//   - memory: a bump allocator over 4 MiB chunks with no free. Every
//     allocation lives until the process exits. Fine for short programs and
//     benchmarks; a long running program needs reference counting or a GC
//     (see bench/results/llvm-prototype-2026-10-08.md).
//   - lists: a List[T] value is a pointer to a header {len, cap, shared}
//     followed by the elements. `shared` is a sticky flag set when a list
//     value is copied (a binding, a field, a capture): a mutation through a
//     binding whose buffer is shared copies the buffer first (copy on
//     write), so value semantics hold without reference counts. Without
//     decrements a buffer once shared stays shared, so the first mutation
//     after every copy clones.
//   - strings: a Str is (ptr, len), immutable; builders for interpolation.
//   - output: buffered stdout, flushed at exit and before a fault.
//   - faults: messages and exit status 70, matching the Go backend's text.
//   - main: runs the Xo entry on a thread with a 1 GiB stack, like Go's
//     maximum goroutine stack, so deep recursion behaves alike.
//
// The interface to generated code uses only scalars and pointers (no
// structs by value), so it does not depend on the C ABI of aggregates.

#include <math.h>
#include <pthread.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#define XO_NORETURN __attribute__((noreturn, noinline, cold))

// ---------------------------------------------------------------------------
// Memory

#define XO_CHUNK (4 << 20)

static char *bump_cur;
static char *bump_end;

__attribute__((noinline)) static void *xo_alloc_slow(int64_t n) {
	if (n > XO_CHUNK / 4) {
		void *p = malloc((size_t)n);
		if (!p) {
			fputs("fault: out of memory\n", stderr);
			exit(70);
		}
		return p;
	}
	char *c = malloc(XO_CHUNK);
	if (!c) {
		fputs("fault: out of memory\n", stderr);
		exit(70);
	}
	bump_cur = c + n;
	bump_end = c + XO_CHUNK;
	return c;
}

void *xo_alloc(int64_t n) {
	n = (n + 15) & ~(int64_t)15;
	if (bump_end - bump_cur < n) {
		return xo_alloc_slow(n);
	}
	void *p = bump_cur;
	bump_cur += n;
	return p;
}

// ---------------------------------------------------------------------------
// Output

static char obuf[1 << 16];
static size_t olen;

static void write_all(int fd, const char *p, size_t n) {
	while (n > 0) {
		ssize_t w = write(fd, p, n);
		if (w <= 0) {
			return;
		}
		p += w;
		n -= (size_t)w;
	}
}

void xo_flush(void) {
	if (olen > 0) {
		write_all(1, obuf, olen);
		olen = 0;
	}
}

void xo_print(const char *p, int64_t n) {
	if ((size_t)n > sizeof obuf / 2) {
		xo_flush();
		write_all(1, p, (size_t)n);
		return;
	}
	if (olen + (size_t)n > sizeof obuf) {
		xo_flush();
	}
	memcpy(obuf + olen, p, (size_t)n);
	olen += (size_t)n;
}

void xo_println(const char *p, int64_t n) {
	xo_print(p, n);
	xo_print("\n", 1);
}

void xo_eprint(const char *p, int64_t n) {
	xo_flush();
	write_all(2, p, (size_t)n);
}

void xo_eprintln(const char *p, int64_t n) {
	xo_eprint(p, n);
	write_all(2, "\n", 1);
}

// ---------------------------------------------------------------------------
// Number formatting (the Go backend's text: strconv.FormatInt and
// FloatStr, the shortest 'g' form that reads back the same value, with
// ".0" added to whole numbers)

static int fmt_int(char *buf, int64_t v) {
	return snprintf(buf, 32, "%lld", (long long)v);
}

// fmt_float writes Go's strconv.FormatFloat(f, 'g', -1, 64) to buf.
static int fmt_go_g(char *buf, double f) {
	if (isnan(f)) {
		return snprintf(buf, 40, "NaN");
	}
	if (isinf(f)) {
		return snprintf(buf, 40, f > 0 ? "+Inf" : "-Inf");
	}
	if (f == 0) {
		return snprintf(buf, 40, signbit(f) ? "-0" : "0");
	}
	// shortest digits that round trip
	char e[40];
	int p;
	for (p = 0; p < 17; p++) {
		snprintf(e, sizeof e, "%.*e", p, f);
		if (strtod(e, NULL) == f) {
			break;
		}
	}
	// e is [-]d[.ddd]e(+|-)XX
	int neg = e[0] == '-';
	char *s = e + neg;
	char digs[24];
	int nd = 0;
	char *q = s;
	for (; *q && *q != 'e'; q++) {
		if (*q >= '0' && *q <= '9') {
			digs[nd++] = *q;
		}
	}
	int x = atoi(q + 1);
	while (nd > 1 && digs[nd - 1] == '0') {
		nd--;
	}
	digs[nd] = 0;
	int dp = x + 1; // decimal point position: value = 0.digs * 10^dp
	int exp = dp - 1;
	char *o = buf;
	if (neg) {
		*o++ = '-';
	}
	if (exp < -4 || exp >= 6) {
		*o++ = digs[0];
		if (nd > 1) {
			*o++ = '.';
			for (int i = 1; i < nd; i++) {
				*o++ = digs[i];
			}
		}
		*o++ = 'e';
		int ax = exp;
		if (ax < 0) {
			*o++ = '-';
			ax = -ax;
		} else {
			*o++ = '+';
		}
		o += snprintf(o, 8, "%02d", ax);
		*o = 0;
		return (int)(o - buf);
	}
	// %f with max(nd - dp, 0) decimals
	if (dp <= 0) {
		*o++ = '0';
		*o++ = '.';
		for (int i = 0; i < -dp; i++) {
			*o++ = '0';
		}
		for (int i = 0; i < nd; i++) {
			*o++ = digs[i];
		}
	} else {
		for (int i = 0; i < dp; i++) {
			*o++ = i < nd ? digs[i] : '0';
		}
		if (nd > dp) {
			*o++ = '.';
			for (int i = dp; i < nd; i++) {
				*o++ = digs[i];
			}
		}
	}
	*o = 0;
	return (int)(o - buf);
}

static int fmt_float(char *buf, double f) {
	int n = fmt_go_g(buf, f);
	if (!strpbrk(buf, ".eIN")) {
		buf[n++] = '.';
		buf[n++] = '0';
		buf[n] = 0;
	}
	return n;
}

// ---------------------------------------------------------------------------
// Faults (core 8): the same messages as std/rt, exit status 70

XO_NORETURN static void fault_text(const char *msg, const char *loc) {
	xo_flush();
	char *buf = malloc(strlen(msg) + strlen(loc) + 32);
	int n = sprintf(buf, "fault: %s at %s\n", msg, loc);
	write_all(2, buf, (size_t)n);
	exit(70);
}

XO_NORETURN void xo_fault(const char *msg, int64_t n, const char *loc) {
	char *m = malloc((size_t)n + 1);
	memcpy(m, msg, (size_t)n);
	m[n] = 0;
	fault_text(m, loc);
}

XO_NORETURN void xo_fault_overflow(int64_t a, int64_t op, int64_t b, const char *loc) {
	char m[160];
	snprintf(m, sizeof m, "integer overflow: %lld %c %lld does not fit Int", (long long)a, (char)op, (long long)b);
	fault_text(m, loc);
}

XO_NORETURN void xo_fault_shl(int64_t a, int64_t b, const char *loc) {
	char m[160];
	snprintf(m, sizeof m, "integer overflow: %lld << %lld does not fit Int", (long long)a, (long long)b);
	fault_text(m, loc);
}

XO_NORETURN void xo_fault_shift(int64_t n, const char *loc) {
	char m[160];
	snprintf(m, sizeof m, "shift amount %lld out of range for Int", (long long)n);
	fault_text(m, loc);
}

XO_NORETURN void xo_fault_divzero(int64_t a, int64_t op, const char *loc) {
	char m[160];
	snprintf(m, sizeof m, "division by zero: %lld %c 0", (long long)a, (char)op);
	fault_text(m, loc);
}

XO_NORETURN void xo_fault_neg(int64_t a, const char *loc) {
	char m[160];
	snprintf(m, sizeof m, "integer overflow: -(%lld) does not fit Int", (long long)a);
	fault_text(m, loc);
}

XO_NORETURN void xo_fault_abs(int64_t a, const char *loc) {
	char m[160];
	snprintf(m, sizeof m, "integer overflow: abs(%lld) does not fit Int", (long long)a);
	fault_text(m, loc);
}

XO_NORETURN void xo_fault_index(int64_t i, int64_t n, const char *loc) {
	char m[160];
	snprintf(m, sizeof m, "index %lld out of range for List of length %lld", (long long)i, (long long)n);
	fault_text(m, loc);
}

XO_NORETURN void xo_fault_float_int(double f, const char *loc) {
	char m[160];
	if (isnan(f)) {
		snprintf(m, sizeof m, "cannot convert NaN to Int");
	} else {
		char b[40];
		fmt_go_g(b, f);
		snprintf(m, sizeof m, "%s is out of range for Int", b);
	}
	fault_text(m, loc);
}

XO_NORETURN void xo_fault_unreachable(const char *loc) {
	fault_text("unreachable code reached (internal compiler error)", loc);
}

// ---------------------------------------------------------------------------
// Strings

typedef struct {
	char *p;
	int64_t len, cap;
} xo_sb;

xo_sb *xo_sb_new(void) {
	xo_sb *b = xo_alloc(sizeof(xo_sb));
	b->cap = 64;
	b->p = xo_alloc(b->cap);
	b->len = 0;
	return b;
}

static void sb_grow(xo_sb *b, int64_t n) {
	if (b->len + n <= b->cap) {
		return;
	}
	int64_t c = b->cap * 2;
	while (c < b->len + n) {
		c *= 2;
	}
	char *p = xo_alloc(c);
	memcpy(p, b->p, (size_t)b->len);
	b->p = p;
	b->cap = c;
}

void xo_sb_str(xo_sb *b, const char *p, int64_t n) {
	sb_grow(b, n);
	memcpy(b->p + b->len, p, (size_t)n);
	b->len += n;
}

void xo_sb_int(xo_sb *b, int64_t v) {
	char t[32];
	int n = fmt_int(t, v);
	xo_sb_str(b, t, n);
}

void xo_sb_float(xo_sb *b, double v) {
	char t[48];
	int n = fmt_float(t, v);
	xo_sb_str(b, t, n);
}

void xo_sb_bool(xo_sb *b, int32_t v) {
	if (v) {
		xo_sb_str(b, "true", 4);
	} else {
		xo_sb_str(b, "false", 5);
	}
}

const char *xo_sb_done(xo_sb *b, int64_t *len) {
	*len = b->len;
	return b->p;
}

const char *xo_str_concat(const char *a, int64_t an, const char *b, int64_t bn) {
	char *p = xo_alloc(an + bn + 1);
	memcpy(p, a, (size_t)an);
	memcpy(p + an, b, (size_t)bn);
	return p;
}

int32_t xo_str_eq(const char *a, int64_t an, const char *b, int64_t bn) {
	return an == bn && (a == b || memcmp(a, b, (size_t)an) == 0);
}

int64_t xo_str_cmp(const char *a, int64_t an, const char *b, int64_t bn) {
	int64_t n = an < bn ? an : bn;
	int c = memcmp(a, b, (size_t)n);
	if (c != 0) {
		return c < 0 ? -1 : 1;
	}
	return an < bn ? -1 : (an > bn ? 1 : 0);
}

// ---------------------------------------------------------------------------
// Lists

typedef struct {
	int64_t len, cap, shared;
} xo_list;

xo_list xo_list_empty = {0, 0, 1};

typedef void (*xo_retain_fn)(void *);

static inline char *list_data(xo_list *l) { return (char *)(l + 1); }

// list_copy returns an unshared copy of l with room for cap elements.
// When l is shared its elements are now held by two buffers, so each is
// retained (its own lists marked shared).
__attribute__((noinline)) static xo_list *list_copy(xo_list *l, int64_t esize, int64_t cap, xo_retain_fn retain) {
	if (cap < 4) {
		cap = 4;
	}
	xo_list *n = xo_alloc((int64_t)sizeof(xo_list) + cap * esize);
	n->len = l->len;
	n->cap = cap;
	n->shared = 0;
	memcpy(list_data(n), list_data(l), (size_t)(l->len * esize));
	if (l->shared && retain) {
		for (int64_t i = 0; i < l->len; i++) {
			retain(list_data(n) + i * esize);
		}
	}
	return n;
}

xo_list *xo_list_new(int64_t n, int64_t esize) {
	int64_t cap = n < 4 ? 4 : n;
	xo_list *l = xo_alloc((int64_t)sizeof(xo_list) + cap * esize);
	l->len = n;
	l->cap = cap;
	l->shared = 0;
	return l;
}

// xo_list_unshare returns an unshared copy of a shared list. Generated
// code calls it on the slow path of a write: `if (l->shared) l =
// xo_list_unshare(l, ...)`, then stores the result back in the binding.
__attribute__((noinline)) xo_list *xo_list_unshare(xo_list *l, int64_t esize, xo_retain_fn retain) {
	return list_copy(l, esize, l->len, retain);
}

// xo_list_grow returns an unshared list with room for one more element
// (the slow path of push: the list is shared or full).
__attribute__((noinline)) xo_list *xo_list_grow(xo_list *l, int64_t esize, xo_retain_fn retain) {
	int64_t cap = l->len == l->cap ? l->cap * 2 : l->cap;
	return list_copy(l, esize, cap, retain);
}

// ---------------------------------------------------------------------------
// Entry

extern void xo_entry(void);

static void *run_main(void *arg) {
	(void)arg;
	xo_entry();
	return NULL;
}

int main(int argc, char **argv) {
	(void)argc;
	(void)argv;
	pthread_attr_t attr;
	pthread_attr_init(&attr);
	pthread_attr_setstacksize(&attr, (size_t)1 << 30);
	pthread_t t;
	if (pthread_create(&t, &attr, run_main, NULL) != 0) {
		run_main(NULL);
	} else {
		pthread_join(t, NULL);
	}
	xo_flush();
	return 0;
}
