// xo_json.c: JSON for the native std/json (decision 0074), included by
// xo_std.c.
//
// Parsing builds a tape, a List[Int] of nodes of three words each, that
// the decoders internal/ssa generates per type walk:
//
//	kind  0 null, 1 false, 2 true, 3 number, 4 string, 5 array, 6 object
//	a     number, string: offset of the text in the source (a string's
//	      after its opening quote); array: elements; object: members
//	b     number, string: length of the raw text (escapes included);
//	      array, object: the tape index after its last node
//
// An object's members are a key (a string node) and a value each, in the
// order of the text, duplicates included (the Go backend keeps the first
// position and the last value of a key: xo_json_members, xo_json_find).
//
// The parser reproduces what std/rt's jsonParse accepts and the text of
// the errors it reports: rt reads values with encoding/json's
// Decoder.Token, whose state machine (stream.go) decides most errors, and
// whose scanner (scanner.go) reads each scalar. internal/llvm's
// TestJSONParseDiff compares the two on a corpus and random inputs.

enum {
	JT_TOP,
	JT_ARRAY_START,
	JT_ARRAY_VALUE,
	JT_ARRAY_COMMA,
	JT_OBJECT_START,
	JT_OBJECT_KEY,
	JT_OBJECT_COLON,
	JT_OBJECT_VALUE,
	JT_OBJECT_COMMA,
};

enum { JK_NULL, JK_FALSE, JK_TRUE, JK_NUM, JK_STR, JK_ARR, JK_OBJ };

typedef struct {
	const unsigned char *s;
	int64_t n, p;
	int state;
	int *stack;
	int64_t depth, scap;
	int64_t *tape;
	int64_t tn, tcap;
	char err[160];
	int failed;
} jparser;

static int j_space(unsigned c) { return c == ' ' || c == '\t' || c == '\r' || c == '\n'; }

// j_quote_char is encoding/json's quoteChar: the byte as a rune in single
// quotes, escaped like strconv.Quote.
static void j_quote_char(char *out, size_t n, unsigned c) {
	if (c == '\'') {
		snprintf(out, n, "'\\''");
		return;
	}
	if (c == '"') {
		snprintf(out, n, "'\"'");
		return;
	}
	switch (c) {
	case '\a':
		snprintf(out, n, "'\\a'");
		return;
	case '\b':
		snprintf(out, n, "'\\b'");
		return;
	case '\f':
		snprintf(out, n, "'\\f'");
		return;
	case '\n':
		snprintf(out, n, "'\\n'");
		return;
	case '\r':
		snprintf(out, n, "'\\r'");
		return;
	case '\t':
		snprintf(out, n, "'\\t'");
		return;
	case '\v':
		snprintf(out, n, "'\\v'");
		return;
	case '\\':
		snprintf(out, n, "'\\\\'");
		return;
	}
	if (c < 0x20 || c == 0x7f) {
		snprintf(out, n, "'\\x%02x'", c);
		return;
	}
	if (c < 0x80) {
		snprintf(out, n, "'%c'", c);
		return;
	}
	// a byte >= 0x80 is the rune U+0080..U+00FF: strconv.IsPrint is false
	// for the C1 controls, U+00A0, and U+00AD
	if (c < 0xA0 || c == 0xA0 || c == 0xAD) {
		snprintf(out, n, "'\\u%04x'", c);
		return;
	}
	snprintf(out, n, "'%c%c'", (char)(0xC0 | (c >> 6)), (char)(0x80 | (c & 0x3F)));
}

static int j_fail(jparser *jp, const char *msg) {
	if (!jp->failed) {
		snprintf(jp->err, sizeof jp->err, "%s", msg);
		jp->failed = 1;
	}
	return -1;
}

// j_fail_char reports "invalid character 'c' <context>".
static int j_fail_char(jparser *jp, unsigned c, const char *context) {
	char q[16];
	j_quote_char(q, sizeof q, c);
	char m[160];
	if (context[0]) {
		snprintf(m, sizeof m, "invalid character %s %s", q, context);
	} else {
		snprintf(m, sizeof m, "invalid character %s", q);
	}
	return j_fail(jp, m);
}

static void j_push_state(jparser *jp, int st) {
	if (jp->depth == jp->scap) {
		jp->scap = jp->scap ? jp->scap * 2 : 32;
		jp->stack = realloc(jp->stack, (size_t)jp->scap * sizeof(int));
		if (!jp->stack) {
			xo_oom();
		}
	}
	jp->stack[jp->depth++] = st;
}

