# std/sql changes

## 2026-11-16: feature: Numeric implements Json (decision 0681)

- Kind: feature
- Commit: this push
- Found by: task #113 (pgx pgtype.Numeric.MarshalJSON and UnmarshalJSON); the lead's decision 2026-11-16
- Go reference: pgx v5.12.0 pgtype/numeric.go (MarshalJSON, UnmarshalJSON), Copyright (c) 2013-2021 Jack Christensen, MIT License
- Test: std/native/sql_test.xo ("Numeric's JSON"): a 40 digit value with its scale, 1e+21, NaN and the infinities as null, null as None, a string refused; the same program on the Go backend and arm64 gave the same output
- Shipped in: not yet released

`derive Json(via: json.Number)`: finite values travel as bare numbers of any length with their scale. Differs from pgx: NaN, Infinity and -Infinity encode as null rather than strings (Xo's encode cannot fail, json.Number writes a non number as null), and decoding a string is json's `expected number` error.

## 2026-11-16: feature: Row.elements for typed arrays (decision 0679)

- Kind: feature
- Commit: this push
- Found by: task #113 (the pgtype ArrayOID rows)
- Go reference: pgx v5.12.0 pgtype/array.go (parseUntypedTextArray, one dimension) and ArrayCodec's element scans, Copyright (c) 2013-2021 Jack Christensen, MIT License
- Test: golden sql_pg_types (uuid, numeric, range and text arrays from text, a driver Array, empty, several dimensions, NULL, element mismatch; Go, arm64, LLVM); scripts/pg-diff.sh `TYPED arr:KIND` for uuid, numeric, date, time, interval, inet and cidr, int4 and int8 ranges, date and numeric ranges, and a composite, equal to pgx on postgres:17-alpine and postgres:14-alpine (110 statements)
- Shipped in: not yet released

`Row.elements(name)` gives an array's elements as one column Rows, read with any reader.

## 2026-11-16: feature: COPY TO STDOUT (decision 0679)

- Kind: feature (breaking for drivers written in Xo and exhaustive TraceOp matches)
- Commit: this push
- Found by: task #113
- Go reference: pgx v5.12.0 pgconn/pgconn.go (PgConn.CopyTo), Copyright (c) 2013-2021 Jack Christensen, MIT License
- Test: golden sql_pg_types (the data in order, an error from out after one call, a server error, in a transaction, the in memory database's Driver error; Go, arm64, LLVM); scripts/pg-diff.sh `COPYTO` cases equal to pgconn on postgres:17-alpine and postgres:14-alpine (text with NULL and escapes, CSV with a header, no rows, rows before a division by zero, a missing table; 100 statements)
- Shipped in: not yet released

`Db.copy_to(q, out)` (and Conn, Tx) runs a COPY TO STDOUT and gives each CopyData to `out`. `DriverConn` gains `copy_to` and `TraceOp` gains `CopyTo`.

## 2026-11-16: feature: large objects (decision 0679)

- Kind: feature
- Commit: this push
- Found by: task #113
- Go reference: pgx v5.12.0 large_objects.go (LargeObjects.Create, Open, Unlink; LargeObject.Write, Read, Seek, Tell, Truncate, Close), Copyright (c) 2013-2021 Jack Christensen, MIT License
- Test: golden sql_pg_types (the SQL each call sends, short write, over-long read, failed unlink; Go, arm64, LLVM); scripts/pg-diff.sh `LO` cases equal to pgx on postgres:17-alpine and postgres:14-alpine (95 statements)
- Shipped in: not yet released

`Tx.lo_*` creates, opens, writes, reads, seeks, truncates, closes and unlinks Postgres large objects through the server's lo_* functions, with pgx's chunking and checks.

## 2026-11-16: feature: numeric as sql.Numeric, numeric ranges (decision 0681)

- Kind: feature
- Commit: this push
- Found by: task #113 (#110's backlog: exact numeric)
- Go reference: pgx v5.12.0 pgtype/numeric.go (parseNumericString, parseScientificNumericString, numberTextBytes, encodeNumericText, toBigInt, Float64Value, Int64Value) and numeric_test.go, Copyright (c) 2013-2021 Jack Christensen, MIT License
- Test: std/native/sql_test.xo (pgx's ScanScientific precision and error cases, Float64Value, Int64Value, TestNumericCodec's text values as display round trips, to_rat; arm64 and LLVM); std/rt/sqlnumeric_test.go (the same on the Go backend); golden sql_pg_types; scripts/pg-diff.sh TYPED numeric, numrange, nummulti and a parameter case, equal to pgx's coefficient and exponent on postgres:17-alpine and postgres:14-alpine (93 statements)
- Shipped in: not yet released

`Row.numeric` reads a numeric column as `sql.Numeric` with pgx's coefficient and exponent, so the scale survives (`1.50` displays as `1.50`); `to_rat` is exact, `to_float` and `to_int` follow pgx's Float64Value and Int64Value. `sql.parse_numeric` also takes the `1e-3` form. Numeric ranges use `Range[Numeric]`.

## 2026-11-16: feature: composite and record columns (decision 0679)

- Kind: feature
- Commit: this push
- Found by: task #113 (#110's backlog: composite and record types)
- Go reference: pgx v5.12.0 pgtype/composite.go (CompositeTextScanner, CompositeTextBuilder, quoteCompositeFieldIfNeeded), Copyright (c) 2013-2021 Jack Christensen, MIT License
- Test: golden sql_pg_types (go, arm64, LLVM: NULL and empty fields, doubled quotes, backslashes, a nested record, malformed text, the parameter's quoting); scripts/pg-diff.sh TYPED fields (a table row type, a nested record, NULL) and a parameter case, equal to pgx's CompositeTextScanner on postgres:17-alpine and postgres:14-alpine (88 statements)
- Shipped in: not yet released

`Row.fields` splits a composite or record column into its fields' texts as pgx's CompositeTextScanner does; `sql.parse_composite` does the same for a nested field, and `sql.composite` builds the text with pgx's quoting. pgx's binary composite scanner and builder are not planned (decision 0278's text results); `Conn.LoadType` is a later item.

## 2026-11-16: feature: ranges and multiranges (decision 0679)

- Kind: feature
- Commit: this push
- Found by: task #113 (#110's backlog: ranges and multiranges)
- Go reference: pgx v5.12.0 pgtype/range.go (parseUntypedTextRange, rangeParseValue, rangeParseQuotedValue, Range, BoundType) and pgtype/multirange.go (parseUntypedTextMultirange, parseRange, Multirange), Copyright (c) 2013-2021 Jack Christensen, MIT License
- Test: golden sql_pg_types (go, arm64, LLVM: bounds, empty, quoted and escaped bounds, offsets, malformed text); scripts/pg-diff.sh TYPED intrange, daterange, timerange, intmulti, datemulti, timemulti cases and a parameter case, equal to pgx on postgres:17-alpine and postgres:14-alpine (85 statements)
- Shipped in: not yet released

`sql.Range[T]` and `sql.Edge[T]` read int4range, int8range, daterange, tsrange and tstzrange columns (`Row.range_int`, `range_date`, `range_time`) and their multiranges (`Row.multirange_*`, a `List[Range[T]]`), parsed as pgx parses the text form; `sql.range_*` and `sql.multirange_*` write them. An infinite bound is a Mismatch; numeric ranges wait for the numeric type.

## 2026-11-16: feature: named arguments, sql.named (decision 0679)

- Kind: feature
- Commit: this push
- Found by: task #113 (#110's backlog: named arguments)
- Go reference: pgx v5.12.0 named_args.go (NamedArgs, StrictNamedArgs, RewriteQuery and its lexer) and named_args_test.go, Copyright (c) 2013-2021 Jack Christensen, MIT License
- Test: std/native/sql_test.xo (pgx's TestNamedArgsRewriteQuery, TestNamedArgsDollarQuotedStrings and TestStrictNamedArgsRewriteQuery cases, arm64 and LLVM, plus a run on the memory driver); std/rt/sqlnamed_test.go (the same cases on the Go backend)
- Shipped in: not yet released

`sql.named(q, args, strict = false)` rewrites each `@name` to `$n` in first use order and returns the arguments in that order; quoted strings, quoted identifiers, dollar quoted strings and comments are skipped as pgx skips them. A missing name reads as NULL; with strict, a missing or unused argument is a Param error. pgx's invalid UTF-8 case has no Str to hold it.

## 2026-11-16: feature: infinite dates and timestamps, macaddr, hstore; xid, cid and xid8 as Integer (decision 0679)

- Kind: feature and behavior
- Commit: this push
- Found by: task #113 (#110's backlog: infinite dates, macaddr, hstore, the Uint32 and Uint64 rows)
- Go reference: pgx v5.11.0 pgtype (Date and Timestamptz InfinityModifier, MacaddrCodec into net.HardwareAddr, Hstore, Uint32Codec and Uint64Codec), Copyright (c) 2013-2021 Jack Christensen, MIT License
- Test: golden sql_pg_types (go, arm64, LLVM); scripts/pg-diff.sh TYPED datebound, timebound, mac and hstore cases and the xid case, equal to pgx on postgres:17-alpine and postgres:14-alpine (78 statements)
- Shipped in: not yet released

`Row.date_bound` and `time_bound` read `infinity` and `-infinity` as `sql.Bound`; `Row.mac` reads macaddr and macaddr8 as their bytes; `Row.hstore` reads an hstore as `Map[Str, Str?]`. An `xid`, `cid` or `xid8` column is now `Integer` (pgx's Uint32 and Uint64), Text before; an `xid8` past Int's range stays Text.

## 2026-11-16: feature: typed readers for uuid, date, time, interval, inet and cidr (decision 0679)

- Kind: feature
- Commit: this push
- Found by: task #110's triage of the open pgtype rows (task #113, the owner's order: Postgres types first)
- Go reference: pgx v5.11.0 pgtype (UUID, Date, Time, Interval, InetCodec into netip.Prefix: their DecodeText), Copyright (c) 2013-2021 Jack Christensen, MIT License
- Test: golden sql_pg_types (go, arm64, LLVM: every case and the Mismatch ones through a driver returning Postgres text); scripts/pg-diff.sh TYPED cases against pgx on postgres:17-alpine and postgres:14-alpine (72 statements, no difference)
- Shipped in: not yet released

`Row.uuid`, `date`, `time_of_day`, `interval` and `prefix` (with `_opt`) parse a column's Postgres text into `uuid.Uuid`, `time.Date`, `time.ClockTime`, `sql.Interval` and `netip.Prefix`; `sql.uuid`, `date`, `time_of_day`, `interval` and `prefix` make the parameters. Before, these columns could only be read as `Row.str` text.

## 2026-11-15: feature: pool statistics and tracing; fix: a cancelled task lost a pool permit

- Kind: feature and fix
- Commit: this commit
- Found by: decision 0648 item 16 (the stdaudit gaps of pgx v5.12.0: no pool statistics, no tracing hooks; Go's DB.Stats); the permit loss was found by the new cancelled waiter test
- Go reference: Go 1.26 database/sql (DB.Stats, DBStats); pgx v5.12.0 (QueryTracer, BatchTracer, CopyFromTracer, PrepareTracer, ConnectTracer and their data structs; pgxpool's Pool.Stat, Stat, AcquireTracer, ReleaseTracer)
- Test: corpus sql (arm64, llvm, go); pg_test.xo on a fake server (arm64, llvm); nine mutants killed
- Shipped in: not yet released

`db.stats()` gives the pool's counts (`sql.PoolStats`: open, in use, idle, connecting, acquires and their time, waits and their time, cancelled waits, new connections, closes by max_idle and max_lifetime). `sql.open(..., on_trace: Some(h))` calls `h` with a `sql.TraceRecord` (op, query, args, rows_affected, err, start_ns, elapsed) as each Connect, Acquire, Release, Query, Exec, Prepare, BatchQuery, Batch and CopyFrom ends. A task cancelled while it gave a connection back to the pool lost the pool's permit (its `send` unwound), so later calls waited forever; the permit goes back with `try_send` now.

## 2026-11-15: feature (breaking for drivers): batches and pipelines

- Kind: feature
- Commit: this commit
- Found by: decision 0648 item 15 (std/sql batch S2; the stdaudit gaps of pgx v5.12.0: no batches or pipelining)
- Go reference: pgx v5.12.0 (Batch, Batch.Queue, QueuedQuery, Conn.SendBatch, BatchResults, pgxpool's Pool, Conn and Tx SendBatch; pgconn's Pipeline with SendQueryParams, Flush and Sync, CommandTag.RowsAffected)
- Test: corpus sql (arm64, llvm, go: the in memory Db's batches, atomic per batch, pipelines, Tx.batch with read only, a driver in Xo for in_tx and on_notice, a driver without batches); pg_test.xo (native and llvm, fake servers: pgx TestConnSendBatch's messages, pgconn's Pipeline with an error skipped to Sync and an error at Sync, windows with Flush and unsent queries after an error; four mutants killed); scripts/pg-diff.sh BATCH lines equal to pgconn's ExecBatch on PostgreSQL 17 and 14 and over TLS (20 cases: atomic rollback, errors inside later windows of 401 queries, a deferred constraint at the Sync, a failed transaction); mysql_test.xo (START TRANSACTION, COMMIT and ROLLBACK per batch); sqlite_test.xo
- Shipped in: not yet released

`db.batch([sql.BatchQuery{query: q, args: [...]}, ...])?` sends the queries at once on one connection and returns a `ResultSet` per query with its rows and `rows_affected()`; a batch is atomic outside a transaction (Postgres's implicit transaction, BEGIN and COMMIT on MySQL and SQLite, a copy of the tables on the in memory database). `db.pipeline([batch1, batch2])?` sends several batches at once, each atomic, with a result per batch. `Conn.batch`, `Conn.pipeline` and `Tx.batch` as well (spec/std.md 10.13). Postgres gets one round trip per 32 KiB window. A driver written in Xo must add `fn pipeline(self, batches: List[List[sql.BatchQuery]], in_tx: Bool) -> List[Result[List[sql.DriverResult], sql.SqlErr]] ! sql.SqlErr { Err(sql.SqlErr.Driver(msg: "no batches")) }` (or run each batch one query at a time, spec/std.md 10.7).

## 2026-11-15: feature (breaking for drivers): Db.conn, LISTEN/NOTIFY, notices, savepoints and DEFERRABLE

- Kind: feature
- Commit: this commit
- Found by: decision 0648 item 14 (std/sql batch S1; the stdaudit gaps of pgx v5.12.0 and database/sql: no connection held across calls, no LISTEN/NOTIFY, no notice handler, no savepoints, no DEFERRABLE)
- Go reference: Go 1.26 database/sql (DB.Conn, Conn, ErrConnDone); pgx v5.12.0 (pgxpool Acquire and Release, Conn.WaitForNotification, pgconn Notice and OnNotice, Tx.Begin's `savepoint sp_N`, `release savepoint`, `rollback to savepoint`, TxOptions.DeferrableMode)
- Test: corpus sql (arm64, llvm, go: a driver in Xo for Conn, savepoints, LISTEN/UNLISTEN quoting, notifications and on_notice; pgx tx_test.go TestTxNestedTransactionCommit and Rollback on a driver and the in memory Db; DEFERRABLE's SET TRANSACTION); pg_test.xo (native and llvm, fake servers: pgx TestListenNotify, TestListenNotifySelfNotification, pgconn TestConnOnNotice and TestConnWaitForNotificationTimeout)
- Shipped in: not yet released

`db.conn()?` holds one pool connection until `close()`, so session state (SET, temporary tables, LISTEN) stays across calls; `c.listen(channel)` and `c.wait_notification()` receive Postgres notifications (one read during an earlier call first; a cancelled wait leaves the connection usable); `open(..., on_notice: Some(f))` and `open_driver` give each server notice to `f`; `tx.begin()` is a savepoint; `begin(deferrable: true)` adds DEFERRABLE (spec/std.md 10.5, 10.7, 10.12). A closed Conn returns `Closed`. A driver written in Xo must add `fn wait_notification(self) -> sql.Notification ! sql.SqlErr { Err(sql.SqlErr.Driver(msg: "LISTEN/NOTIFY is Postgres only")) }` and `fn notices(self) -> List[sql.Notice] { [] }` to its connection.

## 2026-11-15: feature: Postgres CancelRequest on cancellation

- Kind: feature
- Commit: this commit
- Found by: decision 0278 item 7 (CancelRequest, listed as a later step in spec/std.md 10.8)
- Go reference: pgx v5.11.0 pgconn (PgConn.CancelRequest: a new connection, the CancelRequest packet, a wait for the server to close it)
- Test: pg_test.xo (native and llvm: a fake server that never answers, cancelled: a CancelRequest with BackendKeyData pid 77 and secret 1234 on a second connection)
- Shipped in: not yet released

A cancelled wait for a Postgres server now sends a CancelRequest on a second connection before dropping the connection, so the server stops the statement instead of running it to its end; the call still returns `Canceled` (spec/std.md 10.8).

## 2026-11-15: feature: MySQL KILL QUERY on cancellation, and the DSN timeouts

- Kind: feature
- Commit: this commit
- Found by: decision 0648 (item 10's later step; item 2's timeouts, accepted and not applied until now)
- Go reference: go-sql-driver/mysql v1.10.1 (Config.Timeout, ReadTimeout, WriteTimeout as the dial's and each packet's deadlines; a cancelled context closes the connection without KILL); MySQL's own client (KILL QUERY on interrupt)
- Test: mysql_test.xo (native and llvm: the DSN durations and a bad one; a fake server that never answers, cancelled: KILL QUERY 7 on a second connection, the handshake's id; readTimeout=50ms: `Conn` "i/o timeout", then a new connection works)
- Shipped in: not yet released

A cancelled wait for a MySQL server now also runs `KILL QUERY <id>` on a second connection, so the server stops the statement instead of running it to its end; the call still returns `Canceled` and drops the connection (spec/std.md 10.10). The DSN's `timeout` limits the dial, and `readTimeout` and `writeTimeout` each packet's read and write (`Conn` "i/o timeout"); a DSN that has them, ignored before, now has them applied, and one with a value that is not a Go duration is a `Param` error.

## 2026-11-15: feature (breaking for drivers): prepared statements and query_all

- Kind: feature
- Commit: this commit
- Found by: decision 0648 (stage D, items 6 and 7)
- Go reference: Go 1.26 database/sql (DB.Prepare, Tx.Prepare, Stmt per connection, Rows.NextResultSet); go-sql-driver/mysql v1.10.1 (mysqlStmt, NextResultSet skipping results without columns); pgx v5 (Prepare: Parse and Describe of a named statement)
- Test: golden sql_prepare (native, llvm, split, Go parity and wasip1: the in memory database, and a driver in Xo with and without server statements, per connection preparation, close on idle and returned connections, a Tx's statement ending with it, query_all over DriverRows.more); mysql_test.xo and pg_test.xo (fake servers: one prepare and an execute per run, a count mismatch refused before sending, close, a script's first set and every set)
- Shipped in: not yet released

`db.prepare(q)?` returns a `Stmt` to run with `query(args)`, `exec(args)` and the rest, and `db.query_all(script)?` returns each result set (spec/std.md 10.3, 10.7). A driver written in Xo must add `fn prepare(self, q: Str) -> sql.DriverStmt? ! sql.SqlErr { None }` to its connection (and rename a method of its own named `prepare`; std/sqlite's became `prepare_at`); to use server statements it returns a `DriverStmt`. Postgres's `query` of a `;` separated script gave the script's last result set and now gives the first, as pgx and go-sql-driver do. A closed `Stmt` returns `Closed` ("database is closed"): a new SqlErr variant would break exhaustive matches.

## 2026-11-15: feature: MySQL RSA authentication and sql.open's `server_pub_key`

- Kind: feature
- Commit: this commit
- Found by: decision 0648 (stage B)
- Go reference: go-sql-driver/mysql v1.10.1 (RegisterServerPubKey, serverPubKey; caching_sha2_password and sha256_password full authentication); Go 1.26 rsa.EncryptOAEP with SHA-1
- Test: mysqlauth_test.xo (an openssl key as SPKI and PKCS #1 PEM; the ciphertext equals Go's EncryptOAEP with the same seed, and openssl decrypts it); mysql_test.xo (a fake server with an in-test key decrypting the password: fetched, pinned, wrong key, serverPubKey without a key); golden sql_open_mysql_args (other drivers refuse a key, native, llvm, Go parity); scripts/mysql-diff.sh's five RSA passes over tls=false (caching_sha2 and a sha256_password user, key fetched and pinned), equal to go-sql-driver on 8.4 and 8.0
- Shipped in: not yet released

Full authentication over plain TCP encrypts the password under the server's RSA key: `sql.open(..., server_pub_key: pem)`, or the server's own with `allowPublicKeyRetrieval=true`. Differs from go-sql-driver on purpose: go-sql-driver always fetches the key, which a man in the middle can supply.

## 2026-11-15: feature: sql.open's `loc`, MySQL's zone

- Kind: feature
- Commit: this commit
- Found by: decision 0648 (stage B; the lead approved the argument)
- Go reference: go-sql-driver/mysql v1.10.1 Config.Loc (parseDateTime's time.Date, appendDateTime's t.In(Loc)); Go 1.26 time.Date for gaps and overlaps
- Test: golden sql_open_loc (native, llvm and Go backends: a zone other than UTC is refused for postgres and any other driver); mysql_test.xo (wall_in against Go's time.Date vectors for New York and Berlin gaps and overlaps, binds in a zone, a DSN loc that names another zone refused); scripts/mysql-diff.sh's two loc=America/New_York passes, equal to go-sql-driver on 8.4 and 8.0
- Shipped in: not yet released

`sql.open(..., loc: time.zone("America/New_York")?)` reads MySQL's DATE, DATETIME and TIMESTAMP as wall times in that zone and writes `At` arguments as its wall clock; the DSN's `loc` must name the same zone. A value, not a DSN lookup: a run time `time.zone(name)` embeds the 398 KB zone database in every std/sql program (measured: 5,666,225 bytes with no zone, 5,715,409 with one constant zone, 6,112,129 with a run time lookup). Every other driver refuses a zone other than UTC.

## 2026-11-15: feature: the MySQL driver (stage A, and stage B's prepared statements)

- Kind: feature
- Commit: this commit
- Found by: decision 0648
- Go reference: github.com/go-sql-driver/mysql v1.10.1 (its DSN, behaviour and error text; the code is written from MySQL's protocol documentation, not from go-sql-driver's, decision 0648 item 11)
- Test: std/native/llrt/mysqlwire_test.xo, mysqlauth_test.xo and mysql_test.xo (packets, scrambles against Python hashlib vectors, DSNs, values, escaping, errors, and fake servers for each auth path; native and llvm); scripts/mysql-diff.sh (go-sql-driver against the Xo driver on mysql:8.4 and mysql:8.0: 182 statements over verified TLS, interpolated, prepared over the binary protocol, prepared with parseTime and clientFoundRows, with multiStatements, and 10 tls mode outcomes, equal)
- Shipped in: not yet released

`sql.open(net, "mysql", dsn)` opens MySQL (spec/std.md 10.10). Two choices differ from go-sql-driver on purpose: `tls` defaults to `true` (verified), and an error in a later result set of a multi statement query is the query's error. A zero date reads as `Text` with `parseTime=true` (go-sql-driver gives Go's zero time, which is outside `Time`'s range).

## 2026-11-12: behavior (breaking): `sql "-"` or `as "-"` leaves a field out of derive SqlRow

- Kind: behavior
- Commit: this commit
- Found by: decision 0622
- Go reference: none (database/sql has no struct scanning; the rule follows encoding/json's `-`)
- Test: golden derive_skip
- Shipped in: not yet released

A `derive SqlRow` field whose sql (else shared) wire string is exactly `-` has no column: a row read fills its default, else its zero, and its type need not be a column type. It was the column `-`; write `as "-,"` for that column.

## 2026-11-11: feature: Rows.column_types and Row.column_types, Go's ColumnTypes

- Kind: feature
- Commit: this push
- Found by: the std audit of 2026-11-11 (gap 25), decision 0609
- Go reference: database/sql ColumnType; pgx v5 stdlib Rows.ColumnType* (v5.12.0's type map); mattn/go-sqlite3 ColumnTypeDatabaseTypeName
- Test: scripts/pg-diff.sh prints each SELECT's types from pgx stdlib's rules and from std/sql (postgres:17, 46 statements, equal); golden sql_column_types (memory database: arm64, llvm, Go parity); pg_test.xo (OIDs and modifiers); sqlite_test.xo (declared types)
- Timing: none (a table lookup per column when a result's description arrives)
- Shipped in: not yet released

ColumnType (name, database_type, length, precision, scale, nullable, kind) from a new defaulted DriverRows.column_types, so existing drivers still compile and give ColumnType{name} per column. Postgres, sqlite and the in memory database fill it on both backends; spec/std.md (std/sql).

## 2026-11-11: docs: the header says what the module is for

- Kind: docs
- Commit: this commit
- Found by: pkg.xolang.org and xolang.dev showed `xo doc std`'s summary of std/sql as a spec reference or a note about the file (the Xo Lead, 2026-11-11)
- Go reference: none
- Test: cmd/xo TestDocStdSummaries
- Shipped in: not yet released

The header comment of std/sql.xo opens with a sentence that says what the module is for, which `xo doc std` lists as its summary. No declaration changed.

## 2026-11-06: perf: the memory driver builds a statement's row environment once

- Kind: perf
- Commit: this push
- Found by: the sat64 profile of stability/notes on the sql store (2026-11-06: dropping the `Table?` of the per row EvalEnv was 4.2% of CPU)
- Go reference: none (database/sql has no engine)
- Test: std/native/sql_test.xo (10 tests), the sql goldens (native_sql, native_sql_index, sql_conv, sql_driver, sql_json_tagged, sql_tx_driver_err, sql_values, sql_wire_names)
- Timing: host 1 (linux/arm64), 2026-11-06, stability/notes harness load sat64, 3 rounds, against its parent (the CREATE INDEX change): xo 36.2 cpu us/req (36.2 to 36.7) against 38.3 (37.4 to 39.0), better; 54.1k req/s (53.9k to 55.1k) against 50.3k (50.1k to 52.1k)
- Shipped in: not yet released

A SELECT's WHERE and ORDER BY, the select list, and an UPDATE's or DELETE's WHERE evaluate every row in one environment that holds the table, instead of wrapping the table anew for each row, which retained and released it per row. Results and errors are unchanged.

## 2026-11-06: feature: the memory driver takes CREATE INDEX and DROP INDEX

- Kind: feature, perf
- Commit: this push
- Found by: the sat64 profile of stability/notes on the sql store (2026-11-06: mem_select 23% of CPU, GET /notes' `WHERE owner = $1` scanned every note under the Db lock; owner is not UNIQUE, so decision 0528's index did not apply)
- Go reference: none (database/sql has no engine; PostgreSQL's CREATE INDEX syntax, SQLite's non unique index as the model)
- Test: std/native/sql_test.xo (an index's rows and order against a scan through inserts, updates to and from NULL, deletes and compaction; IF NOT EXISTS, unnamed, DROP INDEX and each error); golden native_sql_index (the same texts on the Go backend)
- Timing: host 1 (linux/arm64), 2026-11-06, against the parent f04a8b4ca. stability/notes harness load sat64, 3 rounds: xo 38.3 cpu us/req (37.4 to 39.0) against 36.6 (36.2 to 37.8), ranges overlap; 50.3k req/s against 31.8k (29.5k to 50.9k, a noisy parent). bench/perf/sqlmem none 200000 (the insert path, with the UNIQUE index kept apart from the CREATE INDEX lists and an ASCII scan of names in lex): wall 1.000 (0.985 to 1.011, n=5). bench/perf/sqlmem owner_idx against owner (20,000 rows, 100 owners): 275 against 4358 ms median, 15.8x (bench/results/sqlmem-create-index-2026-11-06.md)
- Shipped in: not yet released

`CREATE INDEX [IF NOT EXISTS] [name] ON table (column)` gives one column an equality index; a `WHERE column = value` (or an AND with one) on an INT or TEXT column then visits only the rows holding the value, in the scan's order, and gives the scan's rows and errors. A UNIQUE column's one row index is still preferred. Unnamed, the index is `table_column_idx`, as in PostgreSQL. `DROP INDEX [IF EXISTS] name` removes it. `CREATE UNIQUE INDEX` and an index on several columns are refused with a syntax error. The Go backend's driver accepts the same statements with the same errors and scans. A value held by more than a quarter of the rows is scanned instead. stability/notes does not index `notes.owner`: its load has one owner, so the index selects every row (harness load sat64 on host 1, 2026-11-06: 36.8k against 44.6k req/s without it, peak RSS 43 against 31 MB).

## 2026-11-06: feature: drivers carry their effects (Driver[fx], Db[fx])

- Kind: feature (API change)
- Commit: this push
- Found by: decision 0534's condition that std/sqlite perform fs, not net.dial
- Go reference: none (database/sql has no effects; its drivers open files freely)
- Test: std/native/sqlite_test.xo (a Driver[fs] through open_driver), std/native/sql_test.xo, golden native_sqlite and sql_driver
- Timing: none (types only; no code path changed)
- Shipped in: not yet released
- Migration: `sql.Db` without an argument still checks as an implicit effect variable; a function that captures a Db in a closure whose type fixes its effects (a handler returned as `fn(...) uses net.dial`) writes `sql.Db[()]` for Postgres and the in memory driver, and so do the functions that pass that Db along (stability/notes and examples/notes-service: 3 signatures each, auth and the two on the path from the Db's creation to it). A driver written in Xo declares `Driver[fx]` and its methods `uses (net.dial, fx)`.

## 2026-11-05: perf: the memory driver looks up UNIQUE columns by index and writes in place

- Kind: perf
- Commit: this push
- Found by: harness load of stability/notes against its Go twin (bench/results/stability-load-2026-11-05.md; note 2026-11-06: the Go twin's GET /notes walked every id issued so far under its lock (O(creates) per list) before 472c82653, so Go relative rows measured before it are inflated (gotchas/twin-cost-grows-with-history.md; rerun in stability-load-2026-11-05.md's 2026-11-06 addendum); the 1.47x below is one of those rows: xo's p50 at 4000/s 1.47x Go's, std/sql about 25% of the CPU profile: full scans and a copy of the table per write under the Db lock)
- Go reference: none (database/sql has no engine; SQLite's rowid and unique indexes are the model)
- Test: std/native/sql_test.xo (8 tests: key lookups on 10,000 rows against the scan's rows, UNIQUE on insert and update, failing statements change nothing, insertion order across compaction, a WHERE the index cannot answer, a transaction's snapshot, CREATE and DROP with trailing text, writes on a growing table); the sql goldens
- Timing: bench/perf/sqlmem (bench/results/sqlmem-0528-2026-11-06.md)
- Shipped in: not yet released

Decision 0528. Each UNIQUE INT or TEXT column (SERIAL columns included) has an equality index, so a WHERE of `col = value` (or an AND with one) on it visits one row and the UNIQUE check is one lookup; other WHEREs scan as before and give the same rows and errors. A write checks everything first and then changes the table in place instead of copying it; a DELETE leaves empty slots, compacted when they pass half the table. A CREATE or DROP with text after the statement still fails and changes nothing.

## 2026-11-05: perf: PostgreSQL COPY batches in a 64 KiB block, not a 128 KiB one

- Kind: perf
- Commit: this push
- Found by: decision 0506 (bench/results/size-class-waste-2026-11-04.md: a Bytes of n bytes takes n + 24 of a block, so a power of two buffer fell into the class twice its size)
- Go reference: none (Go's size classes are about four per doubling, so its 16 and 32 KiB buffers waste about 1%)
- Test: none of its own (a capacity and a send threshold: 65,504 and 64,480 bytes)
- Timing: none (memory: half the block per COPY batch)
- Shipped in: not yet released

A COPY FROM batch is built in a 65,504 byte builder (the 64 KiB class) and sent at 64,480 bytes, so a row of up to 1 KiB more still fits; 65,536 + 1,024 took the 128 KiB class.

## 2026-11-04: perf: the connection URL parser checks a character without slicing

- Kind: perf
- Commit: this push
- Found by: cmd/xo TestVetCostStd on cb9df459a (XO0913 at pgwire.xo:594: `p[0..1] is ":" | "/" | "?" | ","` made a one character Str slice in the host loop only to compare it)
- Go reference: none (pgconn's ParseConfig reads the same characters)
- Test: go test ./cmd/xo -run TestVetCostStd; the pgwire URL tests (std/native/pgwire_test.xo)
- Shipped in: not yet released

The check after a bracketed IPv6 host is four `starts_with` calls now.


## 2026-11-04: fix: pg copy_from stops calling rows once the server refuses the COPY

- Kind: fix
- Commit: this push
- Found by: the owner's v0.38.1 evaluation: after a refusal the callback kept running for all 100k rows, and never returned with an endless source
- Go reference: pgx CopyFrom (reads the server's messages while it sends)
- Test: llrt/pg_test.xo "copy_from stops calling rows once the server refuses the COPY" (arm64, LLVM)
- Shipped in: not yet released

The Postgres driver reads the server's messages in a task while the rows are sent. An error the server raises mid COPY (a bad row, a constraint) stops the calls to `rows`; the driver sends CopyDone, collects the server's answer and returns its error. An error from `rows` still sends CopyFail and wins.

## 2026-11-03: feature: copy_from (COPY FROM STDIN) and quote_ident

- Kind: feature (a new required method on DriverConn: breaking for driver authors)
- Commit: this commit
- Found by: the 2026-10-30 backend survey (rank 9): bulk loads were an `exec` per row
- Go reference: pgx Conn.CopyFrom (text format)
- Decision: 0479 (Accepted, owner, 2026-11-03)
- Test: the sql corpus (arm64, LLVM, Go backend); against Postgres 17: special characters (tab, newline, backslash, CR, NULL, bytea, arrays, jsonb) read back exactly, an error from `rows` rolls the COPY back and leaves the connection usable, scripts/pg-diff.sh
- Timing: Mac, Postgres 17 in Docker: 100,000 rows 0.15 s and 1,000,000 rows 3.4 s at 3.8 MB max RSS; the pgx CopyFrom twin and the `exec` per row and multi-VALUES rows on host 1 follow

`Db.copy_from` and `Tx.copy_from` take a row callback, so the rows are never all in memory; the Postgres driver sends them as CopyData messages of about 64 KiB. `DriverConn` gains `copy_from`: a driver without COPY returns `Driver`. `sql.quote_ident` quotes an identifier.

## 2026-11-03: feature: Rows, a row cursor (Db.rows, Tx.rows, next, next_as, close, each)

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-30 backend survey (rank 9): `Db.query` read every row before it returned, where Go's database/sql streams
- Go reference: database/sql Rows (Next, Scan, Close), pgx
- Decision: 0479 (Accepted, owner, 2026-11-03)
- Test: the sql corpus (arm64, LLVM, Go backend), scripts/pg-diff.sh (45 statements through the new query path), Mac memory run below
- Timing: Mac, Postgres 17 in Docker, 1,000,000 rows of about 100 bytes: `query` max RSS 320 MB, `rows` 3.9 MB (100,000 rows: 34 MB and 3.8 MB); wall time and the Go (pgx) twin row follow on host 1

`Db.rows` and `Tx.rows` return a `Rows` read with `next` or `next_as[T]`, closed with `close`, or walked with `Db.each`. The Postgres driver reads DataRows off the socket as `next` asks; a driver may return a few rows and a `DriverCursor` for the rest (`DriverRows.cursor`, default None, so existing drivers are unchanged), and `query` reads the cursor to the end. The in memory database and the Go backend read the result whole.

## 2026-11-03: feature: begin takes an isolation level and read_only (Go's TxOptions)

- Kind: feature
- Commit: this commit
- Found by: the backend survey of 2026-10-30, rank 13 (std-coverage.md: ledger)
- Go reference: database/sql DB.BeginTx with sql.TxOptions; pgx's stdlib begin modes
- Test: internal/stdtest corpus sql (a recording driver sees SET TRANSACTION per level and read_only, a refused mode rolls back and frees the connection; the in memory Db refuses writes with 25006), native and Go backends
- Shipped in: not yet released

`db.begin(isolation: sql.Isolation.Serializable, read_only: true)`; both
default to what `begin()` did. A driver gets `SET TRANSACTION ISOLATION
LEVEL ..., READ ONLY` right after BEGIN (the DriverConn interface is
unchanged); the in memory Db is serializable at every level and refuses a
write in a read only transaction with Postgres's 25006.

## 2026-10-23: behavior: SqlErr prints plain messages (decision 0358)

- Kind: behavior
- Commit: this commit
- Found by: the Intel field report on 0.26.0 (error texts nested Debug output)
- Go reference: database/sql and pgconn error texts
- Test: goldens sql_driver, sql_values, sql_tx_driver_err, sql_wire_names, native_sql, net_only, std_error_display (arm64, LLVM, Go parity), gen sql_tx, sql_decode, sql_wire_names

`Display` of `SqlErr` (`${e}`, an uncaught error's report) printed the
Debug form, `Conn(msg: "Io(msg: \"tls: ...\")")`. It prints a plain
sentence now: `syntax error: MSG`, MSG for `Constraint`, `Param` and
`Driver`, `no such table: NAME`, `no rows in result set`, `no such column:
NAME`, `column COL: MSG`, `connection: MSG`, `MSG (SQLSTATE CODE)` for
`Server`, `transaction already committed or rolled back`, `database is
closed`, `canceled`. A TLS or network failure under the Postgres driver
reads `connection: tls: handshake failure`, one level. `Debug` and value
comparison are unchanged.

## 2026-10-23: fix: the Postgres driver's denied dial reads as std.md 10.9 says

- Kind: fix
- Commit: this commit
- Found by: the Intel field report on 0.26.0 (Net.only text)
- Go reference: none (the driver dials through Net)
- Test: std/native/llrt/pg_test.xo "an attenuated Net"

`sql.open` over `os.net.only(...)` with an address outside the list
answered `Conn("Denied(addr: db.test:5432)")`; it is now
`Conn("dial db.test:5432: denied by Net.only")`, the text 10.9 gives.

## 2026-10-23: fix: Postgres numeric, json and date columns read as the spec says; a wrong parameter count is Param

- Kind: fix
- Commit: this commit
- Found by: the Intel field report on 0.26.0 (b10, b11)
- Go reference: pgx's Rows.Values and pgconn's PgError (08P01)
- Test: internal/llvm/testdata/golden/sql_conv (arm64, LLVM, Go parity), pg_test "server errors: a wrong parameter count"

`row.float` and `float_opt` read a Text that parses as a Float (a
`numeric` column), `row.str` and `str_opt` read `JsonText` (json and
jsonb), and `row.time` and `time_opt` read a date's Text (`2026-10-20`) as
its midnight in UTC, as 10.9 promises; each returned `Mismatch` before, on
the native backends and on the Go backend. A Postgres statement given the
wrong number of arguments answered `Conn("bind message supplies ...")`,
which reads like a lost connection: the server's 08P01 bind error is now
`Param("statement has N placeholders but M arguments were given")`, the
text the in memory driver gives, `42P02` (no parameter $1, a statement with
placeholders and no arguments) is `Param`, and any other 08P01 is `Server`
(`Conn` stays for the connection exceptions that lose the connection).

## 2026-10-21: fix: a driver transaction stays done when its commit or rollback fails

- Kind: fix
- Commit: 26fb491f
- Found by: Xo Lang 3's Mutex.with rule (core 5.4: an Err out of the closure undoes its writes)
- Go reference: database/sql Tx.Commit and Rollback (done set before the driver call)
- Test: internal/llvm/testdata/golden/sql_tx_driver_err

`Tx.commit` and `rollback` of a driver's transaction set `done` and called
the driver inside the `Mutex.with` closure, so a driver error undid `done`
while the connection was already back in the pool: a second commit ran the
driver on a pooled connection (the test program deadlocked there). They now
mark `done` in a section of its own and call the driver after it, so the
transaction stays done, the connection is given back once, and a second
commit or rollback reports TxDone (or Ok for rollback) without touching it.

## 2026-10-20: feature: Postgres over TLS

- Kind: feature
- Commit: c6acafc2
- Found by: survey
- Go reference: github.com/jackc/pgx/v5 pgconn (configTLS, SSLRequest)
- Test: scripts/pg-diff.sh (PostgreSQL 17 with ssl=on: 45 statements over verify-full and 9 sslmode outcomes, the same as pgx); std/native/llrt/pg_test.xo

The driver sends SSLRequest and upgrades the connection with std/tls's
`tls.client` (decision 0281) for `prefer`, `require`, `verify-ca`, and
`verify-full` (the default), and tries TLS after a refused plain
connection for `allow`; `require` with `roots` checks the chain, as libpq.

## 2026-10-20: feature: Row.value

- Kind: feature
- Commit: 0b8d412f
- Found by: test
- Go reference: github.com/jackc/pgx/v5 Rows.Values
- Test: scripts/pg-diff.sh (each row printed through value)

`row.value(name)` is a column's SqlValue as the driver read it, on every
backend. The Postgres differential test needed it: a native Row's Debug
text is `<row>`, as for every opaque std type.

## 2026-10-20: feature: a Postgres driver written in Xo

- Kind: port
- Commit: 944a4fee
- Found by: survey
- Go reference: github.com/jackc/pgx/v5 v5.11.0 pgproto3 and pgconn (outside Go's std)
- Test: std/native/llrt/pgwire_test.xo (pgx's message vectors and connection string corpora, RFC 7677); std/native/llrt/pg_test.xo (fake servers on FakeNet, decision 0292; was golden pg_fake); scripts/pg-diff.sh against PostgreSQL 17 and 14 in Docker, the same output as pgx

`sql.open(net, "postgres", dsn)` opens PostgreSQL natively: protocol 3,
SCRAM-SHA-256, MD5, and cleartext authentication, keyword and URL
connection strings, simple and extended queries, and text results decoded
by type. TLS (`sslmode` `require` and stronger) waits for decision 0281.

## 2026-10-20: feature: drivers written in Xo, opened with open_driver

- Kind: feature
- Commit: 17d32a65
- Found by: survey
- Go reference: none
- Test: internal/llvm/testdata/golden/sql_driver

A driver implements `sql.Driver` and `sql.DriverConn` in Xo and is opened
with `sql.open_driver(net, driver, dsn)`, which gives it std/sql's pool
(`max_open`, `max_idle`, `max_lifetime`), transactions on one connection,
and cancellation.

## 2026-10-20: feature: bytes, JSON, and array values, and SQLSTATE codes

- Kind: feature
- Commit: a7566d52
- Found by: survey
- Go reference: none
- Test: internal/llvm/testdata/golden/sql_values; internal/llvm/testdata/golden/native_sql

`SqlValue` gains `Blob`, `JsonText`, and `Array` (with `sql.bytes`,
`sql.json`, `sql.array`, and the Row accessors `bytes`, `json`, `ints`,
`strs`, `items`), `derive SqlRow` reads `Bytes` and list fields, and the in
memory driver has `BYTEA`, `JSON`, and `TYPE[]` columns.

## 2026-10-20: behavior: Constraint carries the SQLSTATE and the constraint's name

- Kind: behavior
- Commit: a7566d52
- Found by: survey
- Go reference: none
- Test: internal/llvm/testdata/golden/native_sql; internal/gen/testdata/golden/sql_tx

`SqlErr.Constraint(msg)` is now `Constraint(msg, code, name)` (unique
`23505`, not null `23502`, a value of another type `42804`), and the new
`Server(code, msg)` carries other server errors. A pattern
`Constraint(m)` becomes `Constraint(m, _, _)`.

## 2026-11-07: perf: ORDER BY skips rows already in order and sorts with two buffers

- Kind: perf
- Commit: this commit
- Found by: the Xo-vs-Go survey (bench/results/xo-vs-go-survey-2026-11-07.md, sqlrow/scan 1.40x Go; host 1 profile: the memory driver's sort_idx 38% cumulative)
- Go reference: none (database/sql has no engine; the twin's stub driver serves rows in order); sort.Stable's contract
- Test: the sql corpus on go, arm64 and llvm; the sql goldens on arm64, LLVM and Go parity; xo test --std-module=native/sql; a probe of 8 ORDER BY queries over 777 shuffled rows (several keys, DESC, NULLs, ties, WHERE, LIMIT and OFFSET) identical to main's and the Go backend's
- Timing: bench/perf/sqlrow scan on linux/arm64 host 1, --ab against main (7 rounds, n=6): 0.574 (0.568 to 0.585; 81.0 to 46.5 ms; the survey's Go 60.8 ms), max RSS 0.949; the same bench ordered by `n DESC, id DESC` (every query sorts): 0.895 (0.876 to 0.918, n=7)

The memory driver's ORDER BY checks whether the rows are already in key order (n - 1 compares) and keeps
them as they are when they are, since a stable sort would not move any; otherwise its merge sort runs bottom
up between two index buffers, where the recursive form sliced the index list and built a new one per call.
Results unchanged.

## 2026-11-15: fix: MySQL text values convert as go-sql-driver's, and Row.values

- Kind: fix (and a feature: `Row.values`)
- Commit: this commit
- Found by: internal/stdtest TestMysqlFuzz (new): 300 generated result sets over a fake server, 55 differed from go-sql-driver v1.10.1
- Go reference: go-sql-driver/mysql v1.10.1 textRows.readRow (strconv.ParseInt, ParseUint for BIGINT UNSIGNED, ParseFloat 32 and 64) and parseDateTime; database/sql Rows.Scan into []any
- Test: native/llrt/mysql's "text values convert as go-sql-driver's" on arm64 and llvm (fails before: the conversions returned Text); the sql corpus's Row.values test on go, arm64 and llvm; TestMysqlFuzz with 3000 cases on arm64 and llvm

A text-protocol integer, FLOAT or DOUBLE whose text did not parse came back as a Text value; go-sql-driver fails
the query with strconv's error, and now Xo does too (`SqlErr.Driver(msg)`, read after the rest of the rows so the
connection stays usable). A FLOAT is a float32 as go-sql-driver reads it, a DATETIME with parseTime follows
parseDateTime (fixed places, digits, then time.Date's normalization), and bytes after a row's last value are not
read. Where go-sql-driver ends the rows early at a length past the packet, or panics on a malformed column
definition, Xo refuses with an error (counted by the fuzz test, not a difference).

`Row.values()` returns the values in column order; `value(name)` and the other accessors read the first of two
columns with one name, so `SELECT a.id, b.id` needs it.

## 2026-11-15: fix: the pool drops an idle MySQL connection the server closed

- Kind: fix
- Commit: this commit
- Found by: internal/stdtest TestMysqlFuzz: with one pool for every case, each case after one where the fake server closed its connection failed in Xo and passed in go-sql-driver
- Go reference: go-sql-driver/mysql v1.10.1 conncheck.go (a non-blocking read of the idle socket before reuse, ErrBadConn when it finds bytes or the end) and database/sql's retry of ErrBadConn on another connection
- Test: native/llrt/mysql's "a fake server: an idle connection the server closed is not reused" on arm64 and llvm (fails before: `Conn(msg: "write tcp: connection reset by peer")`)

`Pool.take` asks an idle connection's `is_broken` before it reuses it, and the MySQL driver's `is_broken` now
also checks that nothing waits on its socket (a MSG_PEEK recv that would block; the test world's pipe and wasm's
non-blocking read alike). Postgres is unchanged: a server may send notices and notifications to an idle connection.

## 2026-11-15: break: std/sql's callback types perform sync (decision 0662)

- Kind: breaking (typing only)
- Commit: this commit
- Found by: the sql corpus on main after decision 0662 landed (XO0701: closures that capture an Atomic or a Mutex passed to `each`, `copy_from`, `on_notice` and `on_trace`)
- Go reference: none (Go's callbacks have no effects)
- Test: the sql corpus on go, arm64 and llvm

`each`'s row function, `copy_from`'s row source, and `open` and `open_driver`'s `on_notice` and `on_trace` take
`uses sync` function types, as decision 0662 has std callbacks that share state on purpose say so. A pure
function still passes; a closure that counts rows in an Atomic or collects notices in a Mutex now does too.

## 2026-11-15: fix: MySQL refuses bytes after a text row's last value again

- Kind: fix
- Commit: this commit
- Found by: reading go-sql-driver master while reporting TestMysqlFuzz's findings upstream (the lead: follow master, the safer behavior and their next release)
- Go reference: go-sql-driver/mysql master 96884ae (#1816, 2026-10-06; in no release yet): textRows.readRow returns ErrMalformPkt when bytes follow the last value
- Test: native/llrt/mysqlwire's "column definitions and text rows" and native/llrt/mysql's "text values convert as go-sql-driver's" on arm64 and llvm; TestMysqlFuzz (its reference, v1.10.1, takes such rows; Xo's refusal there is counted as a changed case Xo refuses)

The 2026-11-15 parity change made Xo ignore them as v1.10.1 does; they are a malformed row again, a `Conn`
error that drops the connection.
