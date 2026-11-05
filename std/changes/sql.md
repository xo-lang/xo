# std/sql changes

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