static int64_t j_node(jparser *jp, int64_t kind, int64_t a, int64_t b) {
	if (jp->tn + 3 > jp->tcap) {
		jp->tcap = jp->tcap ? jp->tcap * 2 : 48;
		jp->tape = realloc(jp->tape, (size_t)jp->tcap * sizeof(int64_t));
		if (!jp->tape) {
			xo_oom();
		}
	}
	int64_t i = jp->tn;
	jp->tape[i] = kind;
	jp->tape[i + 1] = a;
	jp->tape[i + 2] = b;
	jp->tn += 3;
	return i;
}

// j_peek is Decoder.peek: the next non-space byte, or -1 at the end.
static int j_peek(jparser *jp) {
	while (jp->p < jp->n && j_space(jp->s[jp->p])) {
		jp->p++;
	}
	return jp->p < jp->n ? jp->s[jp->p] : -1;
}

static int j_value_allowed(jparser *jp) {
	switch (jp->state) {
	case JT_TOP:
	case JT_ARRAY_START:
	case JT_ARRAY_VALUE:
	case JT_OBJECT_VALUE:
		return 1;
	}
	return 0;
}

static void j_value_end(jparser *jp) {
	switch (jp->state) {
	case JT_ARRAY_START:
	case JT_ARRAY_VALUE:
		jp->state = JT_ARRAY_COMMA;
		break;
	case JT_OBJECT_VALUE:
		jp->state = JT_OBJECT_COMMA;
		break;
	}
}

// j_token_error is Decoder.tokenError.
static int j_token_error(jparser *jp, unsigned c) {
	const char *ctx = "";
	switch (jp->state) {
	case JT_TOP:
	case JT_ARRAY_START:
	case JT_ARRAY_VALUE:
	case JT_OBJECT_VALUE:
		ctx = "looking for beginning of value";
		break;
	case JT_ARRAY_COMMA:
		ctx = "after array element";
		break;
	case JT_OBJECT_KEY:
		ctx = "looking for beginning of object key string";
		break;
	case JT_OBJECT_COLON:
		ctx = "after object key";
		break;
	case JT_OBJECT_COMMA:
		ctx = "after object key:value pair";
		break;
	}
	return j_fail_char(jp, c, ctx);
}

