/* xo_sqlite.c: see xo_sqlite.h (draft, Xo Lang 2, 2026-11-06) */
#include "sqlite3.h"
#include "xo_sqlite.h"

/* the consumed length of the last prepare per connection; a connection is
   used by one fiber at a time (the pool serializes it), so a field keyed
   by the handle is enough: kept in the handle's client data */
static const char *xo_sq_key = "xo_sq_prepared_len";

sqlite3 *xo_sq_open(const char *path, int flags, const char *vfs) {
	sqlite3 *db = 0;
	sqlite3_open_v2(path, &db, flags | SQLITE_OPEN_EXRESCODE, vfs && *vfs ? vfs : 0);
	if (db) sqlite3_extended_result_codes(db, 1);
	return db;
}

sqlite3_stmt *xo_sq_prepare(sqlite3 *db, const char *sql, long long n, unsigned int flags) {
	sqlite3_stmt *s = 0;
	const char *tail = 0;
	int rc = sqlite3_prepare_v3(db, sql, (int)n, flags, &s, &tail);
	long long used = tail ? (long long)(tail - sql) : n;
	sqlite3_set_clientdata(db, xo_sq_key, (void *)(intptr_t)used, 0);
	return rc == SQLITE_OK ? s : 0;
}

long long xo_sq_prepared_len(sqlite3 *db) {
	return (long long)(intptr_t)sqlite3_get_clientdata(db, xo_sq_key);
}

int xo_sq_bind_text(sqlite3_stmt *s, int i, const char *text, long long n) {
	return sqlite3_bind_text64(s, i, text, (sqlite3_uint64)n, SQLITE_TRANSIENT, SQLITE_UTF8);
}

int xo_sq_bind_blob(sqlite3_stmt *s, int i, const void *data, long long n) {
	return sqlite3_bind_blob64(s, i, data, (sqlite3_uint64)n, SQLITE_TRANSIENT);
}

/* text before bytes: column_bytes after column_text gives the UTF-8
   length (sqlite3.h "Result Values From A Query": the safe order) */
xo_sq_buf xo_sq_column_text(sqlite3_stmt *s, int i) {
	xo_sq_buf b;
	b.p = sqlite3_column_text(s, i);
	b.n = sqlite3_column_bytes(s, i);
	return b;
}

xo_sq_buf xo_sq_column_blob(sqlite3_stmt *s, int i) {
	xo_sq_buf b;
	b.p = sqlite3_column_blob(s, i);
	b.n = sqlite3_column_bytes(s, i);
	return b;
}

/* the write gate (xo_sqlite.h, task #229). SQLite's busy handling is a
   poll: a writer that got SQLITE_BUSY naps and tries again, while one that
   just finished a statement steps again at once and wins nearly every
   release, so a writer that never got in failed at its busy timeout (8
   writers on one WAL file: 9 of 10 runs). Once the oldest waiter has
   waited XO_SQ_GRACE (counted from the last such turn too, so a queue of
   busy writers does not become one strict line) the next turn is its
   own: other writers wait until it steps or leaves.
   The table lives for the process (bounded at XO_SQ_GATES files); its
   lock is SQLite's static application mutex. */
#include <stdlib.h>
#include <string.h>

#define XO_SQ_GATES 64
#define XO_SQ_GRACE 50000000LL

typedef struct xo_sq_wait { long long t, joined, deadline; } xo_sq_wait;
typedef struct xo_sq_gate_s {
	char *name;
	long long next, served;
	int n, cap;
	xo_sq_wait *q;
} xo_sq_gate_s;

static xo_sq_gate_s xo_sq_gates[XO_SQ_GATES];
static int xo_sq_ngates;

static sqlite3_mutex *xo_sq_mu(void) { return sqlite3_mutex_alloc(SQLITE_MUTEX_STATIC_APP1); }

long long xo_sq_gate(sqlite3 *db) {
	const char *f = sqlite3_db_filename(db, "main");
	long long g = 0;
	int i;
	if (!f || !*f) return 0;
	sqlite3_mutex_enter(xo_sq_mu());
	for (i = 0; i < xo_sq_ngates; i++) {
		if (strcmp(xo_sq_gates[i].name, f) == 0) { g = i + 1; break; }
	}
	if (g == 0 && xo_sq_ngates < XO_SQ_GATES) {
		size_t n = strlen(f) + 1;
		char *name = malloc(n);
		if (name) {
			memcpy(name, f, n);
			xo_sq_gates[xo_sq_ngates].name = name;
			g = ++xo_sq_ngates;
		}
	}
	sqlite3_mutex_leave(xo_sq_mu());
	return g;
}

/* drop the waiters whose deadline has passed (a task that faulted or was
   stopped without leaving); the mutex is held */
static void xo_sq_expire(xo_sq_gate_s *gs, long long now) {
	int i, k = 0;
	for (i = 0; i < gs->n; i++) {
		if (gs->q[i].deadline >= now) gs->q[k++] = gs->q[i];
	}
	gs->n = k;
}

long long xo_sq_gate_join(long long g, long long now, long long deadline) {
	xo_sq_gate_s *gs;
	long long t;
	if (g <= 0 || g > XO_SQ_GATES) return 0;
	gs = &xo_sq_gates[g - 1];
	sqlite3_mutex_enter(xo_sq_mu());
	if (gs->n == gs->cap) {
		int cap = gs->cap ? gs->cap * 2 : 16;
		xo_sq_wait *q = realloc(gs->q, (size_t)cap * sizeof *q);
		if (!q) { sqlite3_mutex_leave(xo_sq_mu()); return 0; }
		gs->q = q;
		gs->cap = cap;
	}
	t = ++gs->next;
	gs->q[gs->n].t = t;
	gs->q[gs->n].joined = now;
	gs->q[gs->n].deadline = deadline;
	gs->n++;
	sqlite3_mutex_leave(xo_sq_mu());
	return t;
}

int xo_sq_gate_may(long long g, long long t, long long now) {
	xo_sq_gate_s *gs;
	int may;
	if (g <= 0 || g > XO_SQ_GATES) return 1;
	gs = &xo_sq_gates[g - 1];
	sqlite3_mutex_enter(xo_sq_mu());
	xo_sq_expire(gs, now);
	may = gs->n == 0 || gs->q[0].t == t ||
		now - (gs->q[0].joined > gs->served ? gs->q[0].joined : gs->served) < XO_SQ_GRACE;
	sqlite3_mutex_leave(xo_sq_mu());
	return may;
}

void xo_sq_gate_leave(long long g, long long t, long long now) {
	xo_sq_gate_s *gs;
	int i;
	if (g <= 0 || g > XO_SQ_GATES || t == 0) return;
	gs = &xo_sq_gates[g - 1];
	sqlite3_mutex_enter(xo_sq_mu());
	for (i = 0; i < gs->n; i++) {
		if (gs->q[i].t == t) {
			if (i == 0) gs->served = now;
			memmove(&gs->q[i], &gs->q[i + 1], (size_t)(gs->n - i - 1) * sizeof gs->q[0]);
			gs->n--;
			break;
		}
	}
	sqlite3_mutex_leave(xo_sq_mu());
}
