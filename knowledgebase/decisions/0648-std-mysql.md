# 0648: MySQL driver for std/sql

Status: Proposed (2026-11-15, aws-43 for the lead). The license route
(item 11: route (a), from MySQL's documentation, go-sql-driver only in
the test oracle) and the TLS default (item 4: `tls=true`, verified) are
the lead's, provisional, on the owner's list to confirm.

## Context

std/sql opens Postgres natively (decision 0278), SQLite through `use c`
(decision 0534) and the in memory database everywhere. MySQL, the other
database most backend services name, has no driver: `sql.open(net,
"mysql", dsn)` reports an unknown driver on every backend, and the Go
backend's `use go` route does not reach native builds, the default since
decision 0248.

Go's ecosystem driver is github.com/go-sql-driver/mysql (v1.9): the
MySQL client/server protocol (protocol 10) over TCP or a Unix socket,
the text and binary result protocols, server side prepared statements,
the four authentication plugins MySQL 8 uses, TLS, multi statements and
multi results, a DSN format every Go service already writes, and
DATETIME and TIMESTAMP read into a time in a chosen location. Its
license is the Mozilla Public License 2.0, not MIT as pgx's is (item 11).

The lead's order (2026-11-15): the driver follows the crypto gap list,
with caching_sha2_password, mysql_native_password and sha256_password,
TLS, prepared statements, the binary protocol, multi results, the DSN,
time and location, a MySQL 8 server in a container on an offload host,
conformance tests and `xo bench --ab`.

## Decision

