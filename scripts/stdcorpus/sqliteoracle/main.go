// Command sqliteoracle prints the rows of std/sqlite's differential corpus:
// github.com/mattn/go-sqlite3 v1.14.53 (std/sqlite's reference, decision 0534;
// the same SQLite 3.54.1 amalgamation) running statements on an in-memory
// database through database/sql, each result shown as std/sqlite's corpus
// shows its own (rows with their storage classes, exec counts, and the error
// as std/sql's portable variants, from SQLite's extended code and message).
// scripts/stdcorpus runs it; it is its own module so the repository's go.mod
// has no third-party requirement.
package main

import (
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	sqlite3 "github.com/mattn/go-sqlite3"
)

// text is std's Str of SQLite text: invalid UTF-8 becomes U+FFFD (spec
// std 66), as strings.ToValidUTF8 replaces each bad run... per byte, as
// Xo's lossy conversion does.
func text(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	var b strings.Builder
	for len(s) > 0 {
		r, n := utf8.DecodeRuneInString(s)
		if r == utf8.RuneError && n == 1 {
			b.WriteRune(utf8.RuneError)
		} else {
			b.WriteString(s[:n])
		}
		s = s[n:]
	}
	return b.String()
}

// show is a column value as both sides print it: I:int, R:the float's
// bits as an Int, T:quoted text, B:hex bytes, N.
func show(v any) string {
	switch t := v.(type) {
	case nil:
		return "N"
	case int64:
		return "I:" + strconv.FormatInt(t, 10)
	case float64:
		return "R:" + strconv.FormatInt(int64(math.Float64bits(t)), 10)
	case string:
		return "T:" + strconv.Quote(text(t))
	case []byte:
		return "B:" + hex.EncodeToString(t)
	}
	return fmt.Sprintf("?%T", v)
}

// codeName is std/native/sqlite.xo's code_name.
func codeName(c int) string {
	names := map[int]string{1: "SQLITE_ERROR", 5: "SQLITE_BUSY", 6: "SQLITE_LOCKED", 7: "SQLITE_NOMEM", 8: "SQLITE_READONLY",
		9: "SQLITE_INTERRUPT", 10: "SQLITE_IOERR", 11: "SQLITE_CORRUPT", 13: "SQLITE_FULL", 14: "SQLITE_CANTOPEN",
		17: "SQLITE_SCHEMA", 18: "SQLITE_TOOBIG", 19: "SQLITE_CONSTRAINT", 20: "SQLITE_MISMATCH", 21: "SQLITE_MISUSE",
		23: "SQLITE_AUTH", 25: "SQLITE_RANGE", 26: "SQLITE_NOTADB", 517: "SQLITE_BUSY_SNAPSHOT", 261: "SQLITE_BUSY_RECOVERY"}
	if n, ok := names[c]; ok {
		return n
	}
	return "SQLITE_" + strconv.Itoa(c)
}

// errText is an error as std/sql's variant: spec std 66's mapping of
// SQLite's extended code and message.
func errText(err error) string {
	var e sqlite3.Error
	if !errors.As(err, &e) {
		return "err other " + err.Error()
	}
	msg := e.Error()
	switch int(e.ExtendedCode) {
	case 2067, 1555: // CONSTRAINT_UNIQUE, CONSTRAINT_PRIMARYKEY
		return "err constraint 23505 " + msg
	case 787:
		return "err constraint 23503 " + msg
	case 1299:
		return "err constraint 23502 " + msg
	case 275:
		return "err constraint 23514 " + msg
	}
	if strings.HasPrefix(msg, "no such table: ") {
		return "err notable " + strings.TrimPrefix(msg, "no such table: ")
	}
	if strings.HasSuffix(msg, ": syntax error") {
		return "err syntax " + strings.TrimSuffix(msg, ": syntax error")
	}
	if strings.HasPrefix(msg, "incomplete input") {
		return "err syntax " + msg
	}
	return "err server " + codeName(int(e.ExtendedCode)) + " " + msg
}

// arg is a case's argument as the corpus spells it and as Go binds it.
type arg struct {
	spell string
	v     any
}

type kase struct {
	setup []string
	query bool
	stmt  string
	args  []arg
}

