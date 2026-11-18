/* xo_sqlite.h: the shapes std/sqlite binds with `use c` (Xo Lang 2, 2026-11-06,
   decision 0534). Every function here exists because the sqlite3.h form has a
   parameter or result the binder skips (spec 4.5): an output pointer, a
   destructor function pointer, an unsigned char or void pointer result.
   sqlite3 and sqlite3_stmt stay opaque, so they bind as handles. */
#ifndef XO_SQLITE_H
#define XO_SQLITE_H
/* guarded: the prebuilt object (decision 0534 stage 2) compiles
   xo_sqlite.c and the binding's adapter, which both include it, as one
   unit */
#include <stdint.h>

typedef struct sqlite3 sqlite3;
typedef struct sqlite3_stmt sqlite3_stmt;

/* a blob or text column as a pointer and a length, returned by value:
   needs the binder's proposed "bytesn" result (ADR item) */
typedef struct xo_sq_buf { const void *p; long long n; } xo_sq_buf;

/* sqlite3_open_v2 with its output pointer folded into the result; NULL
   only when SQLite could not allocate the handle. The open's code is
   xo_sq_errcode(db) after (the handle exists even when the open failed,
   and must still be closed). */
sqlite3 *xo_sq_open(const char *path, int flags, const char *vfs);

/* sqlite3_prepare_v3; NULL on an error (read xo_sq_errmsg(db)) or for a
   text that holds no statement (xo_sq_errcode(db) is 0 then). *tail is
   dropped: the driver prepares one statement at a time and reads the
   consumed length with xo_sq_prepared_len. */
sqlite3_stmt *xo_sq_prepare(sqlite3 *db, const char *sql, long long n, unsigned int flags);
long long xo_sq_prepared_len(sqlite3 *db);

/* bind with SQLITE_TRANSIENT, so SQLite copies before Xo's Str or Bytes
   is released */
int xo_sq_bind_text(sqlite3_stmt *s, int i, const char *text, long long n);
int xo_sq_bind_blob(sqlite3_stmt *s, int i, const void *data, long long n);

/* column_text and column_blob as a pointer and length (copied by Xo
   before the next step, reset or finalize) */
xo_sq_buf xo_sq_column_text(sqlite3_stmt *s, int i);
xo_sq_buf xo_sq_column_blob(sqlite3_stmt *s, int i);

/* sqlite3.h's own functions that bind as they are, declared here with
   sqlite3.h's prototypes (sqlite3_int64 is long long on every target) so
   the binding needs this header alone */
int sqlite3_step(sqlite3_stmt *s);
int sqlite3_reset(sqlite3_stmt *s);
int sqlite3_finalize(sqlite3_stmt *s);
int sqlite3_close_v2(sqlite3 *db);
int sqlite3_bind_int64(sqlite3_stmt *s, int i, long long v);
int sqlite3_bind_double(sqlite3_stmt *s, int i, double v);
int sqlite3_bind_null(sqlite3_stmt *s, int i);
int sqlite3_bind_parameter_count(sqlite3_stmt *s);
const char *sqlite3_bind_parameter_name(sqlite3_stmt *s, int i);
int sqlite3_column_count(sqlite3_stmt *s);
int sqlite3_column_type(sqlite3_stmt *s, int i);
long long sqlite3_column_int64(sqlite3_stmt *s, int i);
double sqlite3_column_double(sqlite3_stmt *s, int i);
const char *sqlite3_column_name(sqlite3_stmt *s, int i);
const char *sqlite3_column_decltype(sqlite3_stmt *s, int i);
long long sqlite3_changes64(sqlite3 *db);
long long sqlite3_last_insert_rowid(sqlite3 *db);
int sqlite3_errcode(sqlite3 *db);
int sqlite3_extended_errcode(sqlite3 *db);
const char *sqlite3_errmsg(sqlite3 *db);
int sqlite3_busy_timeout(sqlite3 *db, int ms);
const char *sqlite3_libversion(void);
int sqlite3_get_autocommit(sqlite3 *db);
int sqlite3_stmt_readonly(sqlite3_stmt *s);
int sqlite3_stmt_busy(sqlite3_stmt *s);
const char *sqlite3_sql(sqlite3_stmt *s);

/* the write gate (task #229): one per database file in the process, so a
   writer that has waited too long gets the next turn instead of losing
   every release to writers that come straight back. 0 is no gate (an
   in-memory or temporary database, or past the table's 64 files). Times
   are poll.now()'s nanoseconds. */
long long xo_sq_gate(sqlite3 *db);
/* queue a waiter until deadline; its ticket (never 0) */
long long xo_sq_gate_join(long long g, long long now, long long deadline);
/* 1 when ticket t (0: a writer not queued) may step now: the queue is
   empty, t is its head, or the head has waited under the grace period
   (since it joined or since the last head left, the later) */
int xo_sq_gate_may(long long g, long long t, long long now);
/* t leaves the queue; now starts the next grace period when t was its head */
void xo_sq_gate_leave(long long g, long long t, long long now);

#endif
