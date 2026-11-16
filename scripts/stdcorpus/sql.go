package main

import "strings"

// std/sql's Row.bytes_opt, Row.ints, Row.strs and sql.float, which no corpus
// reached: std.md 10.4 says the _opt accessors map NULL to None, while a non
// optional accessor reads a NULL as Mismatch, and a name the row does not
// have is NoColumn. The in memory driver is the one the corpus talks to (the
// same on every backend), so the rows need no Go twin: the rule is the
// spec's and every assertion names it.
func sqlCorpus() string {
	var o strings.Builder
	o.WriteString(header("std/sql's Row.bytes_opt, Row.ints, Row.strs and sql.float on the in memory driver: Some for a value (empty included), None for NULL, the Mismatch and NoColumn errors the non optional forms give, and the element type checks of the array accessors (std.md 10.4)"))
	o.WriteString("use std/big\nuse std/json\nuse std/sql\nuse std/time\n")
	o.WriteString(sqlTests)
	o.WriteString(sqlDriverTests)
	o.WriteString(sqlSessionTests)
	o.WriteString(sqlBatchTests)
	o.WriteString(sqlTraceTests)
	o.WriteString(sqlConnTests)
	return o.String()
}

// std/sql's pool and the Driver/DriverConn interfaces (std.md 10.4,
// decision 0278), which no corpus reached (the 2026-10-26 resolved audit).
// sql.memory has no pool and no connection, so it never calls a DriverConn:
// what exercises the interface is a Driver the program writes itself, handed
// to open_driver, which gives it the pool, transactions and cancellation.
const sqlDriverTests = `
struct FakeDriver {
  n: Mutex[Int]
  log: Mutex[List[Str]]
}

struct FakeConn {
  n: Mutex[Int]
  log: Mutex[List[Str]]
  broken: Atomic[Bool]
}

fn record(log: Mutex[List[Str]], what: Str) {
  log.with(fn(var ls: List[Str]) -> () { ls.push(what) })
}

fn (d: FakeDriver) connect(net: Net, dsn: Str) -> sql.DriverConn ! sql.SqlErr uses net.dial {
  record(d.log, "connect")
  if dsn.is_empty() {
    return Err(sql.SqlErr.Conn(msg: "no dsn"))
  }
  Ok(FakeConn{n: d.n, log: d.log, broken: Atomic.new(false)})
}

fn (c: FakeConn) query(q: Str, args: List[sql.SqlValue]) -> sql.DriverRows ! sql.SqlErr uses net.dial {
  record(c.log, "query")
  let n = c.n.with_read(fn(v: Int) -> Int { v })
  Ok(sql.DriverRows{columns: ["n"], rows: [[sql.int(n)]]})
}

fn (c: FakeConn) exec(q: Str, args: List[sql.SqlValue]) -> sql.ExecResult ! sql.SqlErr uses net.dial {
  record(c.log, "exec")
  c.n.with(fn(var v) -> () { v += 1 })
  Ok(sql.ExecResult{rows_affected: 1, last_insert_id: 0})
}

fn (c: FakeConn) begin() -> () ! sql.SqlErr uses net.dial {
  record(c.log, "begin")
  Ok(())
}

fn (c: FakeConn) commit() -> () ! sql.SqlErr uses net.dial {
  record(c.log, "commit")
  Ok(())
}

fn (c: FakeConn) rollback() -> () ! sql.SqlErr uses net.dial {
  record(c.log, "rollback")
  Ok(())
}

fn (c: FakeConn) ping() -> () ! sql.SqlErr uses net.dial {
  record(c.log, "ping")
  Ok(())
}

fn (c: FakeConn) close() uses net.dial {
  record(c.log, "close")
  c.broken.set(true)
}

fn (c: FakeConn) copy_from(table: Str, columns: List[Str], rows: fn() -> List[sql.SqlValue]? ! sql.SqlErr) -> Int ! sql.SqlErr uses net.dial {
  Err(sql.SqlErr.Driver(msg: "no COPY"))
}

fn (c: FakeConn) copy_to(q: Str, out: fn(Bytes) -> () ! sql.SqlErr uses sync) -> Int ! sql.SqlErr uses net.dial {
  Err(sql.SqlErr.Driver(msg: "no COPY"))
}

fn (c: FakeConn) is_broken() -> Bool {
  record(c.log, "is_broken")
  c.broken.get()
}

/// No statements on the server: std/sql runs the text each time.
fn (c: FakeConn) prepare(q: Str) -> sql.DriverStmt? ! sql.SqlErr { None }

/// No LISTEN/NOTIFY and no notices (decision 0648 item 14).
fn (c: FakeConn) wait_notification() -> sql.Notification ! sql.SqlErr uses net.dial {
  Err(sql.SqlErr.Driver(msg: "no LISTEN/NOTIFY"))
}

fn (c: FakeConn) notices() -> List[sql.Notice] { [] }

fn (c: FakeConn) pipeline(batches: List[List[sql.BatchQuery]], in_tx: Bool) -> List[Result[List[sql.DriverResult], sql.SqlErr]] ! sql.SqlErr {
  Err(sql.SqlErr.Driver(msg: "fake: no batches"))
}

test "a Driver the program writes drives Db's pool: query, exec, begin, commit, rollback, ping and close" {
  let n = Mutex.new(0)
  let log: Mutex[List[Str]] = Mutex.new([])
  let db = sql.open_driver(os.net, FakeDriver{n: n, log: log}, "fake:mem", max_open: 1, max_idle: 0)?
  // open_driver connected once before it returned, so connect ran
  expect(db.query_one("SELECT n")?.int("n")?) == 0
  // exec reaches the connection, and the pool gives it back afterwards
  db.exec("BUMP")?
  expect(db.query_one("SELECT n")?.int("n")?) == 1
  db.ping()?
  // a transaction holds its connection until it ends
  let tx = db.begin()?
  tx.exec("BUMP")?
  tx.commit()?
  expect(db.query_one("SELECT n")?.int("n")?) == 2
  let tx2 = db.begin()?
  tx2.exec("BUMP")?
  tx2.rollback()?
  // std/sql hands rollback to the driver, and this driver keeps no
  // transaction log: the row stays, which is the driver's business
  expect(db.query_one("SELECT n")?.int("n")?) == 3
  db.close()?
  // a closed Db refuses further work, so those calls reached the pool's state
  expect(match db.query_one("SELECT n") {
    Ok(_) => "ok"
    Err(e) => e.display()
  }) == "database is closed"
  // every DriverConn method ran, because the driver recorded each call:
  // the pool calls is_broken and then close whenever it does not keep a
  // connection, and max_idle 0 keeps none
  let calls = log.with_read(fn(ls: List[Str]) -> List[Str] { ls })
  for want in ["connect", "query", "exec", "begin", "commit", "rollback", "ping", "close", "is_broken"] {
    expect(calls.contains(want)) == true
  }
}

struct RowItem {
  id: Int
  name: Str
} derive SqlRow

struct CurDriver {
  closed: Mutex[Int]
}

struct CurConn {
  closed: Mutex[Int]
}

/// The rest of a result: ids from next to 5, counting the closes.
struct Cur {
  at: Mutex[Int]
  closed: Mutex[Int]
}

fn (d: CurDriver) connect(net: Net, dsn: Str) -> sql.DriverConn ! sql.SqlErr uses net.dial {
  Ok(CurConn{closed: d.closed})
}

fn (c: CurConn) query(q: Str, args: List[sql.SqlValue]) -> sql.DriverRows ! sql.SqlErr uses net.dial {
  Ok(sql.DriverRows{columns: ["id"], rows: [[sql.int(1)]], cursor: Some(Cur{at: Mutex.new(2), closed: c.closed})})
}

fn (c: CurConn) exec(q: Str, args: List[sql.SqlValue]) -> sql.ExecResult ! sql.SqlErr uses net.dial {
  Ok(sql.ExecResult{rows_affected: 0, last_insert_id: 0})
}

fn (c: CurConn) begin() -> () ! sql.SqlErr uses net.dial { Ok(()) }
fn (c: CurConn) commit() -> () ! sql.SqlErr uses net.dial { Ok(()) }
fn (c: CurConn) rollback() -> () ! sql.SqlErr uses net.dial { Ok(()) }
fn (c: CurConn) ping() -> () ! sql.SqlErr uses net.dial { Ok(()) }
fn (c: CurConn) close() uses net.dial {}
fn (c: CurConn) copy_from(table: Str, columns: List[Str], rows: fn() -> List[sql.SqlValue]? ! sql.SqlErr) -> Int ! sql.SqlErr uses net.dial {
  Err(sql.SqlErr.Driver(msg: "no COPY"))
}

fn (c: CurConn) copy_to(q: Str, out: fn(Bytes) -> () ! sql.SqlErr uses sync) -> Int ! sql.SqlErr uses net.dial {
  Err(sql.SqlErr.Driver(msg: "no COPY"))
}

fn (c: CurConn) is_broken() -> Bool { false }

/// No statements on the server: std/sql runs the text each time.
fn (c: CurConn) prepare(q: Str) -> sql.DriverStmt? ! sql.SqlErr { None }

/// No LISTEN/NOTIFY and no notices (decision 0648 item 14).
fn (c: CurConn) wait_notification() -> sql.Notification ! sql.SqlErr uses net.dial {
  Err(sql.SqlErr.Driver(msg: "no LISTEN/NOTIFY"))
}

fn (c: CurConn) notices() -> List[sql.Notice] { [] }

fn (c: CurConn) pipeline(batches: List[List[sql.BatchQuery]], in_tx: Bool) -> List[Result[List[sql.DriverResult], sql.SqlErr]] ! sql.SqlErr {
  Err(sql.SqlErr.Driver(msg: "fake: no batches"))
}

fn (c: Cur) next() -> List[sql.SqlValue]? ! sql.SqlErr uses net.dial {
  let n = c.at.with(fn(var v) -> Int {
    let was = v
    v += 1
    was
  })
  if n > 5 {
    return Ok(None)
  }
  Ok(Some([sql.int(n)]))
}

fn (c: Cur) rest() -> List[List[sql.SqlValue]] ! sql.SqlErr uses net.dial {
  var out: List[List[sql.SqlValue]] = []
  while {
    match c.next()? {
      Some(r) => out.push(r)
      None => break
    }
  }
  Ok(out)
}

fn (c: Cur) close() uses net.dial {
  c.closed.with(fn(var v) -> () { v += 1 })
}

test "a driver's cursor: query reads it whole, rows as asked, and a one connection pool is free again after the end, a close or an error in each" {
  let closed = Mutex.new(0)
  let db = sql.open_driver(os.net, CurDriver{closed: closed}, "cur:mem", max_open: 1, max_idle: 1)?
  let all = db.query("SELECT id")?
  expect(all.map(fn(r) { r.int("id").ok() ?? -1 })) == [1, 2, 3, 4, 5]
  // rows: the first row came with the result, the rest from the cursor
  let rs = db.rows("SELECT id")?
  expect(rs.columns()) == ["id"]
  var seen: List[Int] = []
  while true {
    match rs.next()? {
      Some(r) => seen.push(r.int("id")?)
      None => break
    }
  }
  expect(seen) == [1, 2, 3, 4, 5]
  // the end released the only connection
  expect(db.query("SELECT id")?.len()) == 5
  // close before the end discards the rest and releases it
  let early = db.rows("SELECT id")?
  expect(early.next()?.is_some()) == true
  early.close()
  early.close()
  expect(db.query("SELECT id")?.len()) == 5
  // each: f's error ends the walk, closes the cursor and is the result
  let r = db.each("SELECT id", [], fn(row) {
    if row.int("id")? == 2 {
      return Err(sql.SqlErr.Driver(msg: "stop at 2"))
    }
  })
  expect(match r {
    Ok(_) => "ok"
    Err(e) => e.display()
  }) == "stop at 2"
  expect(db.query("SELECT id")?.len()) == 5
  db.close()?
}

test "Row.values is the row in column order, and reaches the second of two columns of one name" {
  let db = sql.memory("CREATE TABLE t (id INT, name TEXT)")?
  db.exec("INSERT INTO t (id, name) VALUES ($1, $2)", [sql.int(7), sql.str("n7")])?
  let r = db.query_one("SELECT id, name, name AS id FROM t")?
  expect(r.columns()) == ["id", "name", "id"]
  expect(r.values()) == [sql.SqlValue.Integer(n: 7), sql.SqlValue.Text(s: "n7"), sql.SqlValue.Text(s: "n7")]
  expect(r.value("id")?) == sql.SqlValue.Integer(n: 7)
  db.close()?
}

test "rows, next_as and each on the in memory Db" {
  let db = sql.memory("CREATE TABLE t (id INT, name TEXT)")?
  for i in 1..4 {
    db.exec("INSERT INTO t (id, name) VALUES ($1, $2)", [sql.int(i), sql.str("n${i}")])?
  }
  let rs = db.rows("SELECT id, name FROM t ORDER BY id")?
  expect(rs.columns()) == ["id", "name"]
  expect(rs.next()?.map(fn(r) { r.str("name").ok() ?? "" })) == Some("n1")
  expect(rs.next()?.map(fn(r) { r.str("name").ok() ?? "" })) == Some("n2")
  let third = db.rows("SELECT id, name FROM t ORDER BY id")?
  expect(third.next_as[RowItem]()?.map(fn(i) { i.name })) == Some("n1")
  third.close()
  rs.close()
  // a closed cursor has no more rows
  expect(rs.next()?.is_none()) == true
  let total = Atomic.new(0)
  db.each("SELECT id FROM t", [], fn(row) {
    total.add(row.int("id")?)
  })?
  expect(total.get()) == 6
  let tx = db.begin()?
  let trs = tx.rows("SELECT id FROM t ORDER BY id")?
  expect(trs.next()?.map(fn(r) { r.int("id").ok() ?? 0 })) == Some(1)
  trs.close()
  tx.commit()?
}

test "copy_from loads rows on the in memory Db and in a transaction, quote_ident quotes, and a driver without COPY says so" {
  let db = sql.memory("CREATE TABLE t (id INT, name TEXT)")?
  let at = Atomic.new(0)
  let n = db.copy_from("t", ["id", "name"], fn() {
    let i = at.add(1)
    if i > 3 {
      return Ok(None)
    }
    Ok(Some([sql.int(i), sql.str("n${i}")]))
  })?
  expect(n) == 3
  let tx = db.begin()?
  let at2 = Atomic.new(10)
  expect(tx.copy_from("t", ["id", "name"], fn() {
    let i = at2.add(1)
    if i > 11 {
      return Ok(None)
    }
    Ok(Some([sql.int(i), sql.str("m${i}")]))
  })?) == 1
  tx.commit()?
  expect(db.query("SELECT id FROM t")?.len()) == 4
  // an error from rows is returned
  let bad = db.copy_from("t", ["id"], fn() { Err(sql.SqlErr.Driver(msg: "boom")) })
  expect(match bad {
    Ok(_) => "ok"
    Err(e) => e.display()
  }) == "boom"
  expect(sql.quote_ident("a")) == "\"a\""
  expect(sql.quote_ident("a\"b")) == "\"a\"\"b\""
  let log: Mutex[List[Str]] = Mutex.new([])
  let fake = sql.open_driver(os.net, FakeDriver{n: Mutex.new(0), log: log}, "fake:mem", max_open: 1)?
  expect(match fake.copy_from("t", ["id"], fn() { Ok(None) }) {
    Ok(_) => "ok"
    Err(e) => e.display()
  }) == "no COPY"
  fake.close()?
}

test "a transaction on the in memory Db commits one row and rolls another back" {
  let db = sql.memory("CREATE TABLE t (id SERIAL, n INT)")?
  db.ping()?
  let tx = db.begin()?
  tx.exec("INSERT INTO t (n) VALUES ($1)", [sql.int(1)])?
  tx.commit()?
  let tx2 = db.begin()?
  tx2.exec("INSERT INTO t (n) VALUES ($1)", [sql.int(2)])?
  tx2.rollback()?
  // the committed row is there, the rolled back one is not
  let rows = db.query("SELECT n FROM t ORDER BY id")?
  expect(rows.len()) == 1
  expect(rows[0].int("n")?) == 1
  // a transaction reads through its own state
  let tx3 = db.begin()?
  expect(tx3.query("SELECT n FROM t")?.len()) == 1
  expect(tx3.query_one("SELECT n FROM t")?.int("n")?) == 1
  tx3.rollback()?
  db.close()?
}

/// A driver that records each statement and refuses a SET TRANSACTION
/// naming READ UNCOMMITTED, as a server without that level would.
struct ModeDriver {
  log: Mutex[List[Str]]
}

struct ModeConn {
  log: Mutex[List[Str]]
}

fn (d: ModeDriver) connect(net: Net, dsn: Str) -> sql.DriverConn ! sql.SqlErr uses net.dial {
  Ok(ModeConn{log: d.log})
}

fn (c: ModeConn) query(q: Str, args: List[sql.SqlValue]) -> sql.DriverRows ! sql.SqlErr uses net.dial {
  record(c.log, q)
  Ok(sql.DriverRows{columns: [], rows: []})
}

fn (c: ModeConn) exec(q: Str, args: List[sql.SqlValue]) -> sql.ExecResult ! sql.SqlErr uses net.dial {
  record(c.log, q)
  if q.contains("UNCOMMITTED") {
    return Err(sql.SqlErr.Server(code: "0A000", msg: "unsupported isolation level"))
  }
  Ok(sql.ExecResult{rows_affected: 0, last_insert_id: 0})
}

fn (c: ModeConn) begin() -> () ! sql.SqlErr uses net.dial {
  record(c.log, "BEGIN")
  Ok(())
}

fn (c: ModeConn) commit() -> () ! sql.SqlErr uses net.dial {
  record(c.log, "COMMIT")
  Ok(())
}

fn (c: ModeConn) rollback() -> () ! sql.SqlErr uses net.dial {
  record(c.log, "ROLLBACK")
  Ok(())
}

fn (c: ModeConn) ping() -> () ! sql.SqlErr uses net.dial { Ok(()) }

fn (c: ModeConn) close() uses net.dial {}

fn (c: ModeConn) copy_from(table: Str, columns: List[Str], rows: fn() -> List[sql.SqlValue]? ! sql.SqlErr) -> Int ! sql.SqlErr uses net.dial {
  Err(sql.SqlErr.Driver(msg: "no COPY"))
}

fn (c: ModeConn) copy_to(q: Str, out: fn(Bytes) -> () ! sql.SqlErr uses sync) -> Int ! sql.SqlErr uses net.dial {
  Err(sql.SqlErr.Driver(msg: "no COPY"))
}

fn (c: ModeConn) is_broken() -> Bool { false }

/// No statements on the server: std/sql runs the text each time.
fn (c: ModeConn) prepare(q: Str) -> sql.DriverStmt? ! sql.SqlErr { None }

/// No LISTEN/NOTIFY and no notices (decision 0648 item 14).
fn (c: ModeConn) wait_notification() -> sql.Notification ! sql.SqlErr uses net.dial {
  Err(sql.SqlErr.Driver(msg: "no LISTEN/NOTIFY"))
}

fn (c: ModeConn) notices() -> List[sql.Notice] { [] }

fn (c: ModeConn) pipeline(batches: List[List[sql.BatchQuery]], in_tx: Bool) -> List[Result[List[sql.DriverResult], sql.SqlErr]] ! sql.SqlErr {
  Err(sql.SqlErr.Driver(msg: "fake: no batches"))
}

test "begin's isolation and read_only reach a driver as SET TRANSACTION after BEGIN" {
  let log: Mutex[List[Str]] = Mutex.new([])
  let db = sql.open_driver(os.net, ModeDriver{log: log}, "mode", max_open: 1)?
  let cases: List[(sql.Isolation, Bool, Str)] = [
    (sql.Isolation.Default, false, "BEGIN|COMMIT"),
    (sql.Isolation.Default, true, "BEGIN|SET TRANSACTION READ ONLY|COMMIT"),
    (sql.Isolation.ReadCommitted, false, "BEGIN|SET TRANSACTION ISOLATION LEVEL READ COMMITTED|COMMIT"),
    (sql.Isolation.RepeatableRead, true, "BEGIN|SET TRANSACTION ISOLATION LEVEL REPEATABLE READ, READ ONLY|COMMIT"),
    (sql.Isolation.Serializable, false, "BEGIN|SET TRANSACTION ISOLATION LEVEL SERIALIZABLE|COMMIT"),
  ]
  for (iso, ro, want) in cases {
    log.with(fn(var ls: List[Str]) -> () { ls = [] })
    let tx = db.begin(isolation: iso, read_only: ro)?
    tx.commit()?
    expect((iso, ro, log.with_read(fn(ls: List[Str]) -> Str { ls.join("|") }))) == (iso, ro, want)
  }
  // deferrable (pgx's TxOptions.DeferrableMode) comes last, as in pgx's BEGIN
  log.with(fn(var ls: List[Str]) -> () { ls = [] })
  let txd = db.begin(isolation: sql.Isolation.Serializable, read_only: true, deferrable: true)?
  txd.commit()?
  expect(log.with_read(fn(ls: List[Str]) -> Str { ls.join("|") })) == "BEGIN|SET TRANSACTION ISOLATION LEVEL SERIALIZABLE, READ ONLY, DEFERRABLE|COMMIT"
  log.with(fn(var ls: List[Str]) -> () { ls = [] })
  let txd2 = db.begin(deferrable: true)?
  txd2.rollback()?
  expect(log.with_read(fn(ls: List[Str]) -> Str { ls.join("|") })) == "BEGIN|SET TRANSACTION DEFERRABLE|ROLLBACK"
  // a refused mode rolls back, returns the server's error and gives the
  // connection back (max_open 1: the next begin would wait forever)
  log.with(fn(var ls: List[Str]) -> () { ls = [] })
  expect(match db.begin(isolation: sql.Isolation.ReadUncommitted) {
    Ok(_) => "ok"
    Err(e) => e.display()
  }) == "unsupported isolation level (SQLSTATE 0A000)"
  expect(log.with_read(fn(ls: List[Str]) -> Str { ls.join("|") })) == "BEGIN|SET TRANSACTION ISOLATION LEVEL READ UNCOMMITTED|ROLLBACK"
  let tx = db.begin()?
  tx.rollback()?
  db.close()?
}

test "a read only transaction on the in memory Db reads and refuses writes as Postgres does" {
  let db = sql.memory("CREATE TABLE t (id SERIAL, n INT)")?
  db.exec("INSERT INTO t (n) VALUES (1)")?
  let tx = db.begin(read_only: true)?
  expect(tx.query_one("SELECT n FROM t")?.int("n")?) == 1
  for q in ["INSERT INTO t (n) VALUES (2)", "  update t SET n = 3", "DELETE FROM t", "CREATE TABLE u (id INT)"] {
    expect(match tx.exec(q) {
      Ok(_) => "ok"
      Err(Server(code, msg)) => "${code} ${msg}"
      Err(e) => e.display()
    }) == "25006 cannot execute ${q.trim().split(" ")[0].to_upper()} in a read-only transaction"
  }
  tx.commit()?
  expect(db.query("SELECT n FROM t")?.len()) == 1
  // every level runs: the in memory Db is serializable whatever is asked
  for iso in [sql.Isolation.ReadUncommitted, sql.Isolation.ReadCommitted, sql.Isolation.RepeatableRead, sql.Isolation.Serializable] {
    let t2 = db.begin(isolation: iso)?
    t2.exec("INSERT INTO t (n) VALUES (4)")?
    t2.rollback()?
  }
  expect(db.query("SELECT n FROM t")?.len()) == 1
  db.close()?
}
`

