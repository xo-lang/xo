// xo_time.c: the zone data of std/time on the native backends (decision
// 0094), included by xo_std.c only when the program uses std/time (the
// build defines XO_TIME and writes xo_tzdata.c, the program's zone data,
// compiled beside the runtime), so other programs do not grow.
// std/native/rttime.xo declares the intrinsics. The zone rules are Xo
// (std/native/llrt/tz.xo, decision 0193 step 5): this file finds the
// TZif data of a name (xo_tz_data) and keeps the zones tz.xo parsed, by
// name (xo_tz_find, xo_tz_keep). std/native/time.xo does
// the rest (civil time, layouts) in Xo.
//
// The C path of the rules (XO_TZ_C, std/native/llrt/tz_c.xo with
// XO_LLRT_C=tz) is below too: a zone is Go's time.Location, the TZif data
// read as LoadLocationFromTZData reads it, and an instant looked up as
// Location.lookup does (transitions, then the POSIX TZ rule of the
// footer, tzset; the first zone for times before the first transition;
// the cache of the zone in effect at load time, which Go fills too), so
// offsets and abbreviations match the Go backend's for every instant.

// The program's zone data (decision 0094), defined in xo_tzdata.c, which
// the build writes and compiles beside the runtime, so the runtime's
// object does not depend on the program (prebuilt runtime objects,
// decision 0157): xo_tz_mode 0 and the TZif data of each constant zone
// name (xo_tz_blobs, ended by a null name), 1 and the whole zoneinfo.zip
// (xo_tz_zip), or 2 for the host's database.
typedef struct {
	const char *name;
	const char *data;
	int64_t len;
} xo_tz_blob;
extern const int64_t xo_tz_mode;
extern const char xo_tz_zip[];
extern const int64_t xo_tz_zip_len;
extern const xo_tz_blob xo_tz_blobs[];

#include <sys/stat.h>

#define TZ_MAX 4096

static void *tz_malloc(size_t n) {
	void *p = calloc(1, n ? n : 1);
	if (!p) {
		xo_oom();
	}
	return p;
}

#ifdef XO_TZ_C
typedef struct {
	int64_t off;
	int dst;
	char *abbr;
} tz_zone;

typedef struct {
	int64_t when;
	int64_t idx;
} tz_tx;

typedef struct {
	char *name;
	tz_zone *zone;
	int64_t nzone;
	tz_tx *tx;
	int64_t ntx;
	char *extend; // the footer's TZ rule, "" when there is none
	// Go's cache: the zone in effect when the zone was loaded
	int64_t cstart, cend;
	const char *cname;
	int64_t cnamelen, coff;
	int has_cache;
} tz_loc;

#define TZ_ALPHA INT64_MIN
#define TZ_OMEGA INT64_MAX

// Loaded zones by id (1..tz_n); 0 is UTC. A name always gets the same id.
static tz_loc *tz_locs[TZ_MAX + 1];
static int64_t tz_n;
static pthread_mutex_t tz_mu = PTHREAD_MUTEX_INITIALIZER;

static char *tz_strndup(const char *p, int64_t n) {
	char *s = tz_malloc((size_t)n + 1);
	memcpy(s, p, (size_t)n);
	s[n] = 0;
	return s;
}

// ---------------------------------------------------------------------------
// Calendar helpers (Go's time package)

static int tz_is_leap(int64_t y) { return y % 4 == 0 && (y % 100 != 0 || y % 400 == 0); }

static int64_t tz_days_in(int64_t m, int64_t y) {
	if (m == 2) {
		return tz_is_leap(y) ? 29 : 28;
	}
	return 30 + ((m + (m >> 3)) & 1);
}

static const int64_t tz_days_before[13] = {0, 31, 59, 90, 120, 151, 181, 212, 243, 273, 304, 334, 365};

static int64_t tz_floor_div(int64_t a, int64_t b) {
	int64_t q = a / b;
	if (a % b != 0 && ((a < 0) != (b < 0))) {
		q--;
	}
	return q;
}

// tz_year_yday is the year and the day of the year (1 based) of the day
// that holds sec.
static void tz_year_yday(int64_t sec, int64_t *year, int64_t *yday) {
	int64_t days = tz_floor_div(sec, 86400);
	int64_t z = days + 719468;
	int64_t era = tz_floor_div(z, 146097);
	int64_t doe = z - era * 146097;
	int64_t yoe = (doe - doe / 1460 + doe / 36524 - doe / 146096) / 365;
	int64_t y = yoe + era * 400;
	int64_t doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
	int64_t mp = (5 * doy + 2) / 153;
	int64_t m = mp < 10 ? mp + 3 : mp - 9;
	if (m <= 2) {
		y++;
	}
	*year = y;
	*yday = days - n_days_from_civil(y, 1, 1) + 1;
}

// ---------------------------------------------------------------------------
// The POSIX TZ rule of the footer (Go's tzset)

typedef struct {
	const char *p;
	int64_t n;
} tz_str;

static int tz_set_name(tz_str *s, tz_str *name) {
	if (s->n == 0) {
		return 0;
	}
	if (s->p[0] != '<') {
		for (int64_t i = 0; i < s->n; i++) {
			char c = s->p[i];
			if ((c >= '0' && c <= '9') || c == ',' || c == '-' || c == '+') {
				if (i < 3) {
					return 0;
				}
				name->p = s->p;
				name->n = i;
				s->p += i;
				s->n -= i;
				return 1;
			}
		}
		if (s->n < 3) {
			return 0;
		}
		*name = *s;
		s->p += s->n;
		s->n = 0;
		return 1;
	}
	for (int64_t i = 0; i < s->n; i++) {
		if (s->p[i] == '>') {
			name->p = s->p + 1;
			name->n = i - 1;
			s->p += i + 1;
			s->n -= i + 1;
			return 1;
		}
	}
	return 0;
}

static int tz_set_num(tz_str *s, int64_t min, int64_t max, int64_t *num) {
	if (s->n == 0) {
		return 0;
	}
	int64_t v = 0;
	for (int64_t i = 0; i < s->n; i++) {
		unsigned char c = (unsigned char)s->p[i];
		if (c < '0' || c > '9') {
			if (i == 0 || v < min) {
				return 0;
			}
			*num = v;
			s->p += i;
			s->n -= i;
			return 1;
		}
		v = v * 10 + (c - '0');
		if (v > max) {
			return 0;
		}
	}
	if (v < min) {
		return 0;
	}
	*num = v;
	s->p += s->n;
	s->n = 0;
	return 1;
}

static int tz_set_offset(tz_str *s, int64_t *off) {
	if (s->n == 0) {
		return 0;
	}
	int neg = 0;
	if (s->p[0] == '+') {
		s->p++;
		s->n--;
	} else if (s->p[0] == '-') {
		s->p++;
		s->n--;
		neg = 1;
	}
	int64_t hours, mins, secs;
	if (!tz_set_num(s, 0, 24 * 7, &hours)) {
		return 0;
	}
	int64_t o = hours * 3600;
	if (s->n == 0 || s->p[0] != ':') {
		*off = neg ? -o : o;
		return 1;
	}
	s->p++;
	s->n--;
	if (!tz_set_num(s, 0, 59, &mins)) {
		return 0;
	}
	o += mins * 60;
	if (s->n == 0 || s->p[0] != ':') {
		*off = neg ? -o : o;
		return 1;
	}
	s->p++;
	s->n--;
	if (!tz_set_num(s, 0, 59, &secs)) {
		return 0;
	}
	o += secs;
	*off = neg ? -o : o;
	return 1;
}

