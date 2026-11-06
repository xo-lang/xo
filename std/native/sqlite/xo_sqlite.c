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
