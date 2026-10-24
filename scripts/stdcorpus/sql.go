package main

import "strings"

// std/sql's Row.bytes_opt, which no corpus reached: std.md 10.4 says the
// _opt accessors map NULL to None, while a non optional accessor reads a
// NULL as Mismatch, and a name the row does not have is NoColumn. The in
// memory driver is the one the corpus talks to (the same on every backend),
// so the rows need no Go twin: the rule is the spec's and every assertion
// names it.
func sqlCorpus() string {
	var o strings.Builder
	o.WriteString(header("std/sql's Row.bytes_opt on the in memory driver: Some for a blob (empty included), None for NULL, and the Mismatch and NoColumn errors the non optional forms give (std.md 10.4)"))
	o.WriteString("use std/sql\n")
	o.WriteString(sqlTests)
	return o.String()
}

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
`