enum { TZ_JULIAN, TZ_DOY, TZ_MWD };

typedef struct {
	int kind;
	int64_t day, week, mon, time;
} tz_rule;

static int tz_set_rule(tz_str *s, tz_rule *r) {
	memset(r, 0, sizeof *r);
	if (s->n == 0) {
		return 0;
	}
	if (s->p[0] == 'J') {
		s->p++;
		s->n--;
		if (!tz_set_num(s, 1, 365, &r->day)) {
			return 0;
		}
		r->kind = TZ_JULIAN;
	} else if (s->p[0] == 'M') {
		s->p++;
		s->n--;
		if (!tz_set_num(s, 1, 12, &r->mon) || s->n == 0 || s->p[0] != '.') {
			return 0;
		}
		s->p++;
		s->n--;
		if (!tz_set_num(s, 1, 5, &r->week) || s->n == 0 || s->p[0] != '.') {
			return 0;
		}
		s->p++;
		s->n--;
		if (!tz_set_num(s, 0, 6, &r->day)) {
			return 0;
		}
		r->kind = TZ_MWD;
	} else {
		if (!tz_set_num(s, 0, 365, &r->day)) {
			return 0;
		}
		r->kind = TZ_DOY;
	}
	if (s->n == 0 || s->p[0] != '/') {
		r->time = 2 * 3600;
		return 1;
	}
	s->p++;
	s->n--;
	return tz_set_offset(s, &r->time);
}

static int64_t tz_rule_time(int64_t year, const tz_rule *r, int64_t off) {
	int64_t s = 0;
	switch (r->kind) {
	case TZ_JULIAN:
		s = (r->day - 1) * 86400;
		if (tz_is_leap(year) && r->day >= 60) {
			s += 86400;
		}
		break;
	case TZ_DOY:
		s = r->day * 86400;
		break;
	case TZ_MWD: {
		int64_t m1 = (r->mon + 9) % 12 + 1;
		int64_t yy0 = year;
		if (r->mon <= 2) {
			yy0--;
		}
		int64_t yy1 = yy0 / 100, yy2 = yy0 % 100;
		int64_t dow = ((26 * m1 - 2) / 10 + 1 + yy2 + yy2 / 4 + yy1 / 4 - 2 * yy1) % 7;
		if (dow < 0) {
			dow += 7;
		}
		int64_t d = r->day - dow;
		if (d < 0) {
			d += 7;
		}
		for (int64_t i = 1; i < r->week; i++) {
			if (d + 7 >= tz_days_in(r->mon, year)) {
				break;
			}
			d += 7;
		}
		d += tz_days_before[r->mon - 1];
		if (tz_is_leap(year) && r->mon > 2) {
			d++;
		}
		s = d * 86400;
		break;
	}
	}
	return s + r->time - off;
}

typedef struct {
	tz_str name;
	int64_t off, start, end;
	int dst;
} tz_hit;

// tz_set is Go's tzset: the zone of the rule s at sec, after the last
// transition at last.
static int tz_set(const char *rule, int64_t last, int64_t sec, tz_hit *h) {
	tz_str s = {rule, (int64_t)strlen(rule)};
	tz_str std_name, dst_name;
	int64_t std_off = 0, dst_off = 0;
	if (!tz_set_name(&s, &std_name) || !tz_set_offset(&s, &std_off)) {
		return 0;
	}
	std_off = -std_off;
	if (s.n == 0 || s.p[0] == ',') {
		h->name = std_name;
		h->off = std_off;
		h->start = last;
		h->end = TZ_OMEGA;
		h->dst = 0;
		return 1;
	}
	if (!tz_set_name(&s, &dst_name)) {
		return 0;
	}
	if (s.n == 0 || s.p[0] == ',') {
		dst_off = std_off + 3600;
	} else {
		if (!tz_set_offset(&s, &dst_off)) {
			return 0;
		}
		dst_off = -dst_off;
	}
	if (s.n == 0) {
		s.p = ",M3.2.0,M11.1.0";
		s.n = (int64_t)strlen(s.p);
	}
	if (s.p[0] != ',' && s.p[0] != ';') {
		return 0;
	}
	s.p++;
	s.n--;
	tz_rule start_rule, end_rule;
	if (!tz_set_rule(&s, &start_rule) || s.n == 0 || s.p[0] != ',') {
		return 0;
	}
	s.p++;
	s.n--;
	if (!tz_set_rule(&s, &end_rule) || s.n > 0) {
		return 0;
	}
	int64_t year, yday;
	tz_year_yday(sec, &year, &yday);
	int64_t ysec = (yday - 1) * 86400 + sec % 86400;
	int64_t ystart = sec - ysec;
	int64_t start_sec = tz_rule_time(year, &start_rule, std_off);
	int64_t end_sec = tz_rule_time(year, &end_rule, dst_off);
	int dst_is_dst = 1, std_is_dst = 0;
	if (end_sec < start_sec) {
		int64_t t = start_sec;
		start_sec = end_sec;
		end_sec = t;
		tz_str tn = std_name;
		std_name = dst_name;
		dst_name = tn;
		t = std_off;
		std_off = dst_off;
		dst_off = t;
		std_is_dst = 1;
		dst_is_dst = 0;
	}
	if (ysec < start_sec) {
		h->name = std_name;
		h->off = std_off;
		h->start = ystart;
		h->end = start_sec + ystart;
		h->dst = std_is_dst;
	} else if (ysec >= end_sec) {
		h->name = std_name;
		h->off = std_off;
		h->start = end_sec + ystart;
		h->end = ystart + 365 * 86400;
		h->dst = std_is_dst;
	} else {
		h->name = dst_name;
		h->off = dst_off;
		h->start = start_sec + ystart;
		h->end = end_sec + ystart;
		h->dst = dst_is_dst;
	}
	return 1;
}

// ---------------------------------------------------------------------------
// Lookup (Go's Location.lookup)

static int64_t tz_first_zone(const tz_loc *l) {
	int used = 0;
	for (int64_t i = 0; i < l->ntx; i++) {
		if (l->tx[i].idx == 0) {
			used = 1;
			break;
		}
	}
	if (!used) {
		return 0;
	}
	if (l->ntx > 0 && l->zone[l->tx[0].idx].dst) {
		for (int64_t zi = l->tx[0].idx - 1; zi >= 0; zi--) {
			if (!l->zone[zi].dst) {
				return zi;
			}
		}
	}
	for (int64_t zi = 0; zi < l->nzone; zi++) {
		if (!l->zone[zi].dst) {
			return zi;
		}
	}
	return 0;
}

