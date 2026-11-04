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
	o.WriteString("use std/sql\n")
	o.WriteString(sqlTests)
	o.WriteString(sqlDriverTests)
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

fn (c: FakeConn) is_broken() -> Bool {
  record(c.log, "is_broken")
  c.broken.get()
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

fn (c: CurConn) is_broken() -> Bool { false }

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

fn (c: ModeConn) is_broken() -> Bool { false }

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
