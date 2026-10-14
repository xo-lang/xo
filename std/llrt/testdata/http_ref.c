// http_ref.c: the C HTTP/1.1 request parser of std/llrt/xo_http.c
// (hp_head, hb_chunked, and the net/url request target parser under
// them), deleted from the runtime when the client's response reading
// moved to Xo (std/native/llrt/http1.xo, audit item B10, 2026-10-14).
// The runtime's parsing is Xo's since decision 0198; these copies are
// test data: http_diff.c and http_fuzz.c include them after xo_rt.c to
// play http1.xo's part (xo_hc_read stops at every head and chunked
// body). No runtime build compiles this file.

// Limits: Go's DefaultMaxHeaderBytes plus the 4096 bytes of slack its
// server allows reads the request line and headers; more is 431.
#define HTTP_MAX_HEAD ((int64_t)(1 << 20) + 4096)
// Header fields per request (Go has no count limit: its bytes limit
// bounds them); more is 431.
#define HTTP_MAX_FIELDS 10000

static int h_vbyte(unsigned c) { return c >= 0x80 || (c >= 0x21 && c <= 0x7e) || c == ' ' || c == '\t'; }

// ---------------------------------------------------------------------------
// Request targets (net/url ParseRequestURI) and queries (ParseQuery)

static int h_ishex(unsigned c) { return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F'); }

static int h_unhex(unsigned c) { return c <= '9' ? (int)(c - '0') : (int)((c | 0x20) - 'a' + 10); }

// host mode (url.shouldEscape for encodeHost): the ASCII bytes allowed
static int h_host_ok(unsigned c) {
	if ((c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
		return 1;
	}
	return strchr("!$&'()*+,;=:[]<>\"-_.~", (int)c) != NULL && c != 0;
}

enum { U_PATH, U_QUERY, U_HOST };

// h_unescape is url.unescape for the path, a query component, or a
// host: 0 on an invalid escape.
static int h_unescape(const char *s, int64_t n, int mode, nbuf *out) {
	for (int64_t i = 0; i < n;) {
		unsigned c = (unsigned char)s[i];
		if (c == '%') {
			if (i + 2 >= n || !h_ishex((unsigned char)s[i + 1]) || !h_ishex((unsigned char)s[i + 2])) {
				return 0;
			}
			if (mode == U_HOST && h_unhex((unsigned char)s[i + 1]) < 8 && !(s[i + 1] == '2' && s[i + 2] == '5')) {
				return 0;
			}
			nbuf_byte(out, (char)(h_unhex((unsigned char)s[i + 1]) << 4 | h_unhex((unsigned char)s[i + 2])));
			i += 3;
			continue;
		}
		if (c == '+' && mode == U_QUERY) {
			nbuf_byte(out, ' ');
		} else {
			if (mode == U_HOST && c < 0x80 && !h_host_ok(c)) {
				return 0;
			}
			nbuf_byte(out, (char)c);
		}
		i++;
	}
	return 1;
}

// h_parse_host is url.parseHost (without the IPv6 zone and address
// checks beyond inet_pton).
static int h_parse_host(const char *h, int64_t n, hstr *out) {
	int64_t ob = -1;
	for (int64_t i = n - 1; i >= 0; i--) {
		if (h[i] == '[') {
			ob = i;
			break;
		}
	}
	if (ob > 0) {
		return 0;
	}
	nbuf b = {0};
	if (ob == 0) {
		int64_t cb = -1;
		for (int64_t i = n - 1; i >= 0; i--) {
			if (h[i] == ']') {
				cb = i;
				break;
			}
		}
		if (cb < 0 || !h_valid_port(h + cb + 1, n - cb - 1)) {
			return 0;
		}
		nbuf hb = {0};
		if (!h_unescape(h + 1, cb - 1, U_HOST, &hb)) {
			free(hb.p);
			return 0;
		}
		nbuf_byte(&hb, 0);
		struct in6_addr a6;
		char *zone = strchr(hb.p, '%');
		if (zone) {
			*zone = 0;
		}
		int ok = nx_pton6(hb.p, &a6) == 1;
		free(hb.p);
		if (!ok) {
			return 0;
		}
		if (!h_unescape(h, n, U_HOST, &b)) {
			free(b.p);
			return 0;
		}
	} else {
		const char *colon = memchr(h, ':', (size_t)n);
		if (colon) {
			int64_t last = n - 1;
			while (last >= 0 && h[last] != ':') {
				last--;
			}
			int64_t i = colon - h;
			if (last != i) {
				// strict colons (Go 1.26): only one colon outside brackets
				return 0;
			}
			if (!h_valid_port(h + i, n - i)) {
				return 0;
			}
		}
		if (!h_unescape(h, n, U_HOST, &b)) {
			free(b.p);
			return 0;
		}
	}
	*out = hs_dup(b.p ? b.p : "", b.n);
	free(b.p);
	return 1;
}

// h_valid_userinfo is the userinfo check of url.parseAuthority (after
// the host parsed): validUserinfo's bytes, then unescape's check of the
// % escapes. 0 with Go's reason in err when it fails.
static int h_valid_userinfo(const char *s, int64_t n, char *err, size_t en) {
	for (int64_t i = 0; i < n; i++) {
		unsigned c = (unsigned char)s[i];
		if ((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || (c != 0 && strchr("-._:~!$&'()*+,;=%@", (int)c))) {
			continue;
		}
		snprintf(err, en, "net/url: invalid userinfo");
		return 0;
	}
	for (int64_t i = 0; i < n; i++) {
		if (s[i] != '%') {
			continue;
		}
		int64_t left = n - i;
		if (left < 3 || !h_ishex((unsigned char)s[i + 1]) || !h_ishex((unsigned char)s[i + 2])) {
			snprintf(err, en, "invalid URL escape \"%.*s\"", (int)(left < 3 ? left : 3), s + i);
			return 0;
		}
		i += 2;
	}
	return 1;
}

// h_url_parse is net/url's parse: ParseRequestURI when via_request,
// else Parse after the fragment is cut. 0 when it fails, with Go's
// reason in err (without the `parse "url": ` prefix).
static int h_url_parse(const char *s, int64_t sn, int via_request, hurl *u, char *err, size_t en) {
	memset(u, 0, sizeof *u);
	snprintf(err, en, "invalid URL");
	for (int64_t i = 0; i < sn; i++) {
		unsigned c = (unsigned char)s[i];
		if (c < ' ' || c == 0x7f) {
			snprintf(err, en, "net/url: invalid control character in URL");
			return 0;
		}
	}
	if (sn == 0 && via_request) {
		snprintf(err, en, "empty url");
		return 0;
	}
	if (sn == 1 && s[0] == '*') {
		u->path = hs_dup("*", 1);
		u->scheme = hs_dup("", 0);
		u->host = hs_dup("", 0);
		u->rawquery = hs_dup("", 0);
		return 1;
	}
	// getScheme
	int64_t rest = 0;
	hstr scheme = hs_dup("", 0);
	for (int64_t i = 0; i < sn; i++) {
		unsigned c = (unsigned char)s[i];
		if ((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			continue;
		}
		if ((c >= '0' && c <= '9') || c == '+' || c == '-' || c == '.') {
			if (i == 0) {
				break;
			}
			continue;
		}
		if (c == ':') {
			if (i == 0) {
				hs_free(&scheme);
				snprintf(err, en, "missing protocol scheme");
				return 0;
			}
			hs_free(&scheme);
			scheme = hs_dup(s, i);
			for (int64_t k = 0; k < i; k++) {
				scheme.p[k] = (char)h_lower(scheme.p[k]);
			}
			rest = i + 1;
		}
		break;
	}
	const char *r = s + rest;
	int64_t rn = sn - rest;
	// a single trailing "?" is ForceQuery with no RawQuery
	int64_t q = -1, qcount = 0;
	for (int64_t i = 0; i < rn; i++) {
		if (r[i] == '?') {
			if (q < 0) {
				q = i;
			}
			qcount++;
		}
	}
	if (rn > 0 && r[rn - 1] == '?' && qcount == 1) {
		u->rawquery = hs_dup("", 0);
		rn--;
	} else if (q >= 0) {
		u->rawquery = hs_dup(r + q + 1, rn - q - 1);
		u->has_query = 1;
		rn = q;
	} else {
		u->rawquery = hs_dup("", 0);
	}
	if (!(rn > 0 && r[0] == '/')) {
		if (scheme.n > 0) {
			// opaque: no host, no path
			u->scheme = scheme;
			u->host = hs_dup("", 0);
			u->path = hs_dup("", 0);
			return 1;
		}
		if (via_request) {
			hs_free(&scheme);
			hs_free(&u->rawquery);
			snprintf(err, en, "invalid URI for request");
			return 0;
		}
		const char *sl = memchr(r, '/', (size_t)rn);
		int64_t segn = sl ? sl - r : rn;
		if (memchr(r, ':', (size_t)segn)) {
			hs_free(&scheme);
			hs_free(&u->rawquery);
			snprintf(err, en, "first path segment in URL cannot contain colon");
			return 0;
		}
	}
	u->scheme = scheme;
	int authority = rn >= 2 && r[0] == '/' && r[1] == '/' && (scheme.n > 0 || (!via_request && !(rn >= 3 && r[2] == '/')));
	if (authority) {
		const char *auth = r + 2;
		int64_t an = rn - 2;
		const char *slash = memchr(auth, '/', (size_t)an);
		int64_t al = slash ? slash - auth : an;
		r = auth + al;
		rn = an - al;
		int64_t at = -1;
		for (int64_t i = al - 1; i >= 0; i--) {
			if (auth[i] == '@') {
				at = i;
				break;
			}
		}
		const char *hp = at >= 0 ? auth + at + 1 : auth;
		int64_t hn = at >= 0 ? al - at - 1 : al;
		if (!h_parse_host(hp, hn, &u->host)) {
			hu_free(u);
			char hq[160];
			snprintf(hq, sizeof hq, "%.*s", (int)(hn < 120 ? hn : 120), hp);
			snprintf(err, en, "invalid host \"%s\"", hq);
			return 0;
		}
		if (at >= 0 && !h_valid_userinfo(auth, at, err, en)) {
			hu_free(u);
			return 0;
		}
	} else {
		u->host = hs_dup("", 0);
	}
	nbuf pb = {0};
	if (!h_unescape(r, rn, U_PATH, &pb)) {
		free(pb.p);
		// the bad escape: %, and up to two bytes after it
		const char *pc = memchr(r, '%', (size_t)rn);
		for (; pc; pc = memchr(pc + 1, '%', (size_t)(rn - (pc + 1 - r)))) {
			int64_t left = rn - (pc - r);
			if (left < 3 || !h_ishex((unsigned char)pc[1]) || !h_ishex((unsigned char)pc[2])) {
				snprintf(err, en, "invalid URL escape \"%.*s\"", (int)(left < 3 ? left : 3), pc);
				break;
			}
		}
		hu_free(u);
		return 0;
	}
	u->path = hs_dup(pb.p ? pb.p : "", pb.n);
	free(pb.p);
	return 1;
}

// h_parse_target is url.ParseRequestURI of a request target: 0 when it
// fails (400). A CONNECT target that is not a path is an authority.
static int h_parse_target(const char *raw, int64_t n, int connect, hurl *u) {
	char err[160];
	if (connect && !(n > 0 && raw[0] == '/')) {
		nbuf t = {0};
		nbuf_put(&t, "http://", 7);
		nbuf_put(&t, raw, n);
		int ok = h_url_parse(t.p, t.n, 1, u, err, sizeof err);
		free(t.p);
		if (ok) {
			hs_free(&u->scheme);
			u->scheme = hs_dup("", 0);
		}
		return ok;
	}
	return h_url_parse(raw, n, 1, u, err, sizeof err);
}

// h_parse_query is std/rt's urlOf over url.ParseQuery: each key once
// with its first value, keys sorted.
static void h_parse_query(const char *q, int64_t n, hlist *out) {
	int64_t amps = 0;
	for (int64_t i = 0; i < n; i++) {
		amps += q[i] == '&';
	}
	if (amps + 1 > 10000) {
		return; // number of URL query parameters exceeded limit
	}
	int64_t i = 0;
	while (i < n) {
		int64_t j = i;
		while (j < n && q[j] != '&') {
			j++;
		}
		const char *kv = q + i;
		int64_t kn = j - i;
		i = j + 1;
		if (memchr(kv, ';', (size_t)kn) || kn == 0) {
			continue;
		}
		const char *eq = memchr(kv, '=', (size_t)kn);
		int64_t kl = eq ? eq - kv : kn;
		nbuf kb = {0}, vb = {0};
		if (!h_unescape(kv, kl, U_QUERY, &kb) || (eq && !h_unescape(eq + 1, kn - kl - 1, U_QUERY, &vb))) {
			free(kb.p);
			free(vb.p);
			continue;
		}
		hstr k = hs_dup(kb.p ? kb.p : "", kb.n), v = hs_dup(vb.p ? vb.p : "", vb.n);
		free(kb.p);
		free(vb.p);
		if (hl_find(out, k.p) >= 0 && memchr(k.p, 0, (size_t)k.n) == NULL) {
			hs_free(&k);
			hs_free(&v);
			continue;
		}
		// a key with a NUL byte cannot be found with hl_find: compare all
		int dup = 0;
		for (int64_t e = 0; e < out->n; e++) {
			if (out->a[e].k.n == k.n && memcmp(out->a[e].k.p, k.p, (size_t)k.n) == 0) {
				dup = 1;
				break;
			}
		}
		if (dup) {
			hs_free(&k);
			hs_free(&v);
			continue;
		}
		hl_add(out, k, v);
	}
	if (out->n > 1) {
		qsort(out->a, (size_t)out->n, sizeof(hpair), hp_cmp);
	}
}

// a raw header field before merging
typedef struct {
	char *k, *v;
	int64_t kn, vn;
} hraw;

// h_line finds the line at b[i..n): its end (without \n, and without the
// \r before it) in *le and the start of the next in *next. 0 when there
// is no \n yet (at eof: the rest is the line, bufio's ReadLine).
static int h_line(const char *b, int64_t i, int64_t n, int eof, int64_t *le, int64_t *next) {
	const char *nl = memchr(b + i, '\n', (size_t)(n - i));
	if (!nl) {
		if (eof && i < n) {
			*le = n;
			*next = n;
			return 1;
		}
		return 0;
	}
	int64_t e = nl - b;
	*next = e + 1;
	if (e > i && b[e - 1] == '\r') {
		e--;
	}
	*le = e;
	return 1;
}

// hp_head reads the request line and the header of the request at the
// start of b (n bytes; eof: no more will come), as net/http's
// readRequest and the checks of conn.readRequest. *used is the length
// of the head on HP_OK.
static int hp_head(const char *b, int64_t n, int eof, hreq *r, int64_t *used) {
	memset(r, 0, sizeof *r);
	if (n == 0) {
		return eof ? HP_EOF : HP_NEED;
	}
	int64_t le, next;
	if (!h_line(b, 0, n, eof, &le, &next)) {
		return n > HTTP_MAX_HEAD ? HP_TOO_LARGE : HP_NEED;
	}
	if (eof && next == n && le == n && n % 4096 == 0) {
		// a request line cut by the end of the stream that fills
		// net/http's 4096 byte reader exactly: its last read finds no
		// byte, so the line reads as io.EOF and the connection closes
		// quietly
		return HP_EOF;
	}
	if (next > HTTP_MAX_HEAD) {
		return HP_TOO_LARGE;
	}
	// METHOD SP target SP proto (strings.Cut on single spaces)
	const char *line = b;
	const char *sp1 = memchr(line, ' ', (size_t)le);
	if (!sp1) {
		return HP_BAD;
	}
	const char *rest = sp1 + 1;
	int64_t restn = le - (rest - line);
	const char *sp2 = memchr(rest, ' ', (size_t)restn);
	if (!sp2) {
		return HP_BAD;
	}
	int64_t mn = sp1 - line;
	if (mn == 0) {
		return HP_BAD;
	}
	for (int64_t i = 0; i < mn; i++) {
		if (!h_tchar((unsigned char)line[i])) {
			return HP_BAD; // invalid method
		}
	}
	const char *proto = sp2 + 1;
	int64_t pn = le - (proto - line);
	int major, minor;
	if (pn == 8 && memcmp(proto, "HTTP/1.1", 8) == 0) {
		major = 1;
		minor = 1;
	} else if (pn == 8 && memcmp(proto, "HTTP/1.0", 8) == 0) {
		major = 1;
		minor = 0;
	} else if (pn == 8 && memcmp(proto, "HTTP/", 5) == 0 && proto[6] == '.' && proto[5] >= '0' && proto[5] <= '9' &&
	           proto[7] >= '0' && proto[7] <= '9') {
		major = proto[5] - '0';
		minor = proto[7] - '0';
	} else {
		return HP_BAD; // malformed HTTP version
	}
	r->method = hs_dup(line, mn);
	r->target = hs_dup(rest, sp2 - rest);
	r->major = major;
	r->minor = minor;
	int connect = hs_eq(r->method, "CONNECT");
	if (!h_parse_target(r->target.p, r->target.n, connect, &r->url)) {
		hr_free(r);
		return HP_BAD;
	}
	if (r->url.rawquery.n > 0) {
		h_parse_query(r->url.rawquery.p, r->url.rawquery.n, &r->query);
	}
	// the header: lines up to an empty one
	int64_t i = next;
	hraw *raw = NULL;
	int64_t nraw = 0, rcap = 0;
	int res = HP_NEED;
	nbuf cont = {0};
	if (i < n && (b[i] == ' ' || b[i] == '\t')) {
		// malformed MIME header initial line (needs the line to say so)
		if (!h_line(b, i, n, eof, &le, &next)) {
			res = eof ? HP_BAD : HP_NEED;
		} else {
			res = HP_BAD;
		}
		goto out;
	}
	for (;;) {
		if (i - 0 > HTTP_MAX_HEAD) {
			res = HP_TOO_LARGE;
			goto out;
		}
		if (!h_line(b, i, n, eof, &le, &next)) {
			res = eof ? HP_BAD : (n > HTTP_MAX_HEAD ? HP_TOO_LARGE : HP_NEED);
			goto out;
		}
		if (le == i) {
			// the empty line ends the header
			i = next;
			break;
		}
		// a field line: it must have a colon
		if (!memchr(b + i, ':', (size_t)(le - i))) {
			res = HP_BAD;
			goto out;
		}
		// trim (spaces and tabs) and join continuation lines
		cont.n = 0;
		int64_t a = i, z = le;
		while (a < z && (b[a] == ' ' || b[a] == '\t')) {
			a++;
		}
		while (z > a && (b[z - 1] == ' ' || b[z - 1] == '\t')) {
			z--;
		}
		nbuf_put(&cont, b + a, z - a);
		i = next;
		for (;;) {
			if (i >= n) {
				if (!eof) {
					res = n > HTTP_MAX_HEAD ? HP_TOO_LARGE : HP_NEED;
					goto out;
				}
				break;
			}
			if (b[i] != ' ' && b[i] != '\t') {
				break;
			}
			// a continuation line: skip its leading spaces and tabs
			int64_t k = i;
			while (k < n && (b[k] == ' ' || b[k] == '\t')) {
				k++;
			}
			if (k >= n && !eof) {
				res = n > HTTP_MAX_HEAD ? HP_TOO_LARGE : HP_NEED;
				goto out;
			}
			nbuf_byte(&cont, ' ');
			if (!h_line(b, k, n, eof, &le, &next)) {
				if (!eof) {
					res = n > HTTP_MAX_HEAD ? HP_TOO_LARGE : HP_NEED;
					goto out;
				}
				// at the end: nothing more (readLineSlice failed)
				i = n;
				break;
			}
			a = k;
			z = le;
			while (a < z && (b[a] == ' ' || b[a] == '\t')) {
				a++;
			}
			while (z > a && (b[z - 1] == ' ' || b[z - 1] == '\t')) {
				z--;
			}
			nbuf_put(&cont, b + a, z - a);
			i = next;
		}
		// key: up to the first colon; token bytes, or spaces (kept as is)
		const char *kv = cont.p;
		const char *colon = memchr(kv, ':', (size_t)cont.n);
		int64_t kn = colon - kv;
		if (kn == 0) {
			res = HP_BAD;
			goto out;
		}
		for (int64_t k = 0; k < kn; k++) {
			unsigned c = (unsigned char)kv[k];
			if (!h_tchar(c) && c != ' ') {
				res = HP_BAD;
				goto out;
			}
		}
		const char *v = colon + 1;
		int64_t vn = cont.n - kn - 1;
		for (int64_t k = 0; k < vn; k++) {
			if (!h_vbyte((unsigned char)v[k])) {
				res = HP_BAD;
				goto out;
			}
		}
		while (vn > 0 && (*v == ' ' || *v == '\t')) {
			v++;
			vn--;
		}
		if (nraw == HTTP_MAX_FIELDS) {
			res = HP_TOO_LARGE;
			goto out;
		}
		if (nraw == rcap) {
			rcap = rcap ? rcap * 2 : 16;
			raw = realloc(raw, (size_t)rcap * sizeof(hraw));
			if (!raw) {
				xo_oom();
			}
		}
		hraw *f = &raw[nraw++];
		f->k = malloc((size_t)kn + 1);
		f->v = malloc((size_t)vn + 1);
		if (!f->k || !f->v) {
			xo_oom();
		}
		memcpy(f->k, kv, (size_t)kn);
		f->k[kn] = 0;
		memcpy(f->v, v, (size_t)vn);
		f->v[vn] = 0;
		f->kn = kn;
		f->vn = vn;
		h_canon(f->k, kn);
	}
	if (i > HTTP_MAX_HEAD) {
		res = HP_TOO_LARGE;
		goto out;
	}
	*used = i;
	// Go's Header: canonical key -> values in order
	{
		// counts per key, for Host and Content-Length checks
		int64_t hosts = 0, tes = 0, cls = 0;
		const char *first_cl = NULL;
		int64_t first_cln = 0;
		int cl_conflict = 0;
		for (int64_t k = 0; k < nraw; k++) {
			if (strcmp(raw[k].k, "Host") == 0) {
				hosts++;
			} else if (strcmp(raw[k].k, "Transfer-Encoding") == 0) {
				tes++;
			} else if (strcmp(raw[k].k, "Content-Length") == 0) {
				// textproto.TrimString: ASCII space, tab, CR, LF
				const char *p = raw[k].v;
				int64_t pl = raw[k].vn;
				while (pl > 0 && strchr(" \t\r\n", p[0]) && p[0]) {
					p++;
					pl--;
				}
				while (pl > 0 && strchr(" \t\r\n", p[pl - 1]) && p[pl - 1]) {
					pl--;
				}
				if (cls == 0) {
					first_cl = p;
					first_cln = pl;
				} else if (pl != first_cln || memcmp(p, first_cl, (size_t)pl) != 0) {
					cl_conflict = 1;
				}
				cls++;
			}
		}
		if (hosts > 1) {
			res = HP_BAD; // too many Host headers
			goto out;
		}
		// readTransfer: Transfer-Encoding (removed from the header)
		int chunked = 0;
		// readTransfer takes HTTP/0.0 for HTTP/1.1
		int tmaj = major, tmin = minor;
		if (tmaj == 0 && tmin == 0) {
			tmaj = tmin = 1;
		}
		if (tes > 0 && (tmaj > 1 || (tmaj == 1 && tmin >= 1))) {
			if (tes != 1) {
				res = HP_BAD_TE;
				goto out;
			}
			for (int64_t k = 0; k < nraw; k++) {
				if (strcmp(raw[k].k, "Transfer-Encoding") == 0) {
					const char *te = raw[k].v;
					if (raw[k].vn != 7 || strncasecmp(te, "chunked", 7) != 0) {
						res = HP_BAD_TE;
						goto out;
					}
				}
			}
			chunked = 1;
		}
		// Content-Length (several must agree)
		int64_t clen = 0;
		if (cl_conflict) {
			res = HP_BAD;
			goto out;
		}
		if (cls > 0) {
			if (first_cln == 0) {
				res = HP_BAD; // invalid empty Content-Length
				goto out;
			}
			uint64_t v = 0;
			for (int64_t k = 0; k < first_cln; k++) {
				if (first_cl[k] < '0' || first_cl[k] > '9' || v > (((uint64_t)1 << 63) - 1 - (uint64_t)(first_cl[k] - '0')) / 10) {
					res = HP_BAD; // bad Content-Length
					goto out;
				}
				v = v * 10 + (uint64_t)(first_cl[k] - '0');
			}
			clen = (int64_t)v;
		}
		if (chunked) {
			clen = -1;
		}
		r->chunked = chunked;
		r->clen = clen;
		// the merged header, without the fields Go removes
		int64_t pragma = -1;
		int have_cc = 0;
		for (int64_t k = 0; k < nraw; k++) {
			const char *key = raw[k].k;
			if (strcmp(key, "Host") == 0 || strcmp(key, "Transfer-Encoding") == 0) {
				continue;
			}
			if (chunked && (strcmp(key, "Content-Length") == 0 || strcmp(key, "Trailer") == 0)) {
				continue;
			}
			if (strcmp(key, "Content-Length") == 0) {
				// deduplicated; its value as written first
				if (hl_find(&r->hdr, key) >= 0) {
					continue;
				}
			}
			int64_t e = -1;
			for (int64_t x = 0; x < r->hdr.n; x++) {
				if (r->hdr.a[x].k.n == raw[k].kn && memcmp(r->hdr.a[x].k.p, key, (size_t)raw[k].kn) == 0) {
					e = x;
					break;
				}
			}
			if (e >= 0) {
				hstr *ov = &r->hdr.a[e].v;
				char *nv = xo_alloc(ov->n + 2 + raw[k].vn + 1);
				memcpy(nv, ov->p, (size_t)ov->n);
				memcpy(nv + ov->n, ", ", 2);
				memcpy(nv + ov->n + 2, raw[k].v, (size_t)raw[k].vn);
				nv[ov->n + 2 + raw[k].vn] = 0;
				xo_free(ov->p);
				ov->p = nv;
				ov->n += 2 + raw[k].vn;
			} else {
				hl_add(&r->hdr, hs_dup(key, raw[k].kn), hs_dup(raw[k].v, raw[k].vn));
			}
			if (strcmp(key, "Pragma") == 0 && pragma < 0) {
				pragma = k;
			}
			if (strcmp(key, "Cache-Control") == 0) {
				have_cc = 1;
			}
		}
		// fixPragmaCacheControl: Pragma: no-cache without Cache-Control
		if (pragma >= 0 && !have_cc && raw[pragma].vn == 8 && memcmp(raw[pragma].v, "no-cache", 8) == 0) {
			hl_add(&r->hdr, hs_dup("Cache-Control", 13), hs_dup("no-cache", 8));
		}
		// req.Host: the URL's host, else the Host header
		const char *host = NULL;
		int64_t hostn = 0;
		for (int64_t k = 0; k < nraw; k++) {
			if (strcmp(raw[k].k, "Host") == 0) {
				host = raw[k].v;
				hostn = raw[k].vn;
			}
		}
		if (r->url.host.n > 0) {
			r->host = hs_dup(r->url.host.p, r->url.host.n);
		} else {
			r->host = hs_dup(host ? host : "", hostn);
		}
		// shouldClose (Connection tokens of every Connection field)
		int has_close = 0, has_ka = 0;
		for (int64_t k = 0; k < nraw; k++) {
			if (strcmp(raw[k].k, "Connection") == 0) {
				has_close |= h_has_token(raw[k].v, raw[k].vn, "close");
				has_ka |= h_has_token(raw[k].v, raw[k].vn, "keep-alive");
			}
		}
		if (major < 1) {
			r->close = 1;
		} else if (major == 1 && minor == 0) {
			r->close = has_close || !has_ka;
		} else {
			r->close = has_close;
		}
		// conn.readRequest's checks
		if (!(major == 1 || (major == 2 && minor == 0 && hs_eq(r->method, "PRI") && hs_eq(r->target, "*")))) {
			r->status = 505;
			r->reason = "unsupported protocol version";
			res = HP_STATUS;
			goto out;
		}
		int h2up = hs_eq(r->method, "PRI") && nraw == 0 && hs_eq(r->url.path, "*") && major == 2 && minor == 0;
		if ((major > 1 || (major == 1 && minor >= 1)) && hosts == 0 && !h2up && !connect) {
			r->status = 400;
			r->reason = "missing required Host header";
			res = HP_STATUS;
			goto out;
		}
		if (hosts == 1) {
			for (int64_t k = 0; k < hostn; k++) {
				unsigned c = (unsigned char)host[k];
				if (!((c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || strchr("!$%&()*+,-.:;=['\\]_~", (int)c)) || c == 0 ||
				    c == '\\') {
					r->status = 400;
					r->reason = "malformed Host header";
					res = HP_STATUS;
					goto out;
				}
			}
		}
		for (int64_t k = 0; k < nraw; k++) {
			for (int64_t x = 0; x < raw[k].kn; x++) {
				if (!h_tchar((unsigned char)raw[k].k[x])) {
					r->status = 400;
					r->reason = "invalid header name";
					res = HP_STATUS;
					goto out;
				}
			}
		}
		// Connection close / keep-alive wishes (Request.wantsClose etc.)
		int64_t ci = hl_find(&r->hdr, "Connection");
		const char *cv = ci >= 0 ? r->hdr.a[ci].v.p : "";
		int64_t cvn = ci >= 0 ? r->hdr.a[ci].v.n : 0;
		// Header.get is the first value only
		for (int64_t k = 0; k < nraw; k++) {
			if (strcmp(raw[k].k, "Connection") == 0) {
				cv = raw[k].v;
				cvn = raw[k].vn;
				break;
			}
		}
		r->wants10ka = major == 1 && minor == 0 && h_server_token(cv, cvn, "keep-alive");
		r->wantsclose = r->close || h_server_token(cv, cvn, "close");
		if (h2up) {
			r->h2up = 1;
			r->clen = 0; // its Body is NoBody (std/rt reads none)
			r->close = 1;
			r->wantsclose = 1;
		}
		const char *ev = NULL;
		int64_t evn = 0;
		for (int64_t k = 0; k < nraw; k++) {
			if (strcmp(raw[k].k, "Expect") == 0) {
				ev = raw[k].v;
				evn = raw[k].vn;
				break;
			}
		}
		if (ev && h_server_token(ev, evn, "100-continue")) {
			r->expect100 = (major > 1 || (major == 1 && minor >= 1)) && r->clen != 0;
		} else if (ev && evn > 0) {
			r->expect_other = 1;
		}
		res = HP_OK;
	}
out:
	for (int64_t k = 0; k < nraw; k++) {
		free(raw[k].k);
		free(raw[k].v);
	}
	free(raw);
	free(cont.p);
	if (res != HP_OK && res != HP_STATUS) {
		hr_free(r);
	}
	return res;
}


// hb_chunked decodes a chunked body at the start of b (n bytes; eof: no
// more will come) into out, as net/http's chunkedReader and the trailer
// reading of body.readTrailer: HB_OK and its length in *used, HB_NEED, or
// HB_ERR (out holds what was decoded before the error).
static int hb_chunked(const char *b, int64_t n, int eof, nbuf *out, int64_t *used) {
	int64_t i = 0, excess = 0;
	out->n = 0;
	for (;;) {
		const char *nl = memchr(b + i, '\n', (size_t)(n - i));
		if (!nl) {
			if (n - i >= 4096) {
				return HB_ERR; // header line too long
			}
			return eof ? HB_ERR : HB_NEED;
		}
		int64_t e = nl - b; // the \n
		// CRLF required, no other CR
		const char *cr = memchr(b + i, '\r', (size_t)(e + 1 - i));
		if (!cr || cr - b != e - 1) {
			return HB_ERR;
		}
		int64_t ll = e - 1 - i; // the line without CRLF
		if (ll >= 4096) {
			return HB_ERR;
		}
		excess += ll + 2;
		int64_t a = i, z = i + ll;
		while (z > a && (b[z - 1] == ' ' || b[z - 1] == '\t')) {
			z--;
		}
		const char *semi = memchr(b + a, ';', (size_t)(z - a));
		if (semi) {
			z = semi - b;
		}
		if (z == a) {
			return HB_ERR; // empty hex number
		}
		uint64_t size = 0;
		for (int64_t k = a; k < z; k++) {
			unsigned c = (unsigned char)b[k];
			if (!h_ishex(c)) {
				return HB_ERR;
			}
			if (k - a == 16) {
				return HB_ERR; // too large
			}
			size = size << 4 | (uint64_t)h_unhex(c);
		}
		excess -= 16 + 2 * (int64_t)(size > (uint64_t)INT64_MAX / 4 ? INT64_MAX / 4 : size);
		if (excess < 0) {
			excess = 0;
		}
		if (excess > 16 * 1024) {
			return HB_ERR; // too much non-data
		}
		i = e + 1;
		if (size == 0) {
			// the trailer: CRLF, or a MIME header up to an empty line
			if (n - i < 2) {
				return eof ? HB_ERR : HB_NEED;
			}
			if (b[i] == '\r' && b[i + 1] == '\n') {
				*used = i + 2;
				return HB_OK;
			}
			// up to a double CRLF within 4096 bytes
			int64_t lim = n - i < 4096 ? n - i : 4096;
			int64_t end = -1;
			for (int64_t k = 3; k < lim; k++) {
				if (b[i + k - 3] == '\r' && b[i + k - 2] == '\n' && b[i + k - 1] == '\r' && b[i + k] == '\n') {
					end = i + k + 1;
					break;
				}
			}
			if (end < 0) {
				if (n - i >= 4096 || eof) {
					return HB_ERR;
				}
				return HB_NEED;
			}
			// the trailer is a MIME header up to its first empty line (a
			// bare LF ends it too)
			hreq tr;
			int64_t tu = 0;
			nbuf fake = {0};
			nbuf_str(&fake, "GET / HTTP/1.0\r\n");
			nbuf_put(&fake, b + i, n - i);
			int t = hp_head(fake.p, fake.n, eof, &tr, &tu);
			free(fake.p);
			if (t == HP_OK || t == HP_STATUS) {
				hr_free(&tr);
			}
			if (t == HP_NEED) {
				return HB_NEED;
			}
			if (t != HP_OK && t != HP_STATUS) {
				return HB_ERR;
			}
			*used = i + tu - 16;
			return HB_OK;
		}
		if (size > (uint64_t)(n - i) || (uint64_t)(n - i) - size < 2) {
			// copy what is there: an error later still delivers it
			// (a size of 2^63 or more is not negative here)
			int64_t have = (uint64_t)(n - i) < size ? n - i : (int64_t)size;
			nbuf_put(out, b + i, have);
			return eof ? HB_ERR : HB_NEED;
		}
		nbuf_put(out, b + i, (int64_t)size);
		i += (int64_t)size;
		if (b[i] != '\r' || b[i + 1] != '\n') {
			return HB_ERR; // malformed chunked encoding
		}
		i += 2;
	}
}