static void tz_lookup_nocache(const tz_loc *l, int64_t sec, tz_hit *h) {
	if (l->nzone == 0) {
		h->name.p = "UTC";
		h->name.n = 3;
		h->off = 0;
		h->start = TZ_ALPHA;
		h->end = TZ_OMEGA;
		h->dst = 0;
		return;
	}
	if (l->ntx == 0 || sec < l->tx[0].when) {
		const tz_zone *z = &l->zone[tz_first_zone(l)];
		h->name.p = z->abbr;
		h->name.n = (int64_t)strlen(z->abbr);
		h->off = z->off;
		h->start = TZ_ALPHA;
		h->end = l->ntx > 0 ? l->tx[0].when : TZ_OMEGA;
		h->dst = z->dst;
		return;
	}
	int64_t lo = 0, hi = l->ntx, end = TZ_OMEGA;
	while (hi - lo > 1) {
		int64_t m = (int64_t)(((uint64_t)lo + (uint64_t)hi) >> 1);
		int64_t lim = l->tx[m].when;
		if (sec < lim) {
			end = lim;
			hi = m;
		} else {
			lo = m;
		}
	}
	const tz_zone *z = &l->zone[l->tx[lo].idx];
	h->name.p = z->abbr;
	h->name.n = (int64_t)strlen(z->abbr);
	h->off = z->off;
	h->start = l->tx[lo].when;
	h->end = end;
	h->dst = z->dst;
	if (lo == l->ntx - 1 && l->extend[0]) {
		tz_hit e;
		if (tz_set(l->extend, h->start, sec, &e)) {
			*h = e;
		}
	}
}

static void tz_lookup(const tz_loc *l, int64_t sec, tz_hit *h) {
	if (l->has_cache && l->cstart <= sec && sec < l->cend) {
		h->name.p = l->cname;
		h->name.n = l->cnamelen;
		h->off = l->coff;
		return;
	}
	tz_lookup_nocache(l, sec, h);
}

// ---------------------------------------------------------------------------
// TZif (Go's LoadLocationFromTZData)

typedef struct {
	const unsigned char *p;
	int64_t n;
	int err;
} tz_rd;

static const unsigned char *tz_read(tz_rd *d, int64_t n) {
	if (n < 0 || d->n < n) {
		d->p += d->n;
		d->n = 0;
		d->err = 1;
		return NULL;
	}
	const unsigned char *p = d->p;
	d->p += n;
	d->n -= n;
	return p;
}

static int tz_big4(tz_rd *d, uint32_t *v) {
	const unsigned char *p = tz_read(d, 4);
	if (!p) {
		return 0;
	}
	*v = (uint32_t)p[0] << 24 | (uint32_t)p[1] << 16 | (uint32_t)p[2] << 8 | (uint32_t)p[3];
	return 1;
}

static int tz_big8(tz_rd *d, uint64_t *v) {
	uint32_t hi, lo;
	if (!tz_big4(d, &hi) || !tz_big4(d, &lo)) {
		return 0;
	}
	*v = (uint64_t)hi << 32 | lo;
	return 1;
}

static void tz_free(tz_loc *l) {
	if (!l) {
		return;
	}
	for (int64_t i = 0; i < l->nzone; i++) {
		free(l->zone[i].abbr);
	}
	free(l->zone);
	free(l->tx);
	free(l->extend);
	free(l->name);
	free(l);
}

static tz_loc *tz_parse(const char *name, const unsigned char *data, int64_t len) {
	tz_rd d = {data, len, 0};
	const unsigned char *p = tz_read(&d, 4);
	if (!p || memcmp(p, "TZif", 4) != 0) {
		return NULL;
	}
	p = tz_read(&d, 16);
	if (!p) {
		return NULL;
	}
	int version;
	switch (p[0]) {
	case 0:
		version = 1;
		break;
	case '2':
		version = 2;
		break;
	case '3':
		version = 3;
		break;
	default:
		return NULL;
	}
	enum { NUTCLocal, NStdWall, NLeap, NTime, NZone, NChar };
	int64_t n[6];
	for (int i = 0; i < 6; i++) {
		uint32_t v;
		if (!tz_big4(&d, &v)) {
			return NULL;
		}
		n[i] = v;
	}
	int is64 = 0;
	if (version > 1) {
		int64_t skip = n[NTime] * 4 + n[NTime] + n[NZone] * 6 + n[NChar] + n[NLeap] * 8 + n[NStdWall] + n[NUTCLocal];
		skip += 4 + 16;
		tz_read(&d, skip);
		is64 = 1;
		for (int i = 0; i < 6; i++) {
			uint32_t v;
			if (!tz_big4(&d, &v)) {
				return NULL;
			}
			n[i] = v;
		}
	}
	int64_t size = is64 ? 8 : 4;
	tz_rd txtimes = {tz_read(&d, n[NTime] * size), n[NTime] * size, 0};
	const unsigned char *txzones = tz_read(&d, n[NTime]);
	tz_rd zonedata = {tz_read(&d, n[NZone] * 6), n[NZone] * 6, 0};
	const unsigned char *abbrev = tz_read(&d, n[NChar]);
	int64_t nabbrev = n[NChar];
	tz_read(&d, n[NLeap] * (size + 4));
	tz_read(&d, n[NStdWall]);
	tz_read(&d, n[NUTCLocal]);
	if (d.err) {
		return NULL;
	}
	if (n[NZone] == 0) {
		return NULL;
	}
	tz_loc *l = tz_malloc(sizeof *l);
	if (d.n > 2 && d.p[0] == '\n' && d.p[d.n - 1] == '\n') {
		l->extend = tz_strndup((const char *)d.p + 1, d.n - 2);
	} else {
		l->extend = tz_strndup("", 0);
	}
	l->nzone = n[NZone];
	l->zone = tz_malloc(sizeof(tz_zone) * (size_t)l->nzone);
	for (int64_t i = 0; i < l->nzone; i++) {
		uint32_t v;
		const unsigned char *b;
		if (!tz_big4(&zonedata, &v) || !(b = tz_read(&zonedata, 2)) || (int64_t)b[1] >= nabbrev) {
			l->nzone = i;
			tz_free(l);
			return NULL;
		}
		l->zone[i].off = (int32_t)v;
		l->zone[i].dst = b[0] != 0;
		const char *a = (const char *)abbrev + b[1];
		int64_t an = 0;
		while (b[1] + an < nabbrev && a[an] != 0) {
			an++;
		}
		l->zone[i].abbr = tz_strndup(a, an);
	}
	l->ntx = n[NTime];
	l->tx = tz_malloc(sizeof(tz_tx) * (size_t)(l->ntx + 1));
	for (int64_t i = 0; i < l->ntx; i++) {
		int64_t w;
		if (!is64) {
			uint32_t v;
			if (!tz_big4(&txtimes, &v)) {
				tz_free(l);
				return NULL;
			}
			w = (int32_t)v;
		} else {
			uint64_t v;
			if (!tz_big8(&txtimes, &v)) {
				tz_free(l);
				return NULL;
			}
			w = (int64_t)v;
		}
		if ((int64_t)txzones[i] >= l->nzone) {
			tz_free(l);
			return NULL;
		}
		l->tx[i].when = w;
		l->tx[i].idx = txzones[i];
	}
	if (l->ntx == 0) {
		l->tx[0].when = TZ_ALPHA;
		l->tx[0].idx = 0;
		l->ntx = 1;
	}
	l->name = tz_strndup(name, (int64_t)strlen(name));
	// Go fills a cache with the zone in effect now; lookups inside its
	// range return it
	struct timespec ts;
	clock_gettime(CLOCK_REALTIME, &ts);
	int64_t sec = (int64_t)ts.tv_sec;
	for (int64_t i = 0; i < l->ntx; i++) {
		if (l->tx[i].when <= sec && (i + 1 == l->ntx || sec < l->tx[i + 1].when)) {
			const tz_zone *z = &l->zone[l->tx[i].idx];
			l->has_cache = 1;
			l->cstart = l->tx[i].when;
			l->cend = TZ_OMEGA;
			l->cname = z->abbr;
			l->cnamelen = (int64_t)strlen(z->abbr);
			l->coff = z->off;
			if (i + 1 < l->ntx) {
				l->cend = l->tx[i + 1].when;
			} else if (l->extend[0]) {
				tz_hit e;
				if (tz_set(l->extend, l->cstart, sec, &e)) {
					l->cstart = e.start;
					l->cend = e.end;
					// the name points into the rule, which lives as long
					// as the zone
					l->cname = e.name.p;
					l->cnamelen = e.name.n;
					l->coff = e.off;
				}
			}
			break;
		}
	}
	return l;
}