func run(db *sql.DB, k kase) string {
	for _, s := range k.setup {
		if _, err := db.Exec(s); err != nil {
			return "setup " + errText(err)
		}
	}
	vs := make([]any, len(k.args))
	for i, a := range k.args {
		vs[i] = a.v
	}
	if !k.query {
		r, err := db.Exec(k.stmt, vs...)
		if err != nil {
			return errText(err)
		}
		n, _ := r.RowsAffected()
		id, _ := r.LastInsertId()
		return fmt.Sprintf("exec %d %d", n, id)
	}
	rows, err := db.Query(k.stmt, vs...)
	if err != nil {
		return errText(err)
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	var out []string
	for rows.Next() {
		cells := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range cells {
			ptrs[i] = &cells[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return "scan " + err.Error()
		}
		var cs []string
		for _, c := range cells {
			cs = append(cs, show(c))
		}
		out = append(out, "("+strings.Join(cs, ",")+")")
	}
	if err := rows.Err(); err != nil {
		return errText(err)
	}
	return "rows " + strings.Join(out, " ")
}

func q(s string) string { return strconv.Quote(s) }

// xoArgs spells the arguments for the corpus: i:N, r:BITS (the float's bits
// as an Int), t:TEXT, b:HEX, n.
func xoArgs(as []arg) string {
	var ss []string
	for _, a := range as {
		ss = append(ss, q(a.spell))
	}
	return "[" + strings.Join(ss, ", ") + "]"
}

func xoList(ss []string) string {
	var out []string
	for _, s := range ss {
		out = append(out, q(s))
	}
	return "[" + strings.Join(out, ", ") + "]"
}

// lit is an argument as an SQL literal (the random cases' setup rows).
func lit(a arg) string {
	switch t := a.v.(type) {
	case nil:
		return "NULL"
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'e', -1, 64)
	case string:
		return "'" + strings.ReplaceAll(t, "'", "''") + "'"
	case []byte:
		return "x'" + hex.EncodeToString(t) + "'"
	}
	panic(a.spell)
}

func ai(n int64) arg   { return arg{"i:" + strconv.FormatInt(n, 10), n} }
func ar(f float64) arg { return arg{"r:" + strconv.FormatInt(int64(math.Float64bits(f)), 10), f} }
func at(s string) arg  { return arg{"t:" + s, s} }
func ab(b []byte) arg  { return arg{"b:" + hex.EncodeToString(b), b} }
func an() arg          { return arg{"n", nil} }

const table = "CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE, score REAL CHECK (score >= 0), data BLOB, n INTEGER, p INTEGER REFERENCES t(id))"

// curated: the spec's mappings, one case each.
func curated() []kase {
	ins := "INSERT INTO t (name, score, data, n) VALUES ('a', 1.5, x'00ff', 7)"
	return []kase{
		{nil, true, "SELECT 1, 2.5, 'x', x'0102', NULL", nil},
		{nil, true, "SELECT typeof(1), typeof(1.0), typeof('a'), typeof(x'00'), typeof(NULL)", nil},
		{nil, true, "SELECT 9223372036854775807 + 1, -9223372036854775808 - 1, 1 / 0, 1 % 0, 5 / 2, 5.0 / 2", nil},
		{nil, true, "SELECT CAST('12abc' AS INTEGER), CAST('1e3' AS REAL), CAST(x'41' AS TEXT), CAST(12 AS BLOB)", nil},
		{nil, true, "SELECT CAST(x'ff41' AS TEXT), length(CAST(x'ff41' AS TEXT)), hex(CAST(x'c3a9' AS TEXT))", nil},
		{nil, true, "SELECT $1, $2, $3, $4, $5", []arg{ai(-3), ar(0.1), at("héllo"), ab([]byte{0, 1, 255}), an()}},
		{nil, true, "SELECT ?1 + ?1, ?2", []arg{ai(21), at("z")}},
		{nil, true, "SELECT ?, ?", []arg{ai(1), ai(2)}},
		{nil, true, "SELECT typeof($1), typeof($2), typeof($3), typeof($4)", []arg{ai(1), ar(1), at("1"), ab([]byte("1"))}},
		{[]string{table}, false, ins, nil},
		{[]string{table, ins}, false, ins, nil},
		{[]string{table}, false, "INSERT INTO t (name) VALUES (NULL)", nil},
		{[]string{table}, false, "INSERT INTO t (name, score) VALUES ('a', -1)", nil},
		{[]string{table, "PRAGMA foreign_keys = ON"}, false, "INSERT INTO t (name, p) VALUES ('a', 99)", nil},
		{[]string{table, ins}, false, "INSERT INTO t (id, name) VALUES (1, 'b')", nil},
		{nil, true, "SELECT * FROM missing", nil},
		{nil, false, "INSERT INTO missing VALUES (1)", nil},
		{nil, true, "SELEC 1", nil},
		{nil, true, "SELECT 1 FROM", nil},
		{nil, true, "SELECT (1", nil},
		{nil, true, "SELECT nosuchfn(1)", nil},
		{nil, true, "SELECT nocol FROM (SELECT 1 AS a)", nil},
		{[]string{table}, true, "SELECT id FROM t", nil},
		{[]string{table, ins, "INSERT INTO t (name) VALUES ('b')"}, false, "UPDATE t SET n = coalesce(n, 0) + 1", nil},
		{[]string{table, ins}, false, "DELETE FROM t WHERE name = $1", []arg{at("a")}},
		{[]string{table, ins}, true, "SELECT id, name, score, data, n, p FROM t", nil},
		{nil, true, "SELECT abs(-9223372036854775808)", nil},
		{nil, true, "SELECT zeroblob(3), zeroblob(0), quote(x'ab'), quote('it''s')", nil},
		{nil, true, "WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x + 1 FROM c WHERE x < 5) SELECT x, x * x FROM c", nil},
		{nil, true, "SELECT json_extract('{\"a\":[1,2.5,\"s\",null]}', '$.a[1]'), json_array(1, 'x', NULL)", nil},
	}
}

// random: seeded tables of mixed values and queries over them.
func random(r *rand.Rand, n int) []kase {
	vals := func() arg {
		switch r.Intn(9) {
		case 0:
			return an()
		case 1:
			return ai([]int64{0, 1, -1, math.MaxInt64, math.MinInt64, 1 << 53, -(1 << 31)}[r.Intn(7)])
		case 2:
			return ai(r.Int63n(2000) - 1000)
		case 3:
			return ar([]float64{0, -0.0, 0.5, -2.25, 1e300, -1e-300, math.MaxFloat64, 3.141592653589793}[r.Intn(8)])
		case 4:
			return ar(r.NormFloat64() * 1000)
		case 5:
			return at([]string{"", "a", "héllo", "日本", "x'y", "1", "2.5", " 3 ", "\t\n", "NULL"}[r.Intn(10)])
		case 6:
			b := make([]byte, r.Intn(6))
			r.Read(b)
			return ab(b)
		case 7:
			return at(strconv.Itoa(r.Intn(100)))
		default:
			return ai(int64(r.Intn(10)))
		}
	}
	types := []string{"INTEGER", "REAL", "TEXT", "BLOB", "NUMERIC", "", "INT", "VARCHAR(5)", "DOUBLE"}
	exprs := []string{"a", "b", "a + b", "a * 2", "a || b", "typeof(a)", "typeof(b)", "length(a)", "a = b", "a < b",
		"coalesce(a, b)", "abs(a)", "round(a, 1)", "upper(a)", "hex(a)", "CAST(a AS INTEGER)", "CAST(b AS REAL)",
		"CAST(a AS TEXT)", "a IS NULL", "max(a, b)", "min(a, b)", "substr(a, 2)", "a / b", "a % b", "-a", "nullif(a, b)"}
	aggs := []string{"count(*)", "count(a)", "sum(a)", "total(a)", "avg(b)", "max(a)", "min(b)", "group_concat(a)"}
	var ks []kase
	for i := 0; i < n; i++ {
		ta, tb := types[r.Intn(len(types))], types[r.Intn(len(types))]
		setup := []string{fmt.Sprintf("CREATE TABLE u (k INTEGER PRIMARY KEY, a %s, b %s)", ta, tb)}
		k := kase{setup: setup, query: true}
		rows := 1 + r.Intn(4)
		var vals2 []arg
		var ph []string
		for j := 0; j < rows; j++ {
			ph = append(ph, fmt.Sprintf("($%d, $%d)", 2*j+1, 2*j+2))
			vals2 = append(vals2, vals(), vals())
		}
		ins := kase{setup: setup, query: false, stmt: "INSERT INTO u (a, b) VALUES " + strings.Join(ph, ", "), args: vals2}
		switch r.Intn(4) {
		case 0:
			ks = append(ks, ins)
			continue
		case 1:
			k.stmt = "SELECT " + aggs[r.Intn(len(aggs))] + ", " + aggs[r.Intn(len(aggs))] + " FROM u"
		default:
			k.stmt = "SELECT " + exprs[r.Intn(len(exprs))] + ", " + exprs[r.Intn(len(exprs))] + " FROM u ORDER BY k"
		}
		var lits []string
		for j := 0; j < rows; j++ {
			lits = append(lits, "("+lit(vals2[2*j])+", "+lit(vals2[2*j+1])+")")
		}
		k.setup = append(append([]string{}, setup...), "INSERT INTO u (a, b) VALUES "+strings.Join(lits, ", "))
		ks = append(ks, k)
	}
	return ks
}

// fuzzSeed is XO_SQLITEFUZZ_SEED (default 0, the committed corpus):
// internal/stdtest/corpusfuzz_test.go's fresh seeds (task #90). A seed
// other than 0 writes only random cases, XO_SQLITEFUZZ_N of them
// (default 2000; scripts/stdproven sets it to its -batch).
func fuzzSeed() int64 {
	seed, _ := strconv.ParseInt(os.Getenv("XO_SQLITEFUZZ_SEED"), 10, 64)
	return seed
}

func fuzzN() int {
	if n, err := strconv.Atoi(os.Getenv("XO_SQLITEFUZZ_N")); err == nil && n > 0 {
		return n
	}
	return 2000
}

func main() {
	r := rand.New(rand.NewSource(29))
	ks := append(curated(), random(r, 150)...)
	if seed := fuzzSeed(); seed != 0 {
		ks = random(rand.New(rand.NewSource(29+seed)), fuzzN())
	}
	fmt.Println("// Generated by scripts/stdcorpus from github.com/mattn/go-sqlite3 v1.14.53 (MIT, Copyright (c) 2014 Yasuhiro Matsumoto) running each case on an in-memory database through database/sql, against std/sqlite (go1.26.1).")
	fmt.Println("// Do not edit: regenerate with go run ./scripts/stdcorpus.")
	fmt.Println()
	fmt.Println("use std/sql")
	fmt.Println("use std/sqlite")
	fmt.Println()
	fmt.Println(helpers)
	fmt.Println("/// (setup statements, query or exec, the statement, its arguments, mattn/go-sqlite3's result)")
	fmt.Println("const CASES: List[(List[Str], Bool, Str, List[Str], Str)] = [")
	for _, k := range ks {
		db, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		db.SetMaxOpenConns(1)
		got := run(db, k)
		db.Close()
		fmt.Printf("  (%s, %v, %s, %s, %s),\n", xoList(k.setup), k.query, q(k.stmt), xoArgs(k.args), q(text(got)))
	}
	fmt.Println("]")
	fmt.Println()
	fmt.Println(tests)
}

const helpers = `fn arg(a: Str) -> sql.SqlValue {
  if a == "n" {
    return sql.null()
  }
  let v = a[2..a.len()]
  match a[0..2] {
    "i:" => sql.int(Int.parse(v).or_fault(a))
    "r:" => sql.float(Float.from_bits(Int.parse(v).or_fault(a)))
    "t:" => sql.str(v)
    _ => sql.bytes(Bytes.from_hex(v).or_fault(a))
  }
}

/// A column value as the oracle shows it.
fn show(v: sql.SqlValue) -> Str {
  match v {
    Null => "N"
    Integer(n) => "I:${n}"
    Real(f) => "R:${f.to_bits()}"
    Text(s) => "T:${s.quote()}"
    Blob(b) => "B:${b.to_hex()}"
    _ => "?"
  }
}

/// An error as the oracle shows mattn's, from std/sql's variant.
fn err_text(e: sql.SqlErr) -> Str {
  match e {
    Constraint(msg, code, _) => "err constraint ${code} ${msg}"
    NoTable(name) => "err notable ${name}"
    Syntax(msg) => "err syntax ${msg}"
    Server(code, msg) => "err server ${code} ${msg}"
    _ => "err other ${e.display()}"
  }
}

fn run(db: sql.Db, query: Bool, stmt: Str, args: List[sql.SqlValue]) -> Str ! sql.SqlErr uses net.dial {
  if !query {
    let r = db.exec(stmt, args)?
    return "exec ${r.rows_affected} ${r.last_insert_id}"
  }
  let rows = db.query(stmt, args)?
  "rows " + rows.map(fn(r) { "(" + r.values().map(show).join(",") + ")" }).join(" ")
}
`

const tests = `test "statements give mattn/go-sqlite3's rows, counts and errors" {
  var i = 0
  for (setup, query, stmt, args, want) in CASES {
    let db = sql.open_driver(os.net, sqlite.driver(os.fs), ":memory:", max_open: 1).or_fault("open")
    var got = ""
    for s in setup {
      if got == "" {
        match db.exec(s, []) {
          Err(e) => got = "setup " + err_text(e)
          _ => {}
        }
      }
    }
    if got == "" {
      got = match run(db, query, stmt, args.map(arg)) {
        Ok(s) => s
        Err(e) => err_text(e)
      }
    }
    db.close().or_fault("close")
    expect("${i} ${got}") == "${i} ${want}"
    i += 1
  }
}`