const sqlTests = `
fn hex_opt(b: Bytes?) -> Str {
  match b {
    Some(x) => x.to_hex()
    None => "none"
  }
}

test "Row.bytes_opt is Some for a blob and None for NULL" {
  let db = sql.memory("CREATE TABLE b (id SERIAL, data BYTEA)")?
  db.exec("INSERT INTO b (data) VALUES ($1)", [sql.bytes(b"\x00\x01\xfe")])?
  db.exec("INSERT INTO b (data) VALUES ($1)", [sql.bytes(b"")])?
  db.exec("INSERT INTO b (data) VALUES ($1)", [sql.null()])?
  let rows = db.query("SELECT id, data FROM b ORDER BY id")?
  expect(hex_opt(rows[0].bytes_opt("data")?)) == "0001fe"
  expect(hex_opt(rows[1].bytes_opt("data")?)) == ""
  expect(hex_opt(rows[2].bytes_opt("data")?)) == "none"
  expect(rows[2].is_null("data")?) == true
  expect(rows[0].is_null("data")?) == false
}

test "a non optional bytes on NULL is Mismatch, and a missing column is NoColumn" {
  let db = sql.memory("CREATE TABLE b (id SERIAL, data BYTEA)")?
  db.exec("INSERT INTO b (data) VALUES ($1)", [sql.null()])?
  let row = db.query_one("SELECT id, data FROM b")?
  expect(match row.bytes("data") {
    Ok(_) => "ok"
    Err(Mismatch(col, _)) => "mismatch ${col}"
    Err(_) => "other"
  }) == "mismatch data"
  expect(match row.bytes_opt("nope") {
    Ok(_) => "ok"
    Err(NoColumn(name)) => "no column ${name}"
    Err(_) => "other"
  }) == "no column nope"
  expect(match row.bytes("nope") {
    Ok(_) => "ok"
    Err(NoColumn(name)) => "no column ${name}"
    Err(_) => "other"
  }) == "no column nope"
}

test "sql.float binds a Float and Row.float reads it back" {
  let db = sql.memory("CREATE TABLE f (id SERIAL, x REAL)")?
  db.exec("INSERT INTO f (x) VALUES ($1)", [sql.float(2.5)])?
  db.exec("INSERT INTO f (x) VALUES ($1)", [sql.float(-0.5)])?
  db.exec("INSERT INTO f (x) VALUES ($1)", [sql.float(1e300)])?
  db.exec("INSERT INTO f (x) VALUES ($1)", [sql.null()])?
  let rows = db.query("SELECT id, x FROM f ORDER BY id")?
  expect(rows[0].float("x")?) == 2.5
  expect(rows[1].float("x")?) == -0.5
  expect(rows[2].float("x")?) == 1e300
  // std.md 10.4: the optional form maps NULL to None, the plain one is
  // Mismatch; a value bound as a Float reads back as the same Float.
  expect(rows[3].float_opt("x")?) == None
  expect(match rows[3].float("x") {
    Ok(_) => "ok"
    Err(Mismatch(col, _)) => "mismatch ${col}"
    Err(_) => "other"
  }) == "mismatch x"
}

test "Row.ints and Row.strs read an array column, and a wrong element type is Mismatch" {
  let db = sql.memory("CREATE TABLE a (id SERIAL, nums INT[], names TEXT[])")?
  db.exec("INSERT INTO a (nums, names) VALUES ($1, $2)", [sql.array([sql.int(1), sql.int(-2), sql.int(300)]), sql.array([sql.str("x"), sql.str("")])])?
  db.exec("INSERT INTO a (nums, names) VALUES ($1, $2)", [sql.array([]), sql.array([])])?
  let rows = db.query("SELECT id, nums, names FROM a ORDER BY id")?
  let ns = rows[0].ints("nums")?
  expect(ns.len()) == 3
  expect(ns[0]) == 1
  expect(ns[1]) == -2
  expect(ns[2]) == 300
  let ss = rows[0].strs("names")?
  expect(ss.len()) == 2
  expect(ss[0]) == "x"
  expect(ss[1]) == ""
  // an empty array is an empty list, not an error
  expect(rows[1].ints("nums")?.len()) == 0
  expect(rows[1].strs("names")?.len()) == 0
  // each accessor checks the elements: a text element is not an integer and
  // the other way round, and the error names the column (std.md 10.4)
  expect(match rows[0].ints("names") {
    Ok(_) => "ok"
    Err(Mismatch(col, _)) => "mismatch ${col}"
    Err(_) => "other"
  }) == "mismatch names"
  expect(match rows[0].strs("nums") {
    Ok(_) => "ok"
    Err(Mismatch(col, _)) => "mismatch ${col}"
    Err(_) => "other"
  }) == "mismatch nums"
}
`