static int tz_valid_name(const char *p, int64_t n) {
	if (n == 0 || n > 255 || (n == 5 && memcmp(p, "Local", 5) == 0) || p[0] == '/') {
		return 0;
	}
	for (int64_t i = 0; i < n; i++) {
		if (p[i] == '\\' || p[i] == 0 || (p[i] == '.' && i + 1 < n && p[i + 1] == '.')) {
			return 0;
		}
	}
	return 1;
}

#endif // XO_TZ_C

// ---------------------------------------------------------------------------
// Zone data: embedded per use, the embedded database, or the host's

static uint32_t tz_le4(const unsigned char *p) { return (uint32_t)p[0] | (uint32_t)p[1] << 8 | (uint32_t)p[2] << 16 | (uint32_t)p[3] << 24; }
static uint32_t tz_le2(const unsigned char *p) { return (uint32_t)p[0] | (uint32_t)p[1] << 8; }

// tz_zip_entry finds a stored entry of a zip archive (std/rt's zipEntry).
static int tz_zip_entry(const unsigned char *z, int64_t zn, const char *name, int64_t nn, const unsigned char **out, int64_t *outn) {
	if (zn < 22) {
		return 0;
	}
	const unsigned char *end = z + zn - 22;
	if (tz_le4(end) != 0x06054b50) {
		return 0;
	}
	int64_t n = tz_le2(end + 10);
	int64_t off = tz_le4(end + 16);
	for (int64_t i = 0; i < n; i++) {
		if (off + 46 > zn) {
			return 0;
		}
		const unsigned char *h = z + off;
		if (tz_le4(h) != 0x02014b50) {
			return 0;
		}
		int64_t method = tz_le2(h + 10);
		int64_t size = tz_le4(h + 24);
		int64_t namelen = tz_le2(h + 28), extralen = tz_le2(h + 30), commentlen = tz_le2(h + 32);
		int64_t local = tz_le4(h + 42);
		if (off + 46 + namelen > zn) {
			return 0;
		}
		const unsigned char *entry = z + off + 46;
		off += 46 + namelen + extralen + commentlen;
		if (namelen != nn || memcmp(entry, name, (size_t)nn) != 0) {
			continue;
		}
		if (method != 0 || local + 30 > zn) {
			return 0;
		}
		const unsigned char *lh = z + local;
		if (tz_le4(lh) != 0x04034b50) {
			return 0;
		}
		int64_t start = local + 30 + tz_le2(lh + 26) + tz_le2(lh + 28);
		if (start + size > zn) {
			return 0;
		}
		*out = z + start;
		*outn = size;
		return 1;
	}
	return 0;
}

// tz_read_file reads a whole regular file (malloc'd), or returns NULL.
static unsigned char *tz_read_file(const char *path, int64_t *n) {
	int fd = open(path, O_RDONLY);
	if (fd < 0) {
		return NULL;
	}
	struct stat st;
	if (fstat(fd, &st) != 0 || !S_ISREG(st.st_mode) || st.st_size > (1 << 24)) {
		close(fd);
		return NULL;
	}
	unsigned char *buf = tz_malloc((size_t)st.st_size + 1);
	int64_t got = 0;
	while (got < st.st_size) {
		ssize_t r = read(fd, buf + got, (size_t)(st.st_size - got));
		if (r <= 0) {
			break;
		}
		got += r;
	}
	close(fd);
	*n = got;
	return buf;
}