1. **A driver in Xo, shipped with std, as Postgres's.** `sql.open(net,
   "mysql", dsn)` opens MySQL natively on every native target, through
   std/sql's `Driver` interface (spec/std.md 10.7): the pool,
   transactions, cancellation and column types come from std/sql. The
   code lives in std/native/llrt/mysqlwire.xo (packets, the handshake,
   the text and binary row codecs), mysqlauth.xo (the plugins) and
   mysql.xo (the DriverConn, DSN, values and errors), beside pg.xo and
   pgwire.xo. It adds no effect beyond `net.dial` (`Db[()]`).
2. **The DSN is go-sql-driver's**, so existing connection strings work:
   `[user[:password]@][net[(addr)]]/dbname[?param1=value1&...]`, with
   `tcp(host:port)` (port 3306 by default) and `unix(/path)`. Parameters
   kept: `charset`, `collation` (go-sql-driver's defaults),
   `loc`, `parseTime`, `timeout`, `readTimeout`, `writeTimeout`, `tls`,
   `allowNativePasswords` (default true), `allowCleartextPasswords`,
   `allowOldPasswords` (refused: the pre 4.1 hash is broken),
   `allowPublicKeyRetrieval`, `multiStatements`, `interpolateParams`,
   `clientFoundRows`, `columnsWithAlias`, `maxAllowedPacket`,
   `rejectReadOnly`, `connectionAttributes`, and any other name passed as
   a system variable (`SET name=value` after connecting), as
   go-sql-driver does. Go's registries become arguments of `open`, as
   0278 item 2 did for Postgres: a TLS config registered by name and a
   server public key registered by name are `roots:` (PEM bytes) and
   `server_pub_key:` (PEM bytes) on `sql.open`, and `tls=custom` and
   `serverPubKey=...` refer to them. Go's `Loc` is `loc: time.Zone =
   time.utc()` on `sql.open` (stage B, 2026-11-15, the lead's approval):
   `time.zone(name)` with a name known only at run time embeds the whole
   zone database, and every std/sql program reaches the driver, so the
   DSN's `loc` only names the zone passed, and a mismatch is a `Param`
   error. Size, a program opening MySQL (darwin/arm64): 5,666,225 bytes
   with no zone, 5,715,409 with `time.zone("Europe/Berlin")`, 6,112,129
   with one run time `time.zone(name)` (the database). **Postgres and
   `loc`:** not honoured, and a zone other than UTC is a `Param` error
   for every driver but MySQL. pgx has no zone setting: `timestamptz` is
   an instant (the zone changes only how it prints, and `At` has none),
   and `timestamp` reads its wall clock as UTC (spec/std.md 10.9), so a
   Postgres `loc` would make the driver differ from pgx; a user who
   wants local wall times converts with `time.civil(t, z)`. No file or environment variable is
   read (no `~/.my.cnf`, no `MYSQL_PWD`).
3. **Authentication.** The handshake's plugin, then AuthSwitchRequest
   and AuthMoreData as the server asks:
   - `caching_sha2_password` (MySQL 8's default): the SHA-256 scramble;
     on fast auth failure the full path sends the password in clear over
     TLS or a Unix socket, else RSA-OAEP (SHA-1, as MySQL and
     go-sql-driver) under the server's key: `server_pub_key`, or fetched
     from the server only with `allowPublicKeyRetrieval=true`, as
     go-sql-driver (a fetched key can be a man in the middle's).
     Correction (stage B, 2026-11-15): go-sql-driver v1.10.1 has no
     `allowPublicKeyRetrieval`; it always fetches the key over plain TCP,
     and the name in a DSN becomes a `SET` the server refuses. Xo keeps
     the rule above (MySQL's own clients' and Connector/J's), so a DSN
     that works with go-sql-driver over plain TCP with no key needs
     `allowPublicKeyRetrieval=true` or `server_pub_key:` here; the error
     says so. RSA is OAEP with SHA-1 over llrt rsa (byte-exact against Go's
     rsa.EncryptOAEP, and openssl decrypts it), its seed from llrt rand,
     since a driver holds no `Rand`.
   - `mysql_native_password`: the SHA-1 scramble (refused when
     `allowNativePasswords=false`).
   - `sha256_password`: the same RSA or clear rules.
   - `mysql_clear_password`: only with `allowCleartextPasswords=true`.
   The crypto is std/crypto's (sha1, sha256, encrypt_oaep), so it is
   constant time where std's is and the Go backend runs Go's.
4. **TLS** uses std/tls's `tls.client`, upgrading the connection after
   the server's handshake (the SSLRequest packet), as go-sql-driver.
   `tls=true` verifies the chain and host name, `skip-verify` verifies
   nothing, `preferred` uses TLS when the server offers it without
   verifying, `false` none, `custom` verifies against `roots:`.
   **Owner point:** the default. go-sql-driver's is `false`. This ADR
   proposes `true`, as 0278 chose `verify-full` over libpq's `prefer`:
   a DSN without `tls` gets an authenticated, encrypted connection, and
   one that names `tls` keeps its meaning. The cost is that MySQL 8's
   auto generated self signed certificate fails `true`, so a default
   install needs `tls=custom` with its CA, `tls=skip-verify` or
   `tls=false`; that failure is a `Conn` error whose text says so and
   names `tls=custom` (with `roots:`) and `tls=skip-verify`, when the
   server's certificate is self signed with MySQL's auto generated
   issuer (`MySQL_Server_..._Auto_Generated_CA_Certificate`). A Unix
   socket never uses TLS.
5. **Queries.** Without arguments a statement runs with COM_QUERY and
   the text protocol (so `exec` of a `;` separated schema works with
   `multiStatements=true`). With arguments it is a server side prepared
   statement, COM_STMT_PREPARE, COM_STMT_EXECUTE with binary parameters,
   the binary row protocol, then COM_STMT_CLOSE, which is what
   go-sql-driver does for `db.Query(q, args...)`; `interpolateParams=true`
   instead escapes the arguments into the text, go-sql-driver's
   algorithm, refused for a multi byte character set it cannot escape
   safely, as go-sql-driver.
6. **Prepared statements as an API (stage D, std/sql).** std/sql has no
   `Stmt` today. This stage adds `Db.prepare(q) -> Stmt[fx]` and
   `Tx.prepare`, with `Stmt.query`, `exec` and `close`, and an optional
   `DriverConn.prepare` (a driver without it gets a std/sql fallback
   that runs the text each time). Statements are per connection, as in
   Go's database/sql: a `Stmt` prepares again on each connection it
   runs on. Postgres implements it with a named statement in the same
   stage. A small per connection statement cache (`stmt_cache=N`,
   default 0) is left for later.
7. **Multi results (stage D, std/sql).** With `multiStatements=true` a
   query can return several result sets. `Db.query` and `rows` keep
   returning the first (the rest are read and dropped, so the
   connection stays usable), and a new `Db.query_all(q, args) ->
   List[ResultSet]` returns each set's columns and rows, Go's
   `Rows.NextResultSet`. `DriverRows` gains `next: DriverRows[fx]? =
   None` for the sets after the first; Postgres's simple query fills it
   too. An error in a later set is the query's error (stage A, found by
   scripts/mysql-diff.sh): go-sql-driver reports it only to a caller of
   `NextResultSet`, so `SELECT 1; DELETE ...` whose DELETE fails looks
   like success there; a failed statement is not dropped here.
8. **Values.** By column type: TINYINT to BIGINT and YEAR `Integer`;
   BIGINT UNSIGNED above Int.max `Text` (its decimal digits, nothing
   rounded); DECIMAL `Text`; FLOAT and DOUBLE `Real`; CHAR, VARCHAR,
   TEXT, ENUM and SET `Text`; BINARY, VARBINARY, BLOB, BIT and GEOMETRY
   `Blob`; JSON `JsonText`; NULL `Null`. DATE, DATETIME and TIMESTAMP
   are `At` when `parseTime=true`, read as wall time in `loc` (default
   UTC; `loc=Local` is refused, there is no process time zone without
   `env`, and a name from the embedded zone data is used instead), and
   `Text` otherwise, go-sql-driver's two behaviours; TIME is `Text`
   (it is a duration up to 838 hours, not a time of day), as
   go-sql-driver, which never parses TIME. Zero dates, and years
   outside `Time`'s range (MySQL allows 1000 to 9999, `Time` covers 1678
   to 2262), read as the server's `Text` ("0000-00-00 00:00:00") with
   parseTime too, as the Postgres driver keeps `infinity`: go-sql-driver's
   Go zero time (year 1) is not a `Time` (stage A, found by
   scripts/mysql-diff.sh, where the first draft gave 1970-01-01). Binds: `Flag` as 1 or 0, `At` formatted in `loc` with
   microseconds (trailing zeros dropped, as go-sql-driver), `Blob` as
   bytes, `Array` refused (`Param`). A wall time in `loc` follows Go's
   `time.Date` in a daylight saving gap or overlap (the tests' vectors
   are Go's). Column
   types (decision 0609): go-sql-driver's `ColumnTypeDatabaseTypeName`,
   length, precision and scale for DECIMAL, nullable from the NOT_NULL
   flag.
9. **Errors carry MySQL's code and SQLSTATE.** `Constraint(msg, code,
   name)` for 1062 duplicate entry (`23000`, the key's name), 1451 and
   1452 foreign keys, 1048 not null and 3819 check; `Syntax` for 1064;
   `NoTable` for 1146; `Conn` for 2006 and 2013 and a broken packet
   sequence; `Server(code, msg)` for the rest, code the SQLSTATE and the
   message prefixed with the error number (`Error 1213 (40001): ...`,
   go-sql-driver's text). `rejectReadOnly=true` marks a connection
   broken on 1290 and 1792, as go-sql-driver.
10. **Cancellation, exec results, copy_from.** A cancelled wait closes
    the connection and marks it broken, as go-sql-driver; `KILL QUERY`
    on a second connection, which stops the server's work too, is a
    later step. `ExecResult` comes from the OK packet: affected rows
    (matched rows with `clientFoundRows=true`) and the real
    `last_insert_id`. `copy_from` is a prepared multi row INSERT in
    batches under `maxAllowedPacket`, inside the caller's transaction
    (0534 item 10's rule); LOAD DATA LOCAL INFILE, which needs the
    server's `local_infile`, is not used.
11. **License: owner point.** go-sql-driver is MPL-2.0, a file level
    copyleft: a file made by modifying or translating MPL code stays
    MPL-2.0 and its source must stay available under it. Xo is MIT and
    has no MPL file. Two routes:
    - (a) **Implement from MySQL's protocol documentation** (the client
      server protocol pages of dev.mysql.com, the MySQL Internals
      manual) and use go-sql-driver only as the oracle the tests
      compare against (item 12), not as source. Its DSN grammar,
      parameter names and behaviours are an interface, not code. This
      ADR proposes (a).
    - (b) **Port go-sql-driver's files**, each ported file keeping
      MPL-2.0's notice and listed in THIRD_PARTY_NOTICES, as the lead's
      note assumed ("a port with attribution"). Faster to write, and
      std then ships MPL files beside MIT ones.
12. **Tests.** Native, go (once the Go backend compiles the module,
    item 13) and llvm:
    - Unit: packet round trips, the scrambles of each plugin against
      vectors computed by go-sql-driver's own functions in the oracle
      module, DSN parsing over go-sql-driver's dsn_test.go cases (the
      expected Config of each as JSON from the oracle), escaping
      (interpolateParams) and time formatting and parsing over its
      utils_test.go cases.
    - A fake server on FakeNet for the state machine: each auth path and
      switch, AuthMoreData's fast and full auth, errors mid result, a
      broken sequence, a cancelled wait, multi results.
    - Conformance against a real server: a MySQL 8.0 and an 8.4 LTS
      container in Docker (scripts/mysql-diff.sh, as scripts/pg-diff.sh;
      no real cloud and no credentials beyond the container's own). The
      same statements through Xo and through go-sql-driver in an oracle
      module (scripts/mysqldiff/gomysql, its own go.mod pinned to
      go-sql-driver v1.10.1, not a dependency of xo or std), compared row
      by row and error by error (2026-11-15: 303 statements in six
      passes over verified TLS, interpolated, prepared (stage B's binary
      protocol), prepared with parseTime and clientFoundRows,
      loc=America/New_York prepared and interpolated (with a daylight
      saving gap and overlap), and multiStatements, and 10 tls mode
      outcomes, equal on 8.4 and 8.0):
      every type of item 8 with parseTime on and off and three `loc`
      values, each plugin with TLS, a Unix socket and RSA, prepared and
      interpolated arguments, multi statements and results,
      transactions, `clientFoundRows`, and the error codes of item 9.
    - Performance: `xo bench --ab` against go-sql-driver on the same
      host for point selects, a 10 000 row read, prepared inserts and
      a batched copy_from; reported here before the status is Accepted.
13. **Backends and stages.** Stage A: the wire, auth, DSN and text
    protocol natively. Stage B: prepared statements over the binary
    protocol, TLS, time and location. Stage C: the conformance and
    bench runs on the offload host. Stage D: std/sql's `prepare` and
    `query_all` (items 6 and 7), for every driver. The Go backend
    reports an unknown driver for `"mysql"` until it compiles std/sql's
    Xo drivers, as for Postgres (spec/std.md 10); WebAssembly has no
    `Net`.

## Solid on three axes (decision 0461)

- **Correct:** the oracle is go-sql-driver itself, on a real server, for
  every type, plugin and error the decision names; the fake server
  covers the paths a real one rarely takes.
- **Secure:** TLS on by default (owner point), no public key fetched
  unless asked, no cleartext password over an unencrypted TCP
  connection, `allowOldPasswords` refused, no file or environment read,
  and `Net.only` bounds where it dials.
- **Fast:** prepared statements and binary rows as go-sql-driver,
  measured against it with `--ab` before Accepted.

## Consequences

- Native programs reach MySQL with no C library and no Go toolchain.
- std/sql gains `prepare` and `query_all` for every driver (stage D).
- std carries a second wire protocol to keep current with MySQL
  releases (8.0 reaches end of life in 2026; 8.4 is the LTS line).
- A DSN with no `tls` parameter behaves differently from go-sql-driver
  if the owner keeps the `true` default; spec/std.md says so.

## Alternatives

- **Bind go-sql-driver with `use go`.** The Go backend only.
- **libmysqlclient through `use c`.** GPL with the FOSS exception, a C
  dependency per target, and it reads option files behind the
  capabilities.
- **TLS default `false`, as go-sql-driver.** Existing DSNs connect the
  same, but in plain text with the password exchange exposed to a
  network observer for native auth.
- **Only interpolated arguments (no server side prepare).** One round
  trip fewer per query, but the text protocol reads every value as
  text, and escaping depends on the connection's character set.
- **A new std/mysql module instead of a name `open` knows.** sqlite is
  a module because it binds C (decision 0534); a driver in Xo with no
  effect beyond net.dial sits beside Postgres's, as 0278 did.
