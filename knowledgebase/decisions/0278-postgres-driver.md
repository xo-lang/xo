# 0278: Postgres driver for std/sql

Status: Accepted (owner, 2026-10-20), with the default `sslmode` changed
to `verify-full` (item 3). Spec: spec/std.md 10.7 to 10.9, landing with
the implementation. Proposed by Xo Lang Lib 1.

## Context

The survey of decision 0276 (knowledgebase/infrastructure/std-coverage.md)
wrote five backend services from the spec; all five needed a real
database, and natively `sql.open` has no driver at all: the plan of
spec/std.md 10 was Go drivers bound with `use go`, which native builds,
the default since decision 0248, cannot use. Three of the five also
needed values std/sql cannot carry (bytea, json, arrays) or the
server's error code (a unique violation told apart from a foreign key
one).

Go's database/sql leaves drivers outside std; the ecosystem's Postgres
drivers (github.com/jackc/pgx, MIT; github.com/lib/pq, MIT) implement
the frontend/backend protocol version 3 over a TCP or Unix socket. The
protocol is small, stable since PostgreSQL 7.4, and fully documented.

## Decision

1. **A driver in Xo, shipped with std.** `sql.open(net, "postgres",
   dsn)` opens a Postgres database on every native target. The driver
   is Xo: protocol version 3 (startup, simple and extended query,
   CancelRequest, SSLRequest), authentication by trust, cleartext
   password, MD5, and SCRAM-SHA-256 (RFC 7677; channel binding, the
   `-PLUS` form, is a later step). Its message codec, connection string
   parsing, SCRAM, and error mapping follow pgx v5 (pgconn,
   pgproto3), attributed per file and in THIRD_PARTY_NOTICES.
2. **The driver dials through the `Net` it is given**, so an attenuated
   `Net` now works: `sql.open(os.net.only(["db.internal"]), ...)`
   reaches only that host, replacing the fail closed rule of decisions
   0156 and 0175 for drivers written in Xo. No environment variable or
   file is read (no `PGPASSWORD`, no `~/.pgpass`, no
   `sslrootcert=path`): the connection string and the arguments of
   `open` are the whole configuration, as capabilities require. Roots
   for a private CA are passed as PEM bytes (`roots:`), read by the
   caller with `Fs`.
3. **TLS** uses `tls.client(c, host, ...)` of std/tls (decision 0281,
   owned by Xo Lang 3), whose `TlsConn` implements `Stream`: the driver
   sends SSLRequest in plain text and upgrades the same connection. `sslmode` maps onto its `TlsVerify` levels: `disable`
   (no TLS), `prefer` (TLS when the server offers it, verification
   `None`, as libpq), `require` (`None`), `verify-ca` (`ChainOnly`),
   `verify-full` (`Full`). The default is `verify-full` (owner,
   2026-10-20), not libpq's and pgx's `prefer`: a connection string
   without `sslmode` gets TLS with the chain and host name verified,
   and one that names `sslmode` keeps its meaning. A Unix socket never
   uses TLS, so the default does not apply there.
4. **A driver interface** (`sql.Driver`, `sql.DriverConn`) and
   `sql.open_driver(net, driver, dsn, ...)`, so other drivers (MySQL,
   SQLite) are ordinary modules. There is no registry (it would be a
   package global, decision 0276 item 4): a third party driver is a
   value passed to `open_driver`; `"postgres"` and the in memory driver
   are the names `open` knows. Pooling (`max_open`, `max_idle`,
   `max_lifetime`), transactions holding one connection, and
   cancellation live in std/sql above the interface, so every driver
   gets them.
5. **Values.** `SqlValue` gains `Blob(b: Bytes)`, `JsonText(s: Str)`,
   and `Array(items: List[SqlValue])`, with constructors `sql.bytes`,
   `sql.json[T: Json]` (encodes), `sql.array`, and Row accessors
   `bytes`, `json[T: Json]` (decodes), `ints`, `strs` (and `_opt`
   forms). Postgres types map as 10.8 lists; types without a mapping
   (numeric, uuid, date, interval, inet, enums) read as `Text`, their
   exact server text, so nothing is lost or rounded.
