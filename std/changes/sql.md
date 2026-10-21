# std/sql changes

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