#ifdef _WIN32
// tz_win_abbrs: the abbreviations of the Windows time zones by key name
// (Go 1.26's time/zoneinfo_abbrs_windows.go, from CLDR windowsZones.xml;
// the IANA zone each stands for in the comment), as Go's time.Local
// names its zones on Windows.
static const struct {
	const char *key, *std, *dst;
} tz_win_abbrs[] = {
	{"Egypt Standard Time", "EET", "EEST"}, // Africa/Cairo
	{"Morocco Standard Time", "+00", "+01"}, // Africa/Casablanca
	{"South Africa Standard Time", "SAST", "SAST"}, // Africa/Johannesburg
	{"South Sudan Standard Time", "CAT", "CAT"}, // Africa/Juba
	{"Sudan Standard Time", "CAT", "CAT"}, // Africa/Khartoum
	{"W. Central Africa Standard Time", "WAT", "WAT"}, // Africa/Lagos
	{"E. Africa Standard Time", "EAT", "EAT"}, // Africa/Nairobi
	{"Sao Tome Standard Time", "GMT", "GMT"}, // Africa/Sao_Tome
	{"Libya Standard Time", "EET", "EET"}, // Africa/Tripoli
	{"Namibia Standard Time", "CAT", "CAT"}, // Africa/Windhoek
	{"Aleutian Standard Time", "HST", "HDT"}, // America/Adak
	{"Alaskan Standard Time", "AKST", "AKDT"}, // America/Anchorage
	{"Tocantins Standard Time", "-03", "-03"}, // America/Araguaina
	{"Paraguay Standard Time", "-04", "-03"}, // America/Asuncion
	{"Bahia Standard Time", "-03", "-03"}, // America/Bahia
	{"SA Pacific Standard Time", "-05", "-05"}, // America/Bogota
	{"Argentina Standard Time", "-03", "-03"}, // America/Buenos_Aires
	{"Eastern Standard Time (Mexico)", "EST", "EST"}, // America/Cancun
	{"Venezuela Standard Time", "-04", "-04"}, // America/Caracas
	{"SA Eastern Standard Time", "-03", "-03"}, // America/Cayenne
	{"Central Standard Time", "CST", "CDT"}, // America/Chicago
	{"Central Brazilian Standard Time", "-04", "-04"}, // America/Cuiaba
	{"Mountain Standard Time", "MST", "MDT"}, // America/Denver
	{"Greenland Standard Time", "-02", "-01"}, // America/Godthab
	{"Turks And Caicos Standard Time", "EST", "EDT"}, // America/Grand_Turk
	{"Central America Standard Time", "CST", "CST"}, // America/Guatemala
	{"Atlantic Standard Time", "AST", "ADT"}, // America/Halifax
	{"Cuba Standard Time", "CST", "CDT"}, // America/Havana
	{"US Eastern Standard Time", "EST", "EDT"}, // America/Indianapolis
	{"SA Western Standard Time", "-04", "-04"}, // America/La_Paz
	{"Pacific Standard Time", "PST", "PDT"}, // America/Los_Angeles
	{"Mountain Standard Time (Mexico)", "MST", "MST"}, // America/Mazatlan
	{"Central Standard Time (Mexico)", "CST", "CST"}, // America/Mexico_City
	{"Saint Pierre Standard Time", "-03", "-02"}, // America/Miquelon
	{"Montevideo Standard Time", "-03", "-03"}, // America/Montevideo
	{"Eastern Standard Time", "EST", "EDT"}, // America/New_York
	{"US Mountain Standard Time", "MST", "MST"}, // America/Phoenix
	{"Haiti Standard Time", "EST", "EDT"}, // America/Port-au-Prince
	{"Magallanes Standard Time", "-03", "-03"}, // America/Punta_Arenas
	{"Canada Central Standard Time", "CST", "CST"}, // America/Regina
	{"Pacific SA Standard Time", "-04", "-03"}, // America/Santiago
	{"E. South America Standard Time", "-03", "-03"}, // America/Sao_Paulo
	{"Newfoundland Standard Time", "NST", "NDT"}, // America/St_Johns
	{"Pacific Standard Time (Mexico)", "PST", "PDT"}, // America/Tijuana
	{"Yukon Standard Time", "MST", "MST"}, // America/Whitehorse
	{"Jordan Standard Time", "+03", "+03"}, // Asia/Amman
	{"Arabic Standard Time", "+03", "+03"}, // Asia/Baghdad
	{"Azerbaijan Standard Time", "+04", "+04"}, // Asia/Baku
	{"SE Asia Standard Time", "+07", "+07"}, // Asia/Bangkok
	{"Altai Standard Time", "+07", "+07"}, // Asia/Barnaul
	{"Middle East Standard Time", "EET", "EEST"}, // Asia/Beirut
	{"Central Asia Standard Time", "+06", "+06"}, // Asia/Bishkek
	{"India Standard Time", "IST", "IST"}, // Asia/Calcutta
	{"Transbaikal Standard Time", "+09", "+09"}, // Asia/Chita
	{"Sri Lanka Standard Time", "+0530", "+0530"}, // Asia/Colombo
	{"Syria Standard Time", "+03", "+03"}, // Asia/Damascus
	{"Bangladesh Standard Time", "+06", "+06"}, // Asia/Dhaka
	{"Arabian Standard Time", "+04", "+04"}, // Asia/Dubai
	{"West Bank Standard Time", "EET", "EEST"}, // Asia/Hebron
	{"W. Mongolia Standard Time", "+07", "+07"}, // Asia/Hovd
	{"North Asia East Standard Time", "+08", "+08"}, // Asia/Irkutsk
	{"Israel Standard Time", "IST", "IDT"}, // Asia/Jerusalem
	{"Afghanistan Standard Time", "+0430", "+0430"}, // Asia/Kabul
	{"Russia Time Zone 11", "+12", "+12"}, // Asia/Kamchatka
	{"Pakistan Standard Time", "PKT", "PKT"}, // Asia/Karachi
	{"Nepal Standard Time", "+0545", "+0545"}, // Asia/Katmandu
	{"North Asia Standard Time", "+07", "+07"}, // Asia/Krasnoyarsk
	{"Magadan Standard Time", "+11", "+11"}, // Asia/Magadan
	{"N. Central Asia Standard Time", "+07", "+07"}, // Asia/Novosibirsk
	{"Omsk Standard Time", "+06", "+06"}, // Asia/Omsk
	{"North Korea Standard Time", "KST", "KST"}, // Asia/Pyongyang
	{"Qyzylorda Standard Time", "+05", "+05"}, // Asia/Qyzylorda
	{"Myanmar Standard Time", "+0630", "+0630"}, // Asia/Rangoon
	{"Arab Standard Time", "+03", "+03"}, // Asia/Riyadh
	{"Sakhalin Standard Time", "+11", "+11"}, // Asia/Sakhalin
	{"Korea Standard Time", "KST", "KST"}, // Asia/Seoul
	{"China Standard Time", "CST", "CST"}, // Asia/Shanghai
	{"Singapore Standard Time", "+08", "+08"}, // Asia/Singapore
	{"Russia Time Zone 10", "+11", "+11"}, // Asia/Srednekolymsk
	{"Taipei Standard Time", "CST", "CST"}, // Asia/Taipei
	{"West Asia Standard Time", "+05", "+05"}, // Asia/Tashkent
	{"Georgian Standard Time", "+04", "+04"}, // Asia/Tbilisi
	{"Iran Standard Time", "+0330", "+0330"}, // Asia/Tehran
	{"Tokyo Standard Time", "JST", "JST"}, // Asia/Tokyo
	{"Tomsk Standard Time", "+07", "+07"}, // Asia/Tomsk
	{"Ulaanbaatar Standard Time", "+08", "+08"}, // Asia/Ulaanbaatar
	{"Vladivostok Standard Time", "+10", "+10"}, // Asia/Vladivostok
	{"Yakutsk Standard Time", "+09", "+09"}, // Asia/Yakutsk
	{"Ekaterinburg Standard Time", "+05", "+05"}, // Asia/Yekaterinburg
	{"Caucasus Standard Time", "+04", "+04"}, // Asia/Yerevan
	{"Azores Standard Time", "-01", "+00"}, // Atlantic/Azores
	{"Cape Verde Standard Time", "-01", "-01"}, // Atlantic/Cape_Verde
	{"Greenwich Standard Time", "GMT", "GMT"}, // Atlantic/Reykjavik
	{"Cen. Australia Standard Time", "ACST", "ACDT"}, // Australia/Adelaide
	{"E. Australia Standard Time", "AEST", "AEST"}, // Australia/Brisbane
	{"AUS Central Standard Time", "ACST", "ACST"}, // Australia/Darwin
	{"Aus Central W. Standard Time", "+0845", "+0845"}, // Australia/Eucla
	{"Tasmania Standard Time", "AEST", "AEDT"}, // Australia/Hobart
	{"Lord Howe Standard Time", "+1030", "+11"}, // Australia/Lord_Howe
	{"W. Australia Standard Time", "AWST", "AWST"}, // Australia/Perth
	{"AUS Eastern Standard Time", "AEST", "AEDT"}, // Australia/Sydney
	{"UTC-11", "-11", "-11"}, // Etc/GMT+11
	{"Dateline Standard Time", "-12", "-12"}, // Etc/GMT+12
	{"UTC-02", "-02", "-02"}, // Etc/GMT+2
	{"UTC-08", "-08", "-08"}, // Etc/GMT+8
	{"UTC-09", "-09", "-09"}, // Etc/GMT+9
	{"UTC+12", "+12", "+12"}, // Etc/GMT-12
	{"UTC+13", "+13", "+13"}, // Etc/GMT-13
	{"UTC", "UTC", "UTC"}, // Etc/UTC
	{"Astrakhan Standard Time", "+04", "+04"}, // Europe/Astrakhan
	{"W. Europe Standard Time", "CET", "CEST"}, // Europe/Berlin
	{"GTB Standard Time", "EET", "EEST"}, // Europe/Bucharest
	{"Central Europe Standard Time", "CET", "CEST"}, // Europe/Budapest
	{"E. Europe Standard Time", "EET", "EEST"}, // Europe/Chisinau
	{"Turkey Standard Time", "+03", "+03"}, // Europe/Istanbul
	{"Kaliningrad Standard Time", "EET", "EET"}, // Europe/Kaliningrad
	{"FLE Standard Time", "EET", "EEST"}, // Europe/Kiev
	{"GMT Standard Time", "GMT", "BST"}, // Europe/London
	{"Belarus Standard Time", "+03", "+03"}, // Europe/Minsk
	{"Russian Standard Time", "MSK", "MSK"}, // Europe/Moscow
	{"Romance Standard Time", "CET", "CEST"}, // Europe/Paris
	{"Russia Time Zone 3", "+04", "+04"}, // Europe/Samara
	{"Saratov Standard Time", "+04", "+04"}, // Europe/Saratov
	{"Volgograd Standard Time", "MSK", "MSK"}, // Europe/Volgograd
	{"Central European Standard Time", "CET", "CEST"}, // Europe/Warsaw
	{"Mauritius Standard Time", "+04", "+04"}, // Indian/Mauritius
	{"Samoa Standard Time", "+13", "+13"}, // Pacific/Apia
	{"New Zealand Standard Time", "NZST", "NZDT"}, // Pacific/Auckland
	{"Bougainville Standard Time", "+11", "+11"}, // Pacific/Bougainville
	{"Chatham Islands Standard Time", "+1245", "+1345"}, // Pacific/Chatham
	{"Easter Island Standard Time", "-06", "-05"}, // Pacific/Easter
	{"Fiji Standard Time", "+12", "+12"}, // Pacific/Fiji
	{"Central Pacific Standard Time", "+11", "+11"}, // Pacific/Guadalcanal
	{"Hawaiian Standard Time", "HST", "HST"}, // Pacific/Honolulu
	{"Line Islands Standard Time", "+14", "+14"}, // Pacific/Kiritimati
	{"Marquesas Standard Time", "-0930", "-0930"}, // Pacific/Marquesas
	{"Norfolk Standard Time", "+11", "+12"}, // Pacific/Norfolk
	{"West Pacific Standard Time", "+10", "+10"}, // Pacific/Port_Moresby
	{"Tonga Standard Time", "+13", "+13"}, // Pacific/Tongatapu
	{NULL, NULL, NULL},
};