// std/sql batch S1 (decision 0648 item 14): Db.conn's Conn, Tx.begin's
// savepoints, LISTEN/NOTIFY and open_driver's on_notice, through a driver
// the program writes (the same on every backend), and on the in memory Db.
// The savepoint texts and their order are pgx v5.12.0's (tx.go dbTx.Begin,
// dbSimulatedNestedTx.Commit and Rollback); the nested commit and rollback
// tests port tx_test.go's TestTxNestedTransactionCommit and
// TestTxNestedTransactionRollback.
const sqlSessionTests = `
struct SessDriver {
  connects: Atomic[Int]
  log: Mutex[List[Str]]
}

struct SessConn {
  id: Int
  log: Mutex[List[Str]]
  notes: Mutex[List[sql.Notice]]
  notifs: Mutex[List[sql.Notification]]
}

fn (d: SessDriver) connect(net: Net, dsn: Str) -> sql.DriverConn ! sql.SqlErr uses net.dial {
  Ok(SessConn{id: d.connects.add(1), log: d.log, notes: Mutex.new([]), notifs: Mutex.new([])})
}

fn (c: SessConn) note(q: Str) {
  record(c.log, "${c.id} ${q}")
}

fn (c: SessConn) query(q: Str, args: List[sql.SqlValue]) -> sql.DriverRows ! sql.SqlErr uses net.dial {
  c.note(q)
  Ok(sql.DriverRows{columns: ["conn"], rows: [[sql.int(c.id)]]})
}

/// "NOTIFY ch payload" keeps a notification from backend 4242, "RAISE
/// msg" a notice and "WARN msg" a warning, as a server would send them.
fn (c: SessConn) exec(q: Str, args: List[sql.SqlValue]) -> sql.ExecResult ! sql.SqlErr uses net.dial {
  c.note(q)
  let w = q.split(" ")
  if w[0] == "NOTIFY" {
    c.notifs.with(fn(var xs: List[sql.Notification]) -> () { xs.push(sql.Notification{pid: 4242, channel: w[1], payload: w[2]}) })
  }
  if w[0] == "RAISE" {
    c.notes.with(fn(var xs: List[sql.Notice]) -> () { xs.push(sql.Notice{severity: "NOTICE", code: "00000", message: w[1]}) })
  }
  if w[0] == "WARN" {
    c.notes.with(fn(var xs: List[sql.Notice]) -> () { xs.push(sql.Notice{severity: "WARNING", code: "01000", message: w[1], hint: "h"}) })
  }
  Ok(sql.ExecResult{rows_affected: 0, last_insert_id: 0})
}

fn (c: SessConn) begin() -> () ! sql.SqlErr uses net.dial {
  c.note("BEGIN")
  Ok(())
}

fn (c: SessConn) commit() -> () ! sql.SqlErr uses net.dial {
  c.note("COMMIT")
  Ok(())
}

fn (c: SessConn) rollback() -> () ! sql.SqlErr uses net.dial {
  c.note("ROLLBACK")
  Ok(())
}

fn (c: SessConn) ping() -> () ! sql.SqlErr uses net.dial { Ok(()) }

fn (c: SessConn) close() uses net.dial {}

fn (c: SessConn) copy_from(table: Str, columns: List[Str], rows: fn() -> List[sql.SqlValue]? ! sql.SqlErr) -> Int ! sql.SqlErr uses net.dial {
  Err(sql.SqlErr.Driver(msg: "no COPY"))
}

fn (c: SessConn) copy_to(q: Str, out: fn(Bytes) -> () ! sql.SqlErr uses sync) -> Int ! sql.SqlErr uses net.dial {
  Err(sql.SqlErr.Driver(msg: "no COPY"))
}

fn (c: SessConn) is_broken() -> Bool { false }

fn (c: SessConn) prepare(q: Str) -> sql.DriverStmt? ! sql.SqlErr { None }

/// The oldest notification kept; a fake has nothing to wait for, so none
/// is Driver.
fn (c: SessConn) wait_notification() -> sql.Notification ! sql.SqlErr uses net.dial {
  let got = c.notifs.with(fn(var xs: List[sql.Notification]) -> sql.Notification? {
    if xs.is_empty() {
      return None
    }
    Some(xs.remove_at(0))
  })
  match got {
    Some(n) => Ok(n)
    None => Err(sql.SqlErr.Driver(msg: "no notification"))
  }
}

/// Notes the pipeline and each query; a query "FAIL ..." fails its batch
/// (the rest of it is not run), "RAISE msg" keeps a notice; each other
/// query gives the connection's id and a count of 1.
fn (c: SessConn) pipeline(batches: List[List[sql.BatchQuery]], in_tx: Bool) -> List[Result[List[sql.DriverResult], sql.SqlErr]] ! sql.SqlErr uses net.dial {
  c.note("PIPELINE ${batches.len()} in_tx=${in_tx}")
  var out: List[Result[List[sql.DriverResult], sql.SqlErr]] = []
  for b in batches {
    var sets: List[sql.DriverResult] = []
    var failed: sql.SqlErr? = None
    for x in b {
      c.note(x.query)
      if x.query.starts_with("FAIL") {
        failed = Some(sql.SqlErr.Server(code: "42000", msg: x.query))
        break
      }
      if x.query.starts_with("RAISE ") {
        c.notes.with(fn(var xs: List[sql.Notice]) -> () { xs.push(sql.Notice{severity: "NOTICE", code: "00000", message: x.query[6..]}) })
      }
      sets.push(sql.DriverResult{columns: ["conn"], rows: [[sql.int(c.id)]], rows_affected: 1})
    }
    out.push(match failed {
      Some(e) => Err(e)
      None => Ok(sets)
    })
  }
  out
}

fn (c: SessConn) notices() -> List[sql.Notice] {
  c.notes.with(fn(var xs: List[sql.Notice]) -> List[sql.Notice] {
    let all = xs
    xs = []
    all
  })
}

fn sess_log(log: Mutex[List[Str]]) -> Str {
  log.with(fn(var ls: List[Str]) -> Str {
    let s = ls.join("|")
    ls = []
    s
  })
}

test "Db.conn holds one connection across calls, a second Conn takes another, and close gives it back; Closed after close" {
  let d = SessDriver{connects: Atomic.new(0), log: Mutex.new([])}
  let db = sql.open_driver(os.net, d, "sess", max_open: 2)?
  let c = db.conn()?
  expect(c.query_one("SELECT conn")?.int("conn")?) == 1
  c.exec("SET search_path TO x")?
  expect(c.query_one("SELECT conn")?.int("conn")?) == 1
  let c2 = db.conn()?
  expect(c2.query_one("SELECT conn")?.int("conn")?) == 2
  expect(sess_log(d.log)) == "1 SELECT conn|1 SET search_path TO x|1 SELECT conn|2 SELECT conn"
  c2.close()
  c2.close()
  expect(c2.exec("SELECT 1").err()) == Some(sql.SqlErr.Closed)
  expect(c2.query("SELECT 1").err()) == Some(sql.SqlErr.Closed)
  expect(c2.begin().err()) == Some(sql.SqlErr.Closed)
  // two permits: c holds one, so the Db runs on the one c2 gave back
  expect(db.query_one("SELECT conn")?.int("conn")?) == 2
  c.close()
  expect(d.connects.get()) == 2
  db.close()?
}

test "a Conn's transaction keeps the connection, and close rolls back one left open" {
  let d = SessDriver{connects: Atomic.new(0), log: Mutex.new([])}
  let db = sql.open_driver(os.net, d, "sess", max_open: 1)?
  let c = db.conn()?
  let tx = c.begin(isolation: sql.Isolation.Serializable, read_only: true, deferrable: true)?
  tx.exec("INSERT 1")?
  tx.commit()?
  expect(tx.commit().err()) == Some(sql.SqlErr.TxDone)
  c.exec("SELECT after")?
  let open = c.begin()?
  open.exec("INSERT 2")?
  c.close()
  expect(open.exec("INSERT 3").err()) == Some(sql.SqlErr.TxDone)
  expect(sess_log(d.log)) == "1 BEGIN|1 SET TRANSACTION ISOLATION LEVEL SERIALIZABLE, READ ONLY, DEFERRABLE|1 INSERT 1|1 COMMIT|1 SELECT after|1 BEGIN|1 INSERT 2|1 ROLLBACK"
  // max_open 1: the connection is back in the pool
  db.exec("SELECT pool")?
  db.close()?
}

test "pgx TestTxNestedTransactionCommit: savepoints sp_1 and sp_2 inside one transaction, released in order" {
  let d = SessDriver{connects: Atomic.new(0), log: Mutex.new([])}
  let db = sql.open_driver(os.net, d, "sess", max_open: 1)?
  let tx = db.begin()?
  tx.exec("insert into foo(id) values (1)")?
  let nested = tx.begin()?
  nested.exec("insert into foo(id) values (2)")?
  let double = nested.begin()?
  double.exec("insert into foo(id) values (3)")?
  double.commit()?
  nested.commit()?
  tx.commit()?
  expect(sess_log(d.log)) == "1 BEGIN|1 insert into foo(id) values (1)|1 savepoint sp_1|1 insert into foo(id) values (2)|1 savepoint sp_2|1 insert into foo(id) values (3)|1 release savepoint sp_2|1 release savepoint sp_1|1 COMMIT"
  db.close()?
}

test "pgx TestTxNestedTransactionRollback: a savepoint's rollback, then the outer transaction goes on; an outer end ends the nested one" {
  let d = SessDriver{connects: Atomic.new(0), log: Mutex.new([])}
  let db = sql.open_driver(os.net, d, "sess", max_open: 1)?
  let tx = db.begin()?
  tx.exec("insert into foo(id) values (1)")?
  let nested = tx.begin()?
  nested.exec("insert into foo(id) values (2)")?
  nested.rollback()?
  expect(nested.exec("insert into foo(id) values (9)").err()) == Some(sql.SqlErr.TxDone)
  tx.exec("insert into foo(id) values (3)")?
  let left = tx.begin()?
  tx.commit()?
  expect(left.exec("insert into foo(id) values (4)").err()) == Some(sql.SqlErr.TxDone)
  expect(left.commit().err()) == Some(sql.SqlErr.TxDone)
  left.rollback()?
  expect(sess_log(d.log)) == "1 BEGIN|1 insert into foo(id) values (1)|1 savepoint sp_1|1 insert into foo(id) values (2)|1 rollback to savepoint sp_1|1 insert into foo(id) values (3)|1 savepoint sp_2|1 COMMIT"
  db.close()?
}

test "pgx TestTxNestedTransactionCommit and Rollback on the in memory Db" {
  let db = sql.memory("CREATE TABLE foo (id INT UNIQUE)")?
  let tx = db.begin()?
  tx.exec("INSERT INTO foo (id) VALUES (1)")?
  let nested = tx.begin()?
  nested.exec("INSERT INTO foo (id) VALUES (2)")?
  let double = nested.begin()?
  double.exec("INSERT INTO foo (id) VALUES (3)")?
  double.commit()?
  nested.commit()?
  expect(db.query("SELECT id FROM foo")?.len()) == 0
  tx.commit()?
  expect(db.query("SELECT id FROM foo")?.len()) == 3
  let tx2 = db.begin()?
  tx2.exec("INSERT INTO foo (id) VALUES (4)")?
  let n2 = tx2.begin()?
  n2.exec("INSERT INTO foo (id) VALUES (5)")?
  expect(n2.query("SELECT id FROM foo")?.len()) == 5
  n2.rollback()?
  expect(tx2.query("SELECT id FROM foo")?.len()) == 4
  tx2.exec("INSERT INTO foo (id) VALUES (6)")?
  tx2.commit()?
  expect(db.query("SELECT id FROM foo")?.len()) == 5
  db.close()?
}

test "a Conn on the in memory Db runs on the Db, has no LISTEN/NOTIFY, and is Closed after close" {
  let db = sql.memory("CREATE TABLE t (id INT)")?
  let c = db.conn()?
  c.exec("INSERT INTO t (id) VALUES (1)")?
  expect(db.query("SELECT id FROM t")?.len()) == 1
  let tx = c.begin()?
  tx.exec("INSERT INTO t (id) VALUES (2)")?
  tx.commit()?
  expect(c.query("SELECT id FROM t")?.len()) == 2
  c.ping()?
  expect(c.listen("ch").err().map(fn(e: sql.SqlErr) -> Bool { e is Driver(..) })) == Some(true)
  expect(c.wait_notification().err().map(fn(e: sql.SqlErr) -> Bool { e is Driver(..) })) == Some(true)
  c.close()
  expect(c.query("SELECT id FROM t").err()) == Some(sql.SqlErr.Closed)
  db.close()?
}

test "LISTEN and UNLISTEN quote the channel, notifications are kept in order, and close runs UNLISTEN *" {
  let d = SessDriver{connects: Atomic.new(0), log: Mutex.new([])}
  let db = sql.open_driver(os.net, d, "sess", max_open: 1)?
  let c = db.conn()?
  c.listen("chat")?
  c.listen("we\"ird")?
  c.exec("NOTIFY chat hello")?
  c.exec("NOTIFY chat again")?
  expect(c.wait_notification()?) == sql.Notification{pid: 4242, channel: "chat", payload: "hello"}
  expect(c.wait_notification()?.payload) == "again"
  expect(c.wait_notification().err()) == Some(sql.SqlErr.Driver(msg: "no notification"))
  c.unlisten("chat")?
  c.close()
  expect(c.wait_notification().err()) == Some(sql.SqlErr.Closed)
  expect(sess_log(d.log)) == "1 LISTEN \"chat\"|1 LISTEN \"we\"\"ird\"|1 NOTIFY chat hello|1 NOTIFY chat again|1 UNLISTEN \"chat\"|1 UNLISTEN *"
  db.close()?
}

test "on_notice gets each notice in order after the call that received it (pgconn TestConnOnNotice)" {
  let d = SessDriver{connects: Atomic.new(0), log: Mutex.new([])}
  let got: Mutex[List[sql.Notice]] = Mutex.new([])
  let db = sql.open_driver(os.net, d, "sess", max_open: 1, on_notice: Some(fn(n: sql.Notice) { got.with(fn(var xs: List[sql.Notice]) -> () { xs.push(n) }) }))?
  db.exec("RAISE hello,world")?
  expect(got.with_read(fn(xs: List[sql.Notice]) -> List[Str] { xs.map(fn(n: sql.Notice) -> Str { "${n.severity} ${n.message}" }) })) == ["NOTICE hello,world"]
  let tx = db.begin()?
  tx.exec("WARN careful")?
  tx.exec("RAISE two")?
  tx.commit()?
  let c = db.conn()?
  c.exec("RAISE three")?
  c.close()
  let all = got.with_read(fn(xs: List[sql.Notice]) -> List[sql.Notice] { xs })
  expect(all.map(fn(n: sql.Notice) -> Str { n.message })) == ["hello,world", "careful", "two", "three"]
  expect(all[1]) == sql.Notice{severity: "WARNING", code: "01000", message: "careful", hint: "h"}
  db.close()?
}
`