static int j_hex(unsigned c) { return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F'); }

// j_scalar is Decoder.Decode of one scalar starting at jp->p: encoding/
// json's scanner from stateBeginValue up to the end of the value (the
// byte after it, or the end of the input). It appends the node.
static int j_scalar(jparser *jp) {
	const unsigned char *s = jp->s;
	int64_t n = jp->n, i = jp->p, start = i;
	unsigned c = s[i];
	if (c == '"') {
		i++;
		for (;;) {
			if (i >= n) {
				return j_fail(jp, "unexpected EOF");
			}
			c = s[i];
			if (c == '"') {
				break;
			}
			if (c == '\\') {
				i++;
				if (i >= n) {
					return j_fail(jp, "unexpected EOF");
				}
				c = s[i];
				switch (c) {
				case 'b':
				case 'f':
				case 'n':
				case 'r':
				case 't':
				case '\\':
				case '/':
				case '"':
					i++;
					continue;
				case 'u':
					i++;
					for (int k = 0; k < 4; k++) {
						if (i >= n) {
							return j_fail(jp, "unexpected EOF");
						}
						if (!j_hex(s[i])) {
							return j_fail_char(jp, s[i], "in \\u hexadecimal character escape");
						}
						i++;
					}
					continue;
				}
				return j_fail_char(jp, c, "in string escape code");
			}
			if (c < 0x20) {
				return j_fail_char(jp, c, "in string literal");
			}
			i++;
		}
		j_node(jp, JK_STR, start + 1, i - start - 1);
		jp->p = i + 1;
		return 0;
	}
	if (c == 't' || c == 'f' || c == 'n') {
		const char *lit = c == 't' ? "true" : c == 'f' ? "false" : "null";
		const char *what = c == 't' ? "true" : c == 'f' ? "false" : "null";
		int64_t L = (int64_t)strlen(lit);
		for (int64_t k = 1; k < L; k++) {
			if (i + k >= n) {
				return j_fail(jp, "unexpected EOF");
			}
			if (s[i + k] != (unsigned char)lit[k]) {
				char ctx[64];
				snprintf(ctx, sizeof ctx, "in literal %s (expecting '%c')", what, lit[k]);
				return j_fail_char(jp, s[i + k], ctx);
			}
		}
		j_node(jp, c == 't' ? JK_TRUE : c == 'f' ? JK_FALSE : JK_NULL, start, L);
		jp->p = i + L;
		return 0;
	}
	// a number: - (0 | [1-9][0-9]*) (. [0-9]+)? ([eE] [+-]? [0-9]+)?
	if (c == '-') {
		i++;
		if (i >= n) {
			return j_fail(jp, "unexpected EOF");
		}
		c = s[i];
		if (!(c >= '0' && c <= '9')) {
			return j_fail_char(jp, c, "in numeric literal");
		}
	} else if (!(c >= '0' && c <= '9')) {
		return j_fail_char(jp, c, "looking for beginning of value");
	}
	if (s[i] == '0') {
		i++;
	} else {
		while (i < n && s[i] >= '0' && s[i] <= '9') {
			i++;
		}
	}
	if (i < n && s[i] == '.') {
		i++;
		if (i >= n) {
			return j_fail(jp, "unexpected EOF");
		}
		if (!(s[i] >= '0' && s[i] <= '9')) {
			return j_fail_char(jp, s[i], "after decimal point in numeric literal");
		}
		while (i < n && s[i] >= '0' && s[i] <= '9') {
			i++;
		}
	}
	if (i < n && (s[i] == 'e' || s[i] == 'E')) {
		i++;
		if (i < n && (s[i] == '+' || s[i] == '-')) {
			i++;
		}
		if (i >= n) {
			return j_fail(jp, "unexpected EOF");
		}
		if (!(s[i] >= '0' && s[i] <= '9')) {
			return j_fail_char(jp, s[i], "in exponent of numeric literal");
		}
		while (i < n && s[i] >= '0' && s[i] <= '9') {
			i++;
		}
	}
	j_node(jp, JK_NUM, start, i - start);
	jp->p = i;
	return 0;
}

// Token kinds j_token returns.
enum { TK_ERR = -1, TK_ARR_OPEN, TK_ARR_CLOSE, TK_OBJ_OPEN, TK_OBJ_CLOSE, TK_SCALAR, TK_KEY, TK_EOF };

// j_token is Decoder.Token. A scalar or key is appended to the tape.
static int j_token(jparser *jp) {
	for (;;) {
		int c = j_peek(jp);
		if (c < 0) {
			j_fail(jp, "EOF");
			return TK_EOF;
		}
		switch (c) {
		case '[':
			if (!j_value_allowed(jp)) {
				j_token_error(jp, (unsigned)c);
				return TK_ERR;
			}
			jp->p++;
			j_push_state(jp, jp->state);
			jp->state = JT_ARRAY_START;
			return TK_ARR_OPEN;
		case ']':
			if (jp->state != JT_ARRAY_START && jp->state != JT_ARRAY_COMMA) {
				j_token_error(jp, (unsigned)c);
				return TK_ERR;
			}
			jp->p++;
			jp->state = jp->stack[--jp->depth];
			j_value_end(jp);
			return TK_ARR_CLOSE;
		case '{':
			if (!j_value_allowed(jp)) {
				j_token_error(jp, (unsigned)c);
				return TK_ERR;
			}
			jp->p++;
			j_push_state(jp, jp->state);
			jp->state = JT_OBJECT_START;
			return TK_OBJ_OPEN;
		case '}':
			if (jp->state != JT_OBJECT_START && jp->state != JT_OBJECT_COMMA) {
				j_token_error(jp, (unsigned)c);
				return TK_ERR;
			}
			jp->p++;
			jp->state = jp->stack[--jp->depth];
			j_value_end(jp);
			return TK_OBJ_CLOSE;
		case ':':
			if (jp->state != JT_OBJECT_COLON) {
				j_token_error(jp, (unsigned)c);
				return TK_ERR;
			}
			jp->p++;
			jp->state = JT_OBJECT_VALUE;
			continue;
		case ',':
			if (jp->state == JT_ARRAY_COMMA) {
				jp->p++;
				jp->state = JT_ARRAY_VALUE;
				continue;
			}
			if (jp->state == JT_OBJECT_COMMA) {
				jp->p++;
				jp->state = JT_OBJECT_KEY;
				continue;
			}
			j_token_error(jp, (unsigned)c);
			return TK_ERR;
		case '"':
			if (jp->state == JT_OBJECT_START || jp->state == JT_OBJECT_KEY) {
				if (j_scalar(jp) < 0) {
					return TK_ERR;
				}
				jp->state = JT_OBJECT_COLON;
				return TK_KEY;
			}
			// fallthrough
		default:
			if (!j_value_allowed(jp)) {
				j_token_error(jp, (unsigned)c);
				return TK_ERR;
			}
			if (j_scalar(jp) < 0) {
				return TK_ERR;
			}
			j_value_end(jp);
			return TK_SCALAR;
		}
	}
}

// j_more is Decoder.More.
static int j_more(jparser *jp) {
	int c = j_peek(jp);
	return c >= 0 && c != ']' && c != '}';
}

// j_parse is std/rt's jsonValue over the whole input, then its check
// for trailing data: 0 and the tape, or -1 and the error text.
static int j_parse(jparser *jp) {
	// frames of open arrays and objects: their tape index
	int64_t *open = NULL;
	int64_t no = 0, ocap = 0;
	int t = j_token(jp);
	for (;;) {
		if (t == TK_ERR || t == TK_EOF) {
			free(open);
			return -1;
		}
		int64_t done = -1; // a value completed at this tape index
		if (t == TK_ARR_OPEN || t == TK_OBJ_OPEN) {
			int64_t i = j_node(jp, t == TK_ARR_OPEN ? JK_ARR : JK_OBJ, 0, 0);
			if (no == ocap) {
				ocap = ocap ? ocap * 2 : 16;
				open = realloc(open, (size_t)ocap * sizeof(int64_t));
				if (!open) {
					xo_oom();
				}
			}
			open[no++] = i;
		} else if (t == TK_SCALAR) {
			done = jp->tn - 3;
		}
		// close the containers that end here, and read the next value
		for (;;) {
			if (done >= 0) {
				if (no == 0) {
					// the top level value: rt rejects a following token
					// ("trailing data") but not an error or the end
					free(open);
					int64_t keep = jp->tn;
					int st = jp->state;
					int t2 = j_token(jp);
					jp->tn = keep;
					jp->state = st;
					if (t2 != TK_ERR && t2 != TK_EOF) {
						jp->failed = 0;
						j_fail(jp, "trailing data");
						return -1;
					}
					jp->failed = 0;
					jp->err[0] = 0;
					return 0;
				}
				jp->tape[open[no - 1] + 1]++;
			}
			int64_t top = open[no - 1];
			if (j_more(jp)) {
				if (jp->tape[top] == JK_OBJ) {
					int k = j_token(jp);
					if (k == TK_ERR || k == TK_EOF) {
						free(open);
						return -1;
					}
				}
				break;
			}
			// the closing delimiter (or an error)
			int k = j_token(jp);
			if (k == TK_ERR || k == TK_EOF) {
				free(open);
				return -1;
			}
			no--;
			jp->tape[top + 2] = jp->tn;
			done = top;
		}
		t = j_token(jp);
	}
}

// j_new parses n bytes at p.
static void j_init(jparser *jp, const char *p, int64_t n) {
	memset(jp, 0, sizeof *jp);
	jp->s = (const unsigned char *)p;
	jp->n = n;
	jp->state = JT_TOP;
}

static void j_free(jparser *jp) {
	free(jp->stack);
	free(jp->tape);
}

// xo_json_parse parses the text at p: the tape as a List[Int], or the
// empty list when it is not valid (xo_json_parse_err tells why).
xo_rlist *xo_json_parse(const char *p, int64_t n) {
	jparser jp;
	j_init(&jp, p, n);
	if (j_parse(&jp) < 0) {
		j_free(&jp);
		return xo_rl_new(0, 8);
	}
	xo_rlist *l = xo_rl_new(jp.tn, 8);
	memcpy(rl_data(l), jp.tape, (size_t)jp.tn * 8);
	l->len = jp.tn;
	j_free(&jp);
	return l;
}

// xo_json_parse_err is the error of parsing the text at p (the text after
// "invalid JSON: "), or "" when it is valid.
xo_rstr *xo_json_parse_err(const char *p, int64_t n, int64_t *len) {
	jparser jp;
	j_init(&jp, p, n);
	int r = j_parse(&jp);
	xo_rstr *s = r < 0 ? nstr(jp.err, (int64_t)strlen(jp.err), len) : nstr("", 0, len);
	j_free(&jp);
	return s;
}

static int64_t tape_at(xo_rlist *t, int64_t i) { return ((int64_t *)rl_data(t))[i]; }

// j_next is the tape index after the value at i.
static int64_t j_next(xo_rlist *t, int64_t i) {
	int64_t k = tape_at(t, i);
	return k == JK_ARR || k == JK_OBJ ? tape_at(t, i + 2) : i + 3;
}

int64_t xo_json_next(xo_rlist *t, int64_t i) { return j_next(t, i); }

// j_unquote appends the value of the raw string text (between the
// quotes) to b, as encoding/json's unquote: escapes decoded, surrogate
// pairs combined, a lone surrogate and invalid UTF-8 become U+FFFD.
static void j_unquote(nbuf *b, const unsigned char *s, int64_t n) {
	for (int64_t i = 0; i < n;) {
		unsigned c = s[i];
		if (c == '\\') {
			i++;
			c = s[i];
			switch (c) {
			case 'b':
				nbuf_byte(b, '\b');
				i++;
				continue;
			case 'f':
				nbuf_byte(b, '\f');
				i++;
				continue;
			case 'n':
				nbuf_byte(b, '\n');
				i++;
				continue;
			case 'r':
				nbuf_byte(b, '\r');
				i++;
				continue;
			case 't':
				nbuf_byte(b, '\t');
				i++;
				continue;
			case 'u': {
				int32_t r = 0;
				for (int k = 1; k <= 4; k++) {
					unsigned h = s[i + k];
					r = r * 16 + (int32_t)(h <= '9' ? h - '0' : (h | 0x20) - 'a' + 10);
				}
				i += 5;
				if (r >= 0xD800 && r < 0xDC00) {
					// a high surrogate: a low one must follow
					if (i + 6 <= n && s[i] == '\\' && s[i + 1] == 'u') {
						int32_t r2 = 0;
						int ok = 1;
						for (int k = 2; k <= 5; k++) {
							unsigned h = s[i + k];
							if (!j_hex(h)) {
								ok = 0;
								break;
							}
							r2 = r2 * 16 + (int32_t)(h <= '9' ? h - '0' : (h | 0x20) - 'a' + 10);
						}
						if (ok && r2 >= 0xDC00 && r2 < 0xE000) {
							r = 0x10000 + ((r - 0xD800) << 10) + (r2 - 0xDC00);
							i += 6;
						} else {
							r = 0xFFFD;
						}
					} else {
						r = 0xFFFD;
					}
				} else if (r >= 0xDC00 && r < 0xE000) {
					r = 0xFFFD;
				}
				char t[4];
				nbuf_put(b, t, n_utf8_encode(t, r));
				continue;
			}
			default: // \\ \/ \"
				nbuf_byte(b, (char)c);
				i++;
				continue;
			}
		}
		if (c < 0x80) {
			nbuf_byte(b, (char)c);
			i++;
			continue;
		}
		int size;
		int32_t r = n_utf8_decode(s + i, n - i, &size);
		if (r == 0xFFFD && size == 1) {
			nbuf_put(b, "\xEF\xBF\xBD", 3);
		} else {
			nbuf_put(b, (const char *)s + i, size);
		}
		i += size;
	}
}

// j_plain reports whether a raw string needs no unquoting (no escapes,
// valid UTF-8).
static int j_plain(const unsigned char *s, int64_t n) {
	for (int64_t i = 0; i < n; i++) {
		if (s[i] == '\\' || s[i] >= 0x80) {
			return memchr(s + i, '\\', (size_t)(n - i)) == NULL && xo_utf8_valid((const char *)s + i, n - i);
		}
	}
	return 1;
}

// xo_json_str is the value of the string node at i.
xo_rstr *xo_json_str(const char *src, int64_t sn, xo_rlist *t, int64_t i, int64_t *len) {
	(void)sn;
	const unsigned char *s = (const unsigned char *)src + tape_at(t, i + 1);
	int64_t n = tape_at(t, i + 2);
	if (j_plain(s, n)) {
		return nstr((const char *)s, n, len);
	}
	nbuf b = {0};
	j_unquote(&b, s, n);
	return nbuf_take(&b, len);
}

// j_key_eq reports whether the string node at i is key.
static int j_key_eq(const char *src, xo_rlist *t, int64_t i, const char *key, int64_t kn) {
	const unsigned char *s = (const unsigned char *)src + tape_at(t, i + 1);
	int64_t n = tape_at(t, i + 2);
	if (j_plain(s, n)) {
		return n == kn && memcmp(s, key, (size_t)n) == 0;
	}
	nbuf b = {0};
	j_unquote(&b, s, n);
	int eq = b.n == kn && memcmp(b.p, key, (size_t)kn) == 0;
	free(b.p);
	return eq;
}

// xo_json_find is the tape index of the value of the last member of the
// object at i whose key is key, or -1 (std/rt keeps the last value).
int64_t xo_json_find(const char *src, int64_t sn, xo_rlist *t, int64_t i, const char *key, int64_t kn) {
	(void)sn;
	int64_t m = tape_at(t, i + 1), j = i + 3, found = -1;
	for (int64_t k = 0; k < m; k++) {
		int64_t v = j + 3;
		if (j_key_eq(src, t, j, key, kn)) {
			found = v;
		}
		j = j_next(t, v);
	}
	return found;
}

// xo_json_members lists the members of the object at i as std/rt's
// jsonObj has them: each key once, at its first position, with its last
// value: (key node, value node) pairs of tape indexes.
xo_rlist *xo_json_members(const char *src, int64_t sn, xo_rlist *t, int64_t i) {
	(void)sn;
	int64_t m = tape_at(t, i + 1), j = i + 3;
	xo_rlist *l = xo_rl_new(m * 2 < 4 ? 4 : m * 2, 8);
	int64_t *o = (int64_t *)rl_data(l);
	int64_t cnt = 0;
	for (int64_t k = 0; k < m; k++) {
		int64_t v = j + 3;
		// an earlier member with the same key takes this value
		int64_t dup = -1;
		const unsigned char *ks = (const unsigned char *)src + tape_at(t, j + 1);
		int64_t kl = tape_at(t, j + 2);
		nbuf kb = {0};
		const char *kp = (const char *)ks;
		int64_t kn = kl;
		if (!j_plain(ks, kl)) {
			j_unquote(&kb, ks, kl);
			kp = kb.p;
			kn = kb.n;
		}
		for (int64_t e = 0; e < cnt && dup < 0; e++) {
			if (j_key_eq(src, t, o[2 * e], kp, kn)) {
				dup = e;
			}
		}
		free(kb.p);
		if (dup >= 0) {
			o[2 * dup + 1] = v;
		} else {
			o[2 * cnt] = j;
			o[2 * cnt + 1] = v;
			cnt++;
		}
		j = j_next(t, v);
	}
	l->len = cnt * 2;
	return l;
}

// xo_json_int_ok reports whether the number node at i is an Int
// (strconv.ParseInt of its text).
int64_t xo_json_int_ok(const char *src, int64_t sn, xo_rlist *t, int64_t i) {
	(void)sn;
	int64_t v;
	return n_parse_int(src + tape_at(t, i + 1), tape_at(t, i + 2), &v);
}

int64_t xo_json_int(const char *src, int64_t sn, xo_rlist *t, int64_t i) {
	(void)sn;
	int64_t v = 0;
	n_parse_int(src + tape_at(t, i + 1), tape_at(t, i + 2), &v);
	return v;
}

int64_t xo_json_float_ok(const char *src, int64_t sn, xo_rlist *t, int64_t i) {
	(void)sn;
	double v;
	return n_parse_float(src + tape_at(t, i + 1), tape_at(t, i + 2), &v);
}

double xo_json_float(const char *src, int64_t sn, xo_rlist *t, int64_t i) {
	(void)sn;
	double v = 0;
	n_parse_float(src + tape_at(t, i + 1), tape_at(t, i + 2), &v);
	return v;
}

// ---------------------------------------------------------------------------
// Encoding

static const char j_hexdig[] = "0123456789abcdef";

// xo_rs_append_json_str is s + t as a JSON string: std/rt's
// appendJSONString (encoding/json without HTML escaping).
xo_rstr *xo_rs_append_json_str(xo_rstr *a, int64_t an, xo_rstr *b, int64_t bn, int64_t *len) {
	const unsigned char *s = (const unsigned char *)b->data;
	int64_t extra = 0;
	for (int64_t i = 0; i < bn; i++) {
		unsigned c = s[i];
		if (c < 0x20 || c == '"' || c == '\\' || c >= 0x80) {
			extra = -1;
			break;
		}
	}
	if (extra == 0) {
		// the common case: nothing to escape
		a = rs_append(a, an, "\"", 1);
		a = rs_append(a, an + 1, (const char *)s, bn);
		a = rs_append(a, an + 1 + bn, "\"", 1);
		*len = an + bn + 2;
		return a;
	}
	nbuf o = {0};
	nbuf_byte(&o, '"');
	int64_t start = 0;
	for (int64_t i = 0; i < bn;) {
		unsigned c = s[i];
		if (c < 0x80) {
			if (c >= 0x20 && c != '"' && c != '\\') {
				i++;
				continue;
			}
			nbuf_put(&o, (const char *)s + start, i - start);
			switch (c) {
			case '\\':
			case '"': {
				char e[2] = {'\\', (char)c};
				nbuf_put(&o, e, 2);
				break;
			}
			case '\b':
				nbuf_put(&o, "\\b", 2);
				break;
			case '\f':
				nbuf_put(&o, "\\f", 2);
				break;
			case '\n':
				nbuf_put(&o, "\\n", 2);
				break;
			case '\r':
				nbuf_put(&o, "\\r", 2);
				break;
			case '\t':
				nbuf_put(&o, "\\t", 2);
				break;
			default: {
				char e[6] = {'\\', 'u', '0', '0', j_hexdig[c >> 4], j_hexdig[c & 0xF]};
				nbuf_put(&o, e, 6);
			}
			}
			i++;
			start = i;
			continue;
		}
		int size;
		int32_t r = n_utf8_decode(s + i, bn - i, &size);
		if (r == 0xFFFD && size == 1) {
			nbuf_put(&o, (const char *)s + start, i - start);
			nbuf_put(&o, "\\ufffd", 6);
			i += size;
			start = i;
			continue;
		}
		if (r == 0x2028 || r == 0x2029) {
			nbuf_put(&o, (const char *)s + start, i - start);
			char e[6] = {'\\', 'u', '2', '0', '2', j_hexdig[r & 0xF]};
			nbuf_put(&o, e, 6);
			i += size;
			start = i;
			continue;
		}
		i += size;
	}
	nbuf_put(&o, (const char *)s + start, bn - start);
	nbuf_byte(&o, '"');
	*len = an + o.n;
	xo_rstr *res = rs_append(a, an, o.p, o.n);
	free(o.p);
	return res;
}

// n_shortest writes the shortest decimal digits of f (finite, not 0)
// that read back as f, and the decimal exponent: f = 0.digits * 10^dp.
static int n_shortest(double f, char *digs, int *dp) {
	char e[40];
	int p;
	for (p = 0; p < 17; p++) {
		snprintf(e, sizeof e, "%.*e", p, fabs(f));
		if (strtod(e, NULL) == fabs(f)) {
			break;
		}
	}
	int nd = 0;
	char *q = e;
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
	*dp = x + 1;
	return nd;
}

// n_json_float writes a Float as encoding/json does: NaN and infinities
// as null (std/rt's jsonFloat), 'f' format, or 'e' below 1e-6 and from
// 1e21 with a one digit exponent kept short (1e-7, not 1e-07).
static int n_json_float(char *out, double f) {
	if (isnan(f) || isinf(f)) {
		memcpy(out, "null", 5);
		return 4;
	}
	char *o = out;
	if (f == 0) {
		if (signbit(f)) {
			*o++ = '-';
		}
		*o++ = '0';
		*o = 0;
		return (int)(o - out);
	}
	char digs[32];
	int dp;
	int nd = n_shortest(f, digs, &dp);
	if (f < 0) {
		*o++ = '-';
	}
	double a = fabs(f);
	if (a < 1e-6 || a >= 1e21) {
		*o++ = digs[0];
		if (nd > 1) {
			*o++ = '.';
			for (int i = 1; i < nd; i++) {
				*o++ = digs[i];
			}
		}
		*o++ = 'e';
		int ex = dp - 1;
		if (ex < 0) {
			*o++ = '-';
			ex = -ex;
		} else {
			*o++ = '+';
		}
		o += snprintf(o, 8, "%d", ex);
		*o = 0;
		return (int)(o - out);
	}
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
	return (int)(o - out);
}

xo_rstr *xo_rs_append_json_float(xo_rstr *a, int64_t an, double f, int64_t *len) {
	char b[48];
	int k = n_json_float(b, f);
	*len = an + k;
	return rs_append(a, an, b, k);
}
