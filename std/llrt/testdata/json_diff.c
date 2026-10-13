// json_diff.c: the native JSON parser (xo_json.c) on inputs from
// standard input, for internal/llvm's TestJSONParseDiff, which compares
// its output with std/rt's jsonParse (encoding/json's Decoder.Token).
//
// Input: records of an 8 byte little endian length and the bytes.
// Output, one line per record: "E <error>" or "O <dump>", where the dump
// is the parsed value: null, true, false, numbers as their text, strings
// as s<hex of the value>, arrays [a,b], objects {<key hex>:v,...} with
// each key once at its first position and its last value.

#define XO_RT_NO_MAIN
// the C path of std/json's text (decision 0193 step 4)
#define XO_JSONTEXT_C
#include "../xo_rt.c"

static void hex(nbuf *b, const char *p, int64_t n) {
	for (int64_t i = 0; i < n; i++) {
		char h[3];
		snprintf(h, sizeof h, "%02x", (unsigned char)p[i]);
		nbuf_put(b, h, 2);
	}
}

static void dump(nbuf *b, const char *src, int64_t sn, xo_rlist *t, int64_t i) {
	int64_t *tp = (int64_t *)rl_data(t);
	switch (tp[i]) {
	case JK_NULL:
		nbuf_str(b, "null");
		return;
	case JK_FALSE:
		nbuf_str(b, "false");
		return;
	case JK_TRUE:
		nbuf_str(b, "true");
		return;
	case JK_NUM:
		nbuf_put(b, src + tp[i + 1], tp[i + 2]);
		return;
	case JK_STR: {
		int64_t n;
		xo_rstr *s = xo_json_str(src, sn, t, i, &n);
		nbuf_byte(b, 's');
		hex(b, s->data, n);
		xo_free(s);
		return;
	}
	case JK_ARR: {
		nbuf_byte(b, '[');
		int64_t j = i + 3;
		for (int64_t k = 0; k < tp[i + 1]; k++) {
			if (k > 0) {
				nbuf_byte(b, ',');
			}
			dump(b, src, sn, t, j);
			j = xo_json_next(t, j);
		}
		nbuf_byte(b, ']');
		return;
	}
	case JK_OBJ: {
		nbuf_byte(b, '{');
		xo_rlist *m = xo_json_members(src, sn, t, i);
		int64_t *mp = (int64_t *)rl_data(m);
		for (int64_t k = 0; k < m->len; k += 2) {
			if (k > 0) {
				nbuf_byte(b, ',');
			}
			int64_t n;
			xo_rstr *s = xo_json_str(src, sn, t, mp[k], &n);
			hex(b, s->data, n);
			xo_free(s);
			nbuf_byte(b, ':');
			dump(b, src, sn, t, mp[k + 1]);
		}
		xo_free(m);
		nbuf_byte(b, '}');
		return;
	}
	}
}

int main(void) {
	for (;;) {
		unsigned char lb[8];
		if (fread(lb, 1, 8, stdin) != 8) {
			break;
		}
		int64_t n = 0;
		for (int k = 7; k >= 0; k--) {
			n = n * 256 + lb[k];
		}
		char *src = malloc((size_t)n + 1);
		if (n > 0 && fread(src, 1, (size_t)n, stdin) != (size_t)n) {
			return 2;
		}
		xo_rlist *t = xo_json_parse(src, n);
		nbuf b = {0};
		if (t->len == 0) {
			int64_t en;
			xo_rstr *e = xo_json_parse_err(src, n, &en);
			nbuf_str(&b, "E ");
			nbuf_put(&b, e->data, en);
			xo_free(e);
		} else {
			nbuf_str(&b, "O ");
			dump(&b, src, n, t, 0);
		}
		xo_free(t);
		nbuf_byte(&b, '\n');
		fwrite(b.p, 1, (size_t)b.n, stdout);
		free(b.p);
		free(src);
	}
	return 0;
}