// std/sql batch S2 (decision 0648 item 15): batch and pipeline (pgx's
// SendBatch, pgconn's Pipeline) on the in memory Db, whose batches are
// atomic on a copy of the tables, and through a Driver the program writes,
// which shows what DriverConn.pipeline is given (in_tx) and that its
// notices reach on_notice.
const sqlBatchTests = `
fn affected(sets: List[sql.ResultSet]) -> List[Int] {
  sets.map(fn(s: sql.ResultSet) -> Int { s.rows_affected() })
}

test "batch on the in memory Db: a set per query in order with its rows and count, RETURNING rows, and an empty batch" {
  let db = sql.memory("CREATE TABLE t (id SERIAL, n INT NOT NULL)")?
  let sets = db.batch([
    sql.BatchQuery{query: "INSERT INTO t (n) VALUES ($1)", args: [sql.int(1)]},
    sql.BatchQuery{query: "INSERT INTO t (n) VALUES ($1), ($2)", args: [sql.int(2), sql.int(3)]},
    sql.BatchQuery{query: "SELECT n FROM t ORDER BY n"},
    sql.BatchQuery{query: "UPDATE t SET n = n + 10 WHERE n > $1", args: [sql.int(1)]},
    sql.BatchQuery{query: "DELETE FROM t WHERE n = 1 RETURNING n"},
  ])?
  expect(sets.len()) == 5
  expect(affected(sets)) == [1, 2, 3, 2, 1]
  expect(sets[0].columns()) == []
  expect(sets[0].rows().len()) == 0
  expect(sets[2].columns()) == ["n"]
  expect(sets[2].rows().map(fn(r: sql.Row) -> Int { r.int("n").ok() ?? -1 })) == [1, 2, 3]
  expect(sets[4].rows()[0].int("n")?) == 1
  expect(db.query("SELECT n FROM t ORDER BY n")?.map(fn(r: sql.Row) -> Int { r.int("n").ok() ?? -1 })) == [12, 13]
  expect(db.batch([])?.len()) == 0
  db.close()?
  expect(db.batch([]).err()) == Some(sql.SqlErr.Closed)
}

test "a batch is atomic: an error rolls back the queries before it and is the call's; one query is one statement" {
  let db = sql.memory("CREATE TABLE t (id SERIAL, n INT NOT NULL)")?
  let r = db.batch([
    sql.BatchQuery{query: "INSERT INTO t (n) VALUES (1)"},
    sql.BatchQuery{query: "INSERT INTO nope (n) VALUES (2)"},
    sql.BatchQuery{query: "INSERT INTO t (n) VALUES (3)"},
  ])
  expect(r.err()) == Some(sql.SqlErr.NoTable(name: "nope"))
  expect(db.query("SELECT n FROM t")?.len()) == 0
  let two = db.batch([sql.BatchQuery{query: "INSERT INTO t (n) VALUES (1); INSERT INTO t (n) VALUES (2)"}])
  expect(two.err()) == Some(sql.SqlErr.Syntax(msg: "expected one statement"))
  let nn = db.batch([sql.BatchQuery{query: "INSERT INTO t (n) VALUES (1)"}, sql.BatchQuery{query: "INSERT INTO t (n) VALUES (NULL)"}])
  expect(nn.is_err()) == true
  expect(db.query("SELECT n FROM t")?.len()) == 0
  db.close()?
}

test "pipeline on the in memory Db: each batch atomic, a failed one does not stop the next, which sees the first's writes" {
  let db = sql.memory("CREATE TABLE t (id SERIAL, n INT NOT NULL)")?
  let outs = db.pipeline([
    [sql.BatchQuery{query: "INSERT INTO t (n) VALUES (1)"}],
    [sql.BatchQuery{query: "INSERT INTO t (n) VALUES (2)"}, sql.BatchQuery{query: "SELECT nope FROM t"}],
    [],
    [sql.BatchQuery{query: "SELECT n FROM t"}],
  ])?
  expect(outs.len()) == 4
  expect(outs[0].is_ok()) == true
  expect(outs[1].err()) == Some(sql.SqlErr.Syntax(msg: "no such column nope"))
  expect((outs[2].ok() ?? []).len()) == 0
  let last = outs[3].ok() ?? []
  expect(last[0].rows().map(fn(r: sql.Row) -> Int { r.int("n").ok() ?? -1 })) == [1]
  expect(db.pipeline([])?.len()) == 0
  db.close()?
}

test "Tx.batch on the in memory Db: inside the transaction, a failed batch changes nothing, read only refuses writes, TxDone after the end" {
  let db = sql.memory("CREATE TABLE t (id SERIAL, n INT NOT NULL)")?
  let tx = db.begin()?
  let sets = tx.batch([sql.BatchQuery{query: "INSERT INTO t (n) VALUES (1)"}, sql.BatchQuery{query: "SELECT n FROM t"}])?
  expect(affected(sets)) == [1, 1]
  expect(tx.batch([sql.BatchQuery{query: "INSERT INTO t (n) VALUES (2)"}, sql.BatchQuery{query: "INSERT INTO t (n) VALUES (NULL)"}]).is_err()) == true
  expect(tx.query("SELECT n FROM t")?.len()) == 1
  expect(db.query("SELECT n FROM t")?.len()) == 0
  tx.commit()?
  expect(db.query("SELECT n FROM t")?.len()) == 1
  expect(tx.batch([sql.BatchQuery{query: "SELECT n FROM t"}]).err()) == Some(sql.SqlErr.TxDone)
  let ro = db.begin(read_only: true)?
  expect(ro.batch([sql.BatchQuery{query: "SELECT n FROM t"}])?.len()) == 1
  expect(ro.batch([sql.BatchQuery{query: "INSERT INTO t (n) VALUES (5)"}]).err()) == Some(sql.SqlErr.Server(code: "25006", msg: "cannot execute INSERT in a read-only transaction"))
  ro.rollback()?
  let c = db.conn()?
  expect(affected(c.batch([sql.BatchQuery{query: "DELETE FROM t"}])?)) == [1]
  expect((c.pipeline([[sql.BatchQuery{query: "SELECT n FROM t"}]])?[0].ok() ?? []).len()) == 1
  c.close()
  expect(c.batch([]).err()) == Some(sql.SqlErr.Closed)
  db.close()?
}

test "a driver's batch and pipeline reach DriverConn.pipeline on one connection: in_tx false on the Db and a Conn, true in a transaction" {
  let d = SessDriver{connects: Atomic.new(0), log: Mutex.new([])}
  let db = sql.open_driver(os.net, d, "sess", max_open: 1)?
  let sets = db.batch([sql.BatchQuery{query: "A"}, sql.BatchQuery{query: "B", args: [sql.int(1)]}])?
  expect(affected(sets)) == [1, 1]
  expect(sets[1].rows()[0].int("conn")?) == 1
  expect(db.batch([])?.len()) == 0
  expect(sess_log(d.log)) == "1 PIPELINE 1 in_tx=false|1 A|1 B"
  let outs = db.pipeline([[sql.BatchQuery{query: "C"}], [sql.BatchQuery{query: "FAIL x"}, sql.BatchQuery{query: "D"}], [sql.BatchQuery{query: "E"}]])?
  expect(outs.len()) == 3
  expect(outs[1].err()) == Some(sql.SqlErr.Server(code: "42000", msg: "FAIL x"))
  expect((outs[2].ok() ?? []).len()) == 1
  expect(sess_log(d.log)) == "1 PIPELINE 3 in_tx=false|1 C|1 FAIL x|1 E"
  expect(db.batch([sql.BatchQuery{query: "FAIL y"}]).err()) == Some(sql.SqlErr.Server(code: "42000", msg: "FAIL y"))
  let tx = db.begin()?
  tx.batch([sql.BatchQuery{query: "F"}])?
  tx.commit()?
  let c = db.conn()?
  c.batch([sql.BatchQuery{query: "G"}])?
  let ctx = c.begin()?
  ctx.batch([sql.BatchQuery{query: "H"}])?
  ctx.rollback()?
  c.pipeline([[sql.BatchQuery{query: "I"}]])?
  c.close()
  expect(sess_log(d.log)) == "1 PIPELINE 1 in_tx=false|1 FAIL y|1 BEGIN|1 PIPELINE 1 in_tx=true|1 F|1 COMMIT|1 PIPELINE 1 in_tx=false|1 G|1 BEGIN|1 PIPELINE 1 in_tx=true|1 H|1 ROLLBACK|1 PIPELINE 1 in_tx=false|1 I"
  db.close()?
}

test "a pipeline's notices reach on_notice after it, and a driver without batches says so" {
  let d = SessDriver{connects: Atomic.new(0), log: Mutex.new([])}
  let got: Mutex[List[Str]] = Mutex.new([])
  let db = sql.open_driver(os.net, d, "sess", max_open: 1, on_notice: Some(fn(n: sql.Notice) { got.with(fn(var xs: List[Str]) -> () { xs.push(n.message) }) }))?
  db.pipeline([[sql.BatchQuery{query: "RAISE one"}], [sql.BatchQuery{query: "RAISE two"}]])?
  expect(got.with_read(fn(xs: List[Str]) -> List[Str] { xs })) == ["one", "two"]
  db.close()?
  let log: Mutex[List[Str]] = Mutex.new([])
  let fake = sql.open_driver(os.net, FakeDriver{n: Mutex.new(0), log: log}, "fake:mem", max_open: 1)?
  expect(fake.batch([sql.BatchQuery{query: "SELECT n"}]).err()) == Some(sql.SqlErr.Driver(msg: "fake: no batches"))
  // an empty batch sends nothing, so the driver is not asked
  expect(fake.batch([])?.len()) == 0
  fake.close()?
}
`