6. **Errors carry the SQLSTATE.** `SqlErr.Constraint` becomes
   `Constraint(msg: Str, code: Str, name: Str)` (the code, `23505`, and
   the constraint's name) and a new `Server(code: Str, msg: Str)`
   carries any other error the server reports with a code. `Syntax`,
   `NoTable`, `Conn`, and `Canceled` are mapped from their codes
   (`42601`, `42P01`, class `08`, `57014`). The in memory driver fills
   the same codes for its constraint failures (`23505` unique, `23502`
   not null), so tests can match on codes. This changes a variant's
   fields, allowed in the `preview` edition (decision 0155); the three
   in repo users (golden programs) move with it.
7. **Protocol choices.** A statement without arguments runs with the
   simple query protocol (so `exec` of a `;` separated schema works,
   as pgx's Exec does); with arguments, the extended protocol with an
   unnamed statement, text format parameters (binary for `Blob`), and
   text format results decoded by type OID. Results are read whole, as
   the existing API returns `List[Row]`. Cancellation of the waiting
   task sends CancelRequest on a second connection and returns
   `Canceled`; the connection is then drained to ReadyForQuery or
   discarded. `last_insert_id` is 0 (use `RETURNING`).
8. **Tests.** pgx's tests for the pieces ported (pgproto3 message
   round trips, connection string parsing, SCRAM vectors, RFC 7677's
   example), an in process fake server on FakeNet for the driver's
   state machine (auth failures, errors mid result, cancellation,
   broken connections), and a differential test against a real
   PostgreSQL in Docker (scripts/docker-linux.sh pattern; the same
   statements through the driver and through psql, the same rows and
   error codes), on PostgreSQL 13 and the current release.
9. **Backends.** The native backends (arm64, LLVM, amd64). WebAssembly
   has no sockets (no `Net`), so it has no network driver. The Go
   backend keeps std/rt's in memory driver and reports an unknown
   driver for `"postgres"` until the Go backend can build std modules
   written in Xo; recorded as a known gap in the coverage table.

Later, each its own step: LISTEN/NOTIFY (a notification stream on a
dedicated connection), COPY, prepared statement caching, row streaming
(a `query_each` that does not buffer), SCRAM channel binding, binary
result format for speed.

## The effect of reading a connection

std/sql's operations declare `uses net.dial`, but `Conn.read` and
`Conn.write` declared `uses net`, so a driver in Xo performed more than
its interface allows (the prototype hit XO0701). Owner, 2026-10-20:
reads and writes on a dialed connection are `net.dial` and on an
accepted one `net.listen`, decided in its own record, decision 0280,
which lands before this driver.

## Consequences

- Every native program can use Postgres with no C library and no Go
  toolchain; std/sql's spec stops promising Go drivers.
- `Net.only` covers database access too.
- Matching on `Constraint(m)` with one field no longer compiles; such
  code binds the new fields too (`Constraint(m, _, _)`).
- std carries a protocol implementation it must keep current with
  PostgreSQL releases (protocol 3.2 of PostgreSQL 18 is optional and
  backward compatible).

## Alternatives

- **Bind pgx with `use go`.** Go backend only; native is the default.
- **libpq through `use c`.** A C dependency per target, against the
  zero-C direction of the native runtime (decision 0243), and libpq
  reads environment variables and files behind the capabilities.
- **A registry of drivers by name.** Needs a package global; a driver
  value passed to `open_driver` does the same without one.
- **Default `sslmode=prefer`, as libpq and pgx.** Keeps existing
  connection strings working unchanged, but a string without `sslmode`
  would then connect without authenticating the server, and silently
  in plain text to a server without TLS. Rejected by the owner; a
  managed database whose CA is not in the system roots passes `roots:`
  or names `sslmode` explicitly.
- **Binary result format first.** Faster, but every type needs a binary
  decoder before it can be read at all; text results read every type
  (as Text at worst) from the start.