// tz_win_caps writes the capitals of a Windows zone name ("Pacific
// Standard Time": "PST"), the abbreviation Go's time.Local makes of it
// when its table has none, into b (size n).
static void tz_win_caps(const WCHAR *w, char *b, size_t n) {
	size_t k = 0;
	for (; *w && k + 1 < n; w++) {
		if (*w >= L'A' && *w <= L'Z') {
			b[k++] = (char)*w;
		}
	}
	b[k] = 0;
	if (k == 0) {
		snprintf(b, n, "LT");
	}
}

// tz_win_offset appends a POSIX TZ offset of bias minutes (west positive,
// as Windows' Bias) to b.
static void tz_win_offset(char *b, size_t n, LONG bias) {
	size_t k = strlen(b);
	long a = bias < 0 ? -bias : bias;
	snprintf(b + k, n - k, "%s%ld:%02ld", bias < 0 ? "-" : "", a / 60, a % 60);
}

// tz_win_date appends a POSIX TZ rule date (Mm.w.d/h:mm:ss) of a
// SYSTEMTIME in Windows' recurring form (wYear 0: wDay is the week, 5 the
// last) to b.
static void tz_win_date(char *b, size_t n, const SYSTEMTIME *t) {
	size_t k = strlen(b);
	snprintf(b + k, n - k, ",M%d.%d.%d/%d:%02d:%02d", t->wMonth, t->wDay, t->wDayOfWeek, t->wHour, t->wMinute, t->wSecond);
}

// tz_win_rule is the system's time zone setting on Windows (decision
// 0197, stage 5; GetDynamicTimeZoneInformation, the registry's
// TimeZoneInformation) as Go's time.Local builds it there: the standard
// abbreviation (sa, 32 bytes), the POSIX TZ rule of the standard offset
// and the daylight saving rule when there is one (rule, 160 bytes), and
// the standard offset in seconds east (off); 0 when there is none.
static int tz_win_rule(char *sa, char *rule, int64_t *off) {
	DYNAMIC_TIME_ZONE_INFORMATION z;
	if (GetDynamicTimeZoneInformation(&z) == TIME_ZONE_ID_INVALID) {
		return 0;
	}
	char da[32], key[128], sn[128];
	tz_win_caps(z.StandardName, sa, 32);
	tz_win_caps(z.DaylightName, da, sizeof da);
	// Go's abbreviations by the zone's key name, then its standard name
	int kn = xo_w_utf8(z.TimeZoneKeyName, -1, key, sizeof key);
	int snn = xo_w_utf8(z.StandardName, -1, sn, sizeof sn);
	for (int i = 0; tz_win_abbrs[i].key; i++) {
		if ((kn > 0 && strcmp(tz_win_abbrs[i].key, key) == 0) || (snn > 0 && strcmp(tz_win_abbrs[i].key, sn) == 0)) {
			snprintf(sa, 32, "%s", tz_win_abbrs[i].std);
			snprintf(da, sizeof da, "%s", tz_win_abbrs[i].dst);
			break;
		}
	}
	snprintf(rule, 160, "<%s>", sa);
	tz_win_offset(rule, 160, z.Bias + z.StandardBias);
	if (z.DaylightDate.wMonth != 0 && z.StandardDate.wMonth != 0 && z.DaylightDate.wYear == 0 && z.StandardDate.wYear == 0) {
		size_t k = strlen(rule);
		snprintf(rule + k, 160 - k, "<%s>", da);
		tz_win_offset(rule, 160, z.Bias + z.DaylightBias);
		tz_win_date(rule, 160, &z.DaylightDate);
		tz_win_date(rule, 160, &z.StandardDate);
	}
	*off = -(int64_t)(z.Bias + z.StandardBias) * 60;
	return 1;
}
#endif

// tz_data_add is l with the n bytes at p appended (a TZif candidate).
static xo_rlist *tz_data_add(xo_rlist *l, const unsigned char *p, int64_t n) { return n_str_list_add(l, (const char *)p, n); }