// sqlTraceTests are Db.stats and open's on_trace (decision 0648 item 16).
const sqlTraceTests = `
fn op_name(o: sql.TraceOp) -> Str {
  match o {
    sql.TraceOp.Connect => "Connect"
    sql.TraceOp.Acquire => "Acquire"
    sql.TraceOp.Release => "Release"
    sql.TraceOp.Query => "Query"
    sql.TraceOp.Exec => "Exec"
    sql.TraceOp.Prepare => "Prepare"
    sql.TraceOp.BatchQuery => "BatchQuery"
    sql.TraceOp.Batch => "Batch"
    sql.TraceOp.CopyFrom => "CopyFrom"
    sql.TraceOp.CopyTo => "CopyTo"
  }
}

fn trace_line(r: sql.TraceRecord) -> Str {
  var s = op_name(r.op)
  if r.query != "" {
    s = "${s} ${r.query}"
  }
  s = "${s} n=${r.rows_affected}"
  if r.err.is_some() {
    s = "${s} err"
  }
  s
}

fn take_trace(got: Mutex[List[Str]]) -> Str {
  got.with(fn(var xs: List[Str]) -> Str {
    let out = xs.join("|")
    xs = []
    out
  })
}

test "Db.stats counts a driver pool's connections, acquires and closes; the in memory Db counts nothing" {
  let d = SessDriver{connects: Atomic.new(0), log: Mutex.new([])}
  let db = sql.open_driver(os.net, d, "sess", max_open: 2, max_idle: 1)?
  let s0 = db.stats()
  expect(s0.max_open) == 2
  expect((s0.open, s0.idle, s0.in_use, s0.connecting)) == (1, 1, 0, 0)
  expect((s0.acquire_count, s0.new_conns, s0.wait_count, s0.canceled_acquire_count)) == (1, 1, 0, 0)
  let c1 = db.conn()?
  let c2 = db.conn()?
  let s1 = db.stats()
  expect((s1.open, s1.idle, s1.in_use, s1.new_conns, s1.acquire_count)) == (2, 0, 2, 2, 3)
  c1.close()
  c2.close()
  let s2 = db.stats()
  expect((s2.open, s2.idle, s2.in_use, s2.max_idle_closed, s2.max_lifetime_closed)) == (1, 1, 0, 1, 0)
  expect(s2.acquire_time >= 0s) == true
  db.query("Q")?
  expect(db.stats().acquire_count) == 4
  db.close()?
  let m = sql.memory("CREATE TABLE t (n INT)")?
  let ms = m.stats()
  expect((ms.max_open, ms.open, ms.acquire_count)) == (0, 0, 0)
  m.close()?
}

test "Db.stats counts a wait for a permit as it starts, its time when it ends, and a wait ended by a cancellation" {
  let d = SessDriver{connects: Atomic.new(0), log: Mutex.new([])}
  let got: Mutex[List[Str]] = Mutex.new([])
  let db = sql.open_driver(os.net, d, "sess", max_open: 1, on_trace: Some(fn(r: sql.TraceRecord) { got.with(fn(var xs: List[Str]) -> () { xs.push(trace_line(r)) }) }))?
  let c = db.conn()?
  let _ = take_trace(got)
  scope s {
    s.spawn_background(fn() { db.query("W").ignore_err("checked through the trace") })
    while db.stats().wait_count < 1 {
      os.clock.sleep(1ms)
    }
    c.close()
  }
  let st = db.stats()
  expect((st.wait_count, st.canceled_acquire_count, st.in_use)) == (1, 0, 0)
  expect(st.wait_time > 0s) == true
  expect(take_trace(got)) == "Release n=0|Acquire n=0|Query W n=1|Release n=0"
  let c2 = db.conn()?
  scope s2 {
    s2.spawn_background(fn() { db.query("X").ignore_err("cancelled while it waits") })
    while db.stats().wait_count < 2 {
      os.clock.sleep(1ms)
    }
    s2.cancel()
  }
  expect(db.stats().canceled_acquire_count) == 1
  c2.close()
  expect(take_trace(got)) == "Acquire n=0|Acquire n=0 err|Release n=0"
  db.close()?
}

test "on_trace gets a record as each pool, query, exec, transaction and batch operation ends" {
  let d = SessDriver{connects: Atomic.new(0), log: Mutex.new([])}
  let got: Mutex[List[Str]] = Mutex.new([])
  let starts: Mutex[List[Int]] = Mutex.new([])
  let h = fn(r: sql.TraceRecord) {
    got.with(fn(var xs: List[Str]) -> () { xs.push(trace_line(r)) })
    starts.with(fn(var xs: List[Int]) -> () { xs.push(r.start_ns) })
  }
  let db = sql.open_driver(os.net, d, "sess", max_open: 1, on_trace: Some(h))?
  expect(take_trace(got)) == "Connect n=0|Acquire n=0|Release n=0"
  db.query("Q", [sql.int(7)])?
  expect(take_trace(got)) == "Acquire n=0|Query Q n=1|Release n=0"
  db.exec("X")?
  expect(take_trace(got)) == "Acquire n=0|Exec X n=0|Release n=0"
  let tx = db.begin()?
  tx.exec("Y")?
  tx.commit()?
  expect(take_trace(got)) == "Acquire n=0|Exec BEGIN n=0|Exec Y n=0|Exec COMMIT n=0|Release n=0"
  db.pipeline([[sql.BatchQuery{query: "A"}, sql.BatchQuery{query: "B"}], [sql.BatchQuery{query: "FAIL z"}]])?
  expect(take_trace(got)) == "Acquire n=0|BatchQuery A n=1|BatchQuery B n=1|Batch n=2 err|Release n=0"
  expect(starts.with_read(fn(xs: List[Int]) -> Bool { xs.all(fn(x: Int) -> Bool { x > 0 }) })) == true
  db.close()?
}
`

