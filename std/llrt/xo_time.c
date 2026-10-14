// xo_time.c: the zone data of std/time on the native backends (decision
// 0094), included by xo_std.c only when the program uses std/time (the
// build defines XO_TIME and writes xo_tzdata.c, the program's zone data,
// compiled beside the runtime), so other programs do not grow.
// std/native/rttime.xo declares the intrinsics. The zone rules are Xo
// (std/native/llrt/tz.xo, decision 0193 step 5): this file finds the
// TZif data of a name (xo_tz_data) and keeps the zones tz.xo parsed, by
// name (xo_tz_find, xo_tz_keep). std/native/time.xo does
// the rest (civil time, layouts) in Xo. Decision 0198 deleted the C path
// of the rules (XO_TZ_C).

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

// ---------------------------------------------------------------------------
// Zone data: embedded per use, the embedded database, or the host's

int64_t xo_x_tz_zip_entry(int64_t z, int64_t zn, int64_t name, int64_t nn, int64_t out);

// tz_zip_entry finds a stored entry of a zip archive (std/rt's zipEntry;
// cabi.xo, decision 0207).
static int tz_zip_entry(const unsigned char *z, int64_t zn, const char *name, int64_t nn, const unsigned char **out, int64_t *outn) {
	int64_t r[2];
	if (!xo_x_tz_zip_entry(XO_ADDR(z), zn, XO_ADDR(name), nn, XO_ADDR(r))) {
		return 0;
	}
	*out = z + r[0];
	*outn = r[1];
	return 1;
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
// year.
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
int64_t xo_x_real_ns(void);
int64_t xo_tz_now(void) { return xo_x_real_ns() / 1000000000; }

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