#ifdef _WIN32
// tz_win_tzif is "Local" on Windows as TZif data for tz.xo: no
// transitions, one zone (the standard offset and abbreviation), and
// tz_win_rule's rule as the footer, which the lookups extend to every
// year (what the C path's tz_windows builds directly).
static xo_rlist *tz_win_tzif(xo_rlist *l) {
	char sa[32], rule[160];
	int64_t off;
	if (!tz_win_rule(sa, rule, &off)) {
		return l;
	}
	size_t an = strlen(sa), rn = strlen(rule);
	unsigned char b[44 + 44 + 6 + 32 + 2 + 160];
	size_t k = 0;
	memset(b, 0, sizeof b);
	// a version 2 header and an empty version 1 block
	memcpy(b, "TZif2", 5);
	k = 44;
	// the version 2 header: no transitions, one zone, an + 1 characters
	memcpy(b + k, "TZif2", 5);
	uint32_t typecnt = 1, charcnt = (uint32_t)an + 1;
	b[k + 20 + 16 + 3] = (unsigned char)typecnt;
	b[k + 20 + 20 + 3] = (unsigned char)charcnt;
	k += 44;
	uint32_t uoff = (uint32_t)(int32_t)off;
	b[k] = (unsigned char)(uoff >> 24);
	b[k + 1] = (unsigned char)(uoff >> 16);
	b[k + 2] = (unsigned char)(uoff >> 8);
	b[k + 3] = (unsigned char)uoff;
	k += 6;
	memcpy(b + k, sa, an);
	k += an + 1;
	b[k++] = '\n';
	memcpy(b + k, rule, rn);
	k += rn;
	b[k++] = '\n';
	return tz_data_add(l, b, (int64_t)k);
}
#endif

// tz_host_data appends the TZif data that may hold the zone name in the
// host's database to l, in the order time.LoadLocation tries them:
// $ZONEINFO (a directory or a zip), then the system directories; "Local"
// is /etc/localtime (on Windows the system's setting, tz_win_tzif).
static xo_rlist *tz_host_data(xo_rlist *l, const char *name) {
	int64_t n;
#ifdef _WIN32
	if (strcmp(name, "Local") == 0) {
		return tz_win_tzif(l);
	}
#endif
	if (strcmp(name, "Local") == 0) {
		unsigned char *data = tz_read_file("/etc/localtime", &n);
		if (data) {
			l = tz_data_add(l, data, n);
			free(data);
		}
		return l;
	}
	const char *zi = getenv("ZONEINFO");
	const char *dirs[] = {zi, "/usr/share/zoneinfo/", "/usr/share/lib/zoneinfo/", "/usr/lib/locale/TZ/", "/etc/zoneinfo/"};
	for (size_t i = 0; i < sizeof dirs / sizeof dirs[0]; i++) {
		if (!dirs[i] || !dirs[i][0]) {
			continue;
		}
		size_t dl = strlen(dirs[i]), nl = strlen(name);
		char *path = tz_malloc(dl + nl + 2);
		memcpy(path, dirs[i], dl);
		size_t k = dl;
		if (k > 0 && path[k - 1] != '/') {
			path[k++] = '/';
		}
		memcpy(path + k, name, nl);
		path[k + nl] = 0;
		unsigned char *data = tz_read_file(path, &n);
		free(path);
		if (!data && i == 0) {
			// $ZONEINFO may be a zip
			unsigned char *zip = tz_read_file(dirs[i], &n);
			if (zip) {
				const unsigned char *e;
				int64_t en;
				if (tz_zip_entry(zip, n, name, (int64_t)nl, &e, &en)) {
					l = tz_data_add(l, e, en);
				}
				free(zip);
			}
			continue;
		}
		if (data) {
			l = tz_data_add(l, data, n);
			free(data);
		}
	}
	return l;
}

// xo_tz_data is the TZif data that may hold the zone named p, in the
// order to try them: from the program's zone data (xo_tzdata.c: the data
// of each constant name, the whole zoneinfo.zip, or with --tzdata=system
// the host's), or with host set from the host's database. tz.xo checks
// the name first and parses each until one is TZif.
xo_rlist *xo_tz_data(const char *p, int64_t n, int64_t host) {
	xo_rlist *l = n_str_list_new(1);
	char *name = cstr_of(p, n);
	if (host || xo_tz_mode == 2) {
		l = tz_host_data(l, name);
	} else if (xo_tz_mode == 1) {
		const unsigned char *e;
		int64_t en;
		if (tz_zip_entry((const unsigned char *)xo_tz_zip, xo_tz_zip_len, name, n, &e, &en)) {
			l = tz_data_add(l, e, en);
		}
	} else {
		for (int64_t i = 0; xo_tz_blobs[i].name; i++) {
			if ((int64_t)strlen(xo_tz_blobs[i].name) == n && memcmp(xo_tz_blobs[i].name, name, (size_t)n) == 0) {
				l = tz_data_add(l, (const unsigned char *)xo_tz_blobs[i].data, xo_tz_blobs[i].len);
				break;
			}
		}
	}
	free(name);
	return l;
}

// The zones tz.xo parsed, by id (1..tzx_n): a name always gets the same
// id and is parsed once. A zone is a List[Int] whose layout only tz.xo
// knows (its first word is the id); the table holds it shared, so any
// thread may use it.
static char *tzx_names[TZ_MAX + 1];
static xo_rlist *tzx_zones[TZ_MAX + 1];
static int64_t tzx_n;
static pthread_mutex_t tzx_mu = PTHREAD_MUTEX_INITIALIZER;

// tzx_get is the zone named p (a reference), or the empty list; tzx_mu
// is held.
static xo_rlist *tzx_get(const char *p, int64_t n) {
	for (int64_t i = 1; i <= tzx_n; i++) {
		if ((int64_t)strlen(tzx_names[i]) == n && memcmp(tzx_names[i], p, (size_t)n) == 0) {
			rc_inc(&tzx_zones[i]->rc);
			return tzx_zones[i];
		}
	}
	return &empty_list;
}

// xo_tz_find is the zone named p kept so far (a reference), or the empty
// list.
xo_rlist *xo_tz_find(const char *p, int64_t n) {
	pthread_mutex_lock(&tzx_mu);
	xo_rlist *z = tzx_get(p, n);
	pthread_mutex_unlock(&tzx_mu);
	return z;
}

// tzx_free releases the kept zones at exit, so the leak check sees them
// freed (xo_rt.c calls it through xo_tz_exit_free).
static void tzx_free(void) {
	pthread_mutex_lock(&tzx_mu);
	for (int64_t i = 1; i <= tzx_n; i++) {
		if (rc_dec(&tzx_zones[i]->rc)) {
			xo_rl_free(tzx_zones[i], 8, NULL);
		}
		free(tzx_names[i]);
		tzx_zones[i] = NULL;
		tzx_names[i] = NULL;
	}
	tzx_n = 0;
	pthread_mutex_unlock(&tzx_mu);
}

// xo_tz_keep keeps the zone z (a List[Int] of at least one word,
// borrowed) that tz.xo parsed under the name p, writes its id to its
// first word, and returns it (a reference); a zone kept under the name
// first is returned instead. The empty list when the table is full.
xo_rlist *xo_tz_keep(const char *p, int64_t n, xo_rlist *z) {
	pthread_mutex_lock(&tzx_mu);
	xo_rlist *k = tzx_get(p, n);
	if (k == &empty_list && tzx_n < TZ_MAX && z->len > 0) {
		int64_t id = ++tzx_n;
		tzx_names[id] = cstr_of(p, n);
		// z is a list of Ints made by the calling thread: written once
		// here, then shared (its elements hold no references)
		((int64_t *)rl_data(z))[0] = id;
		xo_rc_mark(&z->rc);
		rc_inc(&z->rc);
		tzx_zones[id] = z;
		xo_tz_exit_free = tzx_free;
		rc_inc(&z->rc);
		k = z;
	}
	pthread_mutex_unlock(&tzx_mu);
	return k;
}