// sqlConnTests reach a Conn's cursor, COPY, typed and statement forms on
// the in memory database, and the parameter helpers decision 0681 and
// sql.named added (TestStdAPICovered, 2026-11-16).
const sqlConnTests = `
test "Numeric's JSON number (pgx's MarshalJSON and UnmarshalJSON): numeric_json, numeric_of_json and the derive that calls them" {
  for (text, want) in [("1.50", "1.50"), ("-0.0030", "-0.0030"), ("100", "100"), ("-1234567890123456789012345678901234567890.50", "-1234567890123456789012345678901234567890.50")] {
    let n = sql.parse_numeric(text).or_fault("valid")
    expect(sql.numeric_json(n).text) == want
    expect(json.encode_str(n)) == want
    expect(sql.numeric_of_json(json.Number{text: want})) == Ok(n)
  }
  expect(sql.numeric_of_json(json.Number{text: "1e+21"}).map(fn(x: sql.Numeric) -> Str { x.display() })) == Ok("1000000000000000000000")
  expect(sql.numeric_json(sql.Numeric.NaN).text) == "NaN"
  expect(json.encode_str([sql.Numeric.NaN, sql.Numeric.Infinity, sql.Numeric.NegInfinity])) == "[null,null,null]"
  expect(json.decode_str[sql.Numeric]("2.5e-3").map(fn(x: sql.Numeric) -> Str { x.display() })) == Ok("0.0025")
  expect(json.decode_str[sql.Numeric]("\"NaN\"").is_err()) == true
}

test "a Conn's rows, each, query_as, query_one_as, prepare and query_all on the in memory Db" {
  let db = sql.memory("CREATE TABLE t (id INT, name TEXT)")?
  let c = db.conn()?
  for i in 1..4 {
    c.exec("INSERT INTO t (id, name) VALUES ($1, $2)", [sql.int(i), sql.str("n${i}")])?
  }
  let rs = c.rows("SELECT id, name FROM t ORDER BY id")?
  expect(rs.next()?.map(fn(r) { r.str("name").ok() ?? "" })) == Some("n1")
  rs.close()
  let total = Atomic.new(0)
  c.each("SELECT id FROM t", [], fn(row) {
    total.add(row.int("id")?)
  })?
  expect(total.get()) == 6
  expect(c.query_as[RowItem]("SELECT id, name FROM t ORDER BY id")?.map(fn(r: RowItem) -> Str { r.name })) == ["n1", "n2", "n3"]
  expect(c.query_one_as[RowItem]("SELECT id, name FROM t WHERE id = $1", [sql.int(2)])?.name) == "n2"
  let st = c.prepare("SELECT name FROM t WHERE id = $1")?
  expect(st.query_one([sql.int(3)])?.str("name")?) == "n3"
  let sets = c.query_all("SELECT id FROM t WHERE id < 2; UPDATE t SET name = 'x' WHERE id = 1; SELECT name FROM t WHERE id = 1")?
  expect(sets.len()) == 2
  expect(sets[0].rows()[0].int("id")?) == 1
  expect(sets[1].rows()[0].str("name")?) == "x"
  c.close()
  db.close()?
}

test "a Conn's copy_from loads rows on the in memory Db, and its copy_to is the in memory database's error" {
  let db = sql.memory("CREATE TABLE t (id INT, name TEXT)")?
  let c = db.conn()?
  let at = Atomic.new(0)
  let n = c.copy_from("t", ["id", "name"], fn() {
    let i = at.add(1)
    if i > 2 {
      return Ok(None)
    }
    Ok(Some([sql.int(i), sql.str("c${i}")]))
  })?
  expect(n) == 2
  expect(c.query("SELECT id FROM t")?.len()) == 2
  let out = c.copy_to("COPY t TO STDOUT", fn(b: Bytes) { Ok(()) })
  expect(match out {
    Ok(_) => "ok"
    Err(e) => e.display()
  }) == "the in memory database has no COPY TO"
  c.close()
}

test "multirange parameters for dates, times and numerics in Postgres's text form" {
  let d1 = time.Date{year: 2026, month: 1, day: 2}
  let d2 = time.Date{year: 2026, month: 1, day: 5}
  expect(sql.multirange_date([sql.Range.Span(lower: sql.Edge.Inclusive(v: d1), upper: sql.Edge.Exclusive(v: d2)), sql.Range.Empty])) == sql.SqlValue.Text(s: "{[2026-01-02,2026-01-05),empty}")
  expect(sql.multirange_date([])) == sql.SqlValue.Text(s: "{}")
  expect(sql.multirange_time([sql.Range.Span(lower: sql.Edge.Inclusive(v: Time.from_unix(0)), upper: sql.Edge.Unbounded)])) == sql.SqlValue.Text(s: "{[\"1970-01-01T00:00:00Z\",)}")
  let lo = sql.parse_numeric("1.50")?
  let hi = sql.parse_numeric("-2")?
  expect(sql.multirange_num([sql.Range.Span(lower: sql.Edge.Exclusive(v: lo), upper: sql.Edge.Inclusive(v: sql.parse_numeric("2.250")?)), sql.Range.Span(lower: sql.Edge.Unbounded, upper: sql.Edge.Exclusive(v: hi))])) == sql.SqlValue.Text(s: "{(1.50,2.250],(,-2)}")
}

test "a Numeric's to_rat is its exact value: the scale does not change it; None for NaN and the infinities" {
  expect(sql.parse_numeric("1.50")?.to_rat()) == Some(big.rat(big.from_int(3), big.from_int(2)))
  expect(sql.parse_numeric("1.50")?.to_rat()) == sql.parse_numeric("1.5")?.to_rat()
  expect(sql.parse_numeric("-2e3")?.to_rat()) == Some(big.rat_from_int(-2000))
  expect(sql.Numeric.NaN.to_rat()) == None
  expect(sql.Numeric.Infinity.to_rat()) == None
  expect(sql.Numeric.NegInfinity.to_rat()) == None
}

test "sql.named rewrites @name to $n in first use order, repeats reuse a number, quoted text is left alone, and strict reports a missing name" {
  let (q, a) = sql.named("INSERT INTO t (id, name, alias) VALUES (@id, @name, @name)", {"name": sql.str("ann"), "id": sql.int(3)})?
  expect(q) == "INSERT INTO t (id, name, alias) VALUES ($1, $2, $2)"
  expect(a) == [sql.int(3), sql.str("ann")]
  let (q2, a2) = sql.named("SELECT name FROM t WHERE id = @id AND name <> '@id'", {"id": sql.int(3)}, strict: true)?
  expect(q2) == "SELECT name FROM t WHERE id = $1 AND name <> '@id'"
  expect(a2) == [sql.int(3)]
  expect(match sql.named("@missing", {}, strict: true) {
    Ok(_) => "ok"
    Err(e) => e.display()
  }) == "named argument @missing has no value"
  let db = sql.memory("CREATE TABLE t (id INT, name TEXT)")?
  let (iq, ia) = sql.named("INSERT INTO t (id, name) VALUES (@id, @name)", {"name": sql.str("bo"), "id": sql.int(7)})?
  db.exec(iq, ia)?
  expect(db.query_one(q2, [sql.int(7)])?.str("name")?) == "bo"
}
`
