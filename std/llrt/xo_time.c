// xo_time.c: the zone rules of std/time on the native backends (decision
// 0094), included by xo_std.c only when the program uses std/time (the
// build defines XO_TIME and writes xo_tzdata.c, the program's zone data,
// compiled beside the runtime), so other programs do not grow. std/native/rttime.xo declares the
// intrinsics; std/native/time.xo does the rest (civil time, layouts) in
// Xo.
//
// A zone is Go's time.Location: the TZif data is read as
// LoadLocationFromTZData reads it, and an instant is looked up as
// Location.lookup does (transitions, then the POSIX TZ rule of the footer,
// tzset; the first zone for times before the first transition; the cache
// of the zone in effect at load time, which Go fills too), so offsets and
// abbreviations match the Go backend's for every instant.

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
#define TZ_MAX 4096

// Loaded zones by id (1..tz_n); 0 is UTC. A name always gets the same id.
static tz_loc *tz_locs[TZ_MAX + 1];
static int64_t tz_n;
static pthread_mutex_t tz_mu = PTHREAD_MUTEX_INITIALIZER;

static void *tz_malloc(size_t n) {
	void *p = calloc(1, n ? n : 1);
	if (!p) {
		xo_oom();
	}
	return p;
}

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

// ---------------------------------------------------------------------------
// Zone data: embedded per use, the embedded database, or the host's

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

// tz_host loads a zone from the host's database as time.LoadLocation
// does: $ZONEINFO (a directory or a zip), then the system directories.
// "Local" is /etc/localtime.
static tz_loc *tz_host(const char *name) {
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
// Intrinsics (std/native/rttime.xo)

int64_t xo_tz_load(const char *p, int64_t n) { return tz_get(p, n, 0); }

int64_t xo_tz_load_host(const char *p, int64_t n) { return tz_get(p, n, 1); }

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