// xo_tz_now is the wall clock in seconds (the time of a load: Go fills
// its cache with the zone in effect then).
int64_t xo_tz_now(void) {
	struct timespec ts;
	clock_gettime(CLOCK_REALTIME, &ts);
	return (int64_t)ts.tv_sec;
}

#ifdef XO_TZ_C
#ifdef _WIN32
// tz_windows is "Local" on Windows for the C path: tz_win_rule's zone.
static tz_loc *tz_windows(void) {
	char sa[32], rule[160];
	int64_t off;
	if (!tz_win_rule(sa, rule, &off)) {
		return NULL;
	}
	tz_loc *l = tz_malloc(sizeof *l);
	l->name = tz_strndup("Local", 5);
	l->nzone = 1;
	l->zone = tz_malloc(sizeof(tz_zone));
	l->zone[0].off = off;
	l->zone[0].abbr = tz_strndup(sa, (int64_t)strlen(sa));
	l->ntx = 1;
	l->tx = tz_malloc(sizeof(tz_tx));
	l->tx[0].when = TZ_ALPHA;
	l->tx[0].idx = 0;
	l->extend = tz_strndup(rule, (int64_t)strlen(rule));
	return l;
}
#endif

// tz_host loads a zone from the host's database as time.LoadLocation
// does: $ZONEINFO (a directory or a zip), then the system directories.
// "Local" is /etc/localtime (on Windows the system's setting).
static tz_loc *tz_host(const char *name) {
#ifdef _WIN32
	if (strcmp(name, "Local") == 0) {
		return tz_windows();
	}
#endif
	if (strcmp(name, "Local") == 0) {
		int64_t n;
		unsigned char *data = tz_read_file("/etc/localtime", &n);
		if (!data) {
			return NULL;
		}
		tz_loc *l = tz_parse(name, data, n);
		free(data);
		return l;
	}
	const char *zi = getenv("ZONEINFO");
	const char *dirs[] = {zi, "/usr/share/zoneinfo/", "/usr/share/lib/zoneinfo/", "/usr/lib/locale/TZ/", "/etc/zoneinfo/"};
	for (size_t i = 0; i < sizeof dirs / sizeof dirs[0]; i++) {
		if (!dirs[i] || !dirs[i][0]) {
			continue;
		}
		size_t dl = strlen(dirs[i]), nl = strlen(name);
		char *path = tz_malloc(dl + nl + 2);
		memcpy(path, dirs[i], dl);
		size_t k = dl;
		if (k > 0 && path[k - 1] != '/') {
			path[k++] = '/';
		}
		memcpy(path + k, name, nl);
		path[k + nl] = 0;
		int64_t n;
		unsigned char *data = tz_read_file(path, &n);
		free(path);
		if (!data && i == 0) {
			// $ZONEINFO may be a zip
			unsigned char *zip = tz_read_file(dirs[i], &n);
			if (zip) {
				const unsigned char *e;
				int64_t en;
				if (tz_zip_entry(zip, n, name, (int64_t)nl, &e, &en)) {
					tz_loc *l = tz_parse(name, e, en);
					free(zip);
					if (l) {
						return l;
					}
					continue;
				}
				free(zip);
			}
		}
		if (!data) {
			continue;
		}
		tz_loc *l = tz_parse(name, data, n);
		free(data);
		if (l) {
			return l;
		}
	}
	return NULL;
}

// tz_build loads a zone from the program's zone data (xo_tzdata.c).
static tz_loc *tz_build(const char *name, int64_t nn) {
	if (xo_tz_mode == 2) {
		return tz_host(name);
	}
	if (xo_tz_mode == 1) {
		const unsigned char *e;
		int64_t en;
		if (!tz_zip_entry((const unsigned char *)xo_tz_zip, xo_tz_zip_len, name, nn, &e, &en)) {
			return NULL;
		}
		return tz_parse(name, e, en);
	}
	for (int64_t i = 0; xo_tz_blobs[i].name; i++) {
		if ((int64_t)strlen(xo_tz_blobs[i].name) == nn && memcmp(xo_tz_blobs[i].name, name, (size_t)nn) == 0) {
			return tz_parse(name, (const unsigned char *)xo_tz_blobs[i].data, xo_tz_blobs[i].len);
		}
	}
	return NULL;
}

// tz_get returns the id of the zone named p (from the program's zone data,
// else from the host's when host is set), or 0.
static int64_t tz_get(const char *p, int64_t n, int host) {
	if (!tz_valid_name(p, n) && !(host && n == 5 && memcmp(p, "Local", 5) == 0)) {
		return 0;
	}
	pthread_mutex_lock(&tz_mu);
	for (int64_t i = 1; i <= tz_n; i++) {
		if ((int64_t)strlen(tz_locs[i]->name) == n && memcmp(tz_locs[i]->name, p, (size_t)n) == 0) {
			pthread_mutex_unlock(&tz_mu);
			return i;
		}
	}
	char *name = cstr_of(p, n);
	tz_loc *l = host ? tz_host(name) : tz_build(name, n);
	free(name);
	int64_t id = 0;
	if (l && tz_n < TZ_MAX) {
		id = ++tz_n;
		__atomic_store_n(&tz_locs[id], l, __ATOMIC_RELEASE);
	} else {
		tz_free(l);
	}
	pthread_mutex_unlock(&tz_mu);
	return id;
}

// ---------------------------------------------------------------------------
// Intrinsics of the C path (std/native/rttime.xo, tz_c.xo)

int64_t xo_tz_load(const char *p, int64_t n) { return tz_get(p, n, 0); }

int64_t xo_tz_load_host(const char *p, int64_t n) { return tz_get(p, n, 1); }
#endif // XO_TZ_C

xo_rstr *xo_tz_local_name(int64_t *len) {
	char buf[1024];
	ssize_t r = readlink("/etc/localtime", buf, sizeof buf - 1);
	if (r > 0) {
		buf[r] = 0;
		char *z = strstr(buf, "zoneinfo/");
		if (z) {
			return rs_of_cstr(z + strlen("zoneinfo/"), len);
		}
	}
	return rs_of_cstr("Local", len);
}

#ifdef XO_TZ_C
static const tz_loc *tz_of(int64_t id) {
	if (id < 1 || id > TZ_MAX) {
		return NULL;
	}
	return __atomic_load_n(&tz_locs[id], __ATOMIC_ACQUIRE);
}

int64_t xo_tz_offset(int64_t id, int64_t sec) {
	const tz_loc *l = tz_of(id);
	if (!l) {
		return 0;
	}
	tz_hit h;
	tz_lookup(l, sec, &h);
	return h.off;
}

xo_rstr *xo_tz_abbr(int64_t id, int64_t sec, int64_t *len) {
	const tz_loc *l = tz_of(id);
	if (!l) {
		return nstr("UTC", 3, len);
	}
	tz_hit h;
	tz_lookup(l, sec, &h);
	return nstr(h.name.p, h.name.n, len);
}
#endif // XO_TZ_C
