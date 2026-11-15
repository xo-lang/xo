// Reference for the MySQL differential test (scripts/mysql-diff.sh,
// decision 0648 item 12): each statement through go-sql-driver/mysql's
// public API (database/sql, the text protocol: interpolateParams=true),
// its values printed in the Debug form of std/sql's Row with decision
// 0648 item 8's mapping, its column types as go-sql-driver reports them,
// or the error's number, SQLSTATE and message with item 9's mapping. This
// module is the only place go-sql-driver appears; std never depends on it.
package main

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-sql-driver/mysql"
)

// q is Xo's Debug text of a Str (std/rt show.go quote).
func q(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		case 0:
			b.WriteString(`\0`)
		case utf8.RuneError:
			b.WriteString(`\u{FFFD}`)
		default:
			if r < 0x20 || r == 0x7f {
				b.WriteString(`\x` + strconv.FormatUint(uint64(r)>>4, 16) + strconv.FormatUint(uint64(r)&15, 16))
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

func xoFloat(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "+Inf"
	case math.IsInf(f, -1):
		return "-Inf"
	}
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eEn") {
		s += ".0"
	}
	return s
}

func blob(b []byte) string { return "Blob(b: b" + q(string(b)) + ")" }

// Time's range in Xo: int64 nanoseconds since 1970.
var minTime, maxTime = time.Unix(0, math.MinInt64), time.Unix(0, math.MaxInt64)

// mysqlText is the server's text of a DATE, DATETIME or TIMESTAMP that
// go-sql-driver parsed (parseTime=true): Go's zero time is MySQL's zero
// date; dec is the column's fractional digits.
func mysqlText(t string, x time.Time, dec int64) string {
	if t == "DATE" {
		if x.IsZero() {
			return "0000-00-00"
		}
		return x.Format("2006-01-02")
	}
	layout, zero := "2006-01-02 15:04:05", "0000-00-00 00:00:00"
	if dec > 0 && dec <= 6 {
		layout += "." + strings.Repeat("0", int(dec))
		zero += "." + strings.Repeat("0", int(dec))
	}
	if x.IsZero() {
		return zero
	}
	return x.Format(layout)
}

// val is decision 0648 item 8's value of what go-sql-driver scanned
// (int64, uint64, float32, float64, []byte or time.Time) for column c.
func val(c *sql.ColumnType, v any) string {
	t := c.DatabaseTypeName()
	switch x := v.(type) {
	case nil:
		return "Null"
	case int64:
		return fmt.Sprintf("Integer(n: %d)", x)
	case uint64:
		if x > math.MaxInt64 {
			return "Text(s: " + q(strconv.FormatUint(x, 10)) + ")"
		}
		return fmt.Sprintf("Integer(n: %d)", x)
	case float32:
		// the server's shortest text of a FLOAT, read as a Float
		f, _ := strconv.ParseFloat(strconv.FormatFloat(float64(x), 'g', -1, 32), 64)
		return "Real(f: " + xoFloat(f) + ")"
	case float64:
		return "Real(f: " + xoFloat(x) + ")"
	case time.Time:
		// outside Time's range (a zero date, a year before 1678): the
		// server's text, as the Postgres driver keeps infinity
		if x.Before(minTime) || x.After(maxTime) {
			dec, _, _ := c.DecimalSize()
			return "Text(s: " + q(mysqlText(t, x, dec)) + ")"
		}
		return "At(t: " + x.UTC().Format(time.RFC3339Nano) + ")"
	case []byte:
		switch t {
		case "BIT", "GEOMETRY", "BINARY", "VARBINARY", "TINYBLOB", "BLOB", "MEDIUMBLOB", "LONGBLOB":
			return blob(x)
		case "JSON":
			return "JsonText(s: " + q(string(x)) + ")"
		}
		return "Text(s: " + q(string(x)) + ")"
	}
	return "Text(s: " + q(fmt.Sprint(v)) + ")"
}

// args reads a case line's arguments as scripts/mysqldiff/xo does.
func args(s string) []any {
	var out []any
	for _, a := range strings.Split(s, " | ") {
		kind, v, _ := strings.Cut(a, ":")
		switch kind {
		case "T":
			out = append(out, v)
		case "I":
			n, _ := strconv.ParseInt(v, 10, 64)
			out = append(out, n)
		case "F":
			f, _ := strconv.ParseFloat(v, 64)
			out = append(out, f)
		case "B":
			out = append(out, v == "t")
		case "X":
			b, _ := hex.DecodeString(v)
			if b == nil {
				b = []byte{}
			}
			out = append(out, b)
		case "D":
			t, _ := time.Parse(time.RFC3339Nano, v)
			out = append(out, t)
		default:
			out = append(out, nil)
		}
	}
	return out
}

func quotedAfter(msg, prefix string) string {
	i := strings.Index(msg, prefix+"'")
	if i < 0 {
		return ""
	}
	name, _, _ := strings.Cut(msg[i+len(prefix)+1:], "'")
	return name
}

// errLine is decision 0648 item 9's SqlErr of a server error, as the Xo
// side prints it.
func errLine(err error) string {
	var me *mysql.MySQLError
	if !errors.As(err, &me) {
		return "error conn"
	}
	state := string(me.SQLState[:])
	msg := fmt.Sprintf("Error %d (%s): %s", me.Number, state, me.Message)
	switch me.Number {
	case 1062, 1586, 1451, 1452, 1216, 1217, 1048, 1364, 3819:
		return "error Constraint " + state + " " + msg
	case 1064, 1149:
		return "error Syntax " + msg
	case 1146:
		return "error NoTable " + quotedAfter(me.Message, "Table ")
	case 1317, 3024:
		return "error Canceled"
	}
	return "error Server " + state + " " + msg
}

// colTypes is each column's name, database type, DECIMAL precision and
// scale, and nullability.
func colTypes(cts []*sql.ColumnType) string {
	var out []string
	for _, c := range cts {
		ps := "-"
		if c.DatabaseTypeName() == "DECIMAL" {
			if p, s, ok := c.DecimalSize(); ok {
				ps = fmt.Sprintf("%d,%d", p, s)
			}
		}
		null := "-"
		if n, ok := c.Nullable(); ok {
			null = strconv.FormatBool(n)
		}
		out = append(out, fmt.Sprintf("%s:%s:%s:%s", c.Name(), c.DatabaseTypeName(), ps, null))
	}
	return strings.Join(out, " ")
}

// dsnFor is the DSN go-sql-driver gets for the Xo driver's dsn: the text
// protocol, and with MY_ROOTS, tls=true and tls=custom verified against
// those roots (the Xo driver's roots: on sql.open).
func dsnFor(dsn string) string {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	dsn += sep + "interpolateParams=true"
	if path := os.Getenv("MY_ROOTS"); path != "" {
		pem, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		pool := x509.NewCertPool()
		pool.AppendCertsFromPEM(pem)
		mysql.RegisterTLSConfig("xo", &tls.Config{RootCAs: pool})
		dsn = strings.Replace(strings.Replace(dsn, "tls=true", "tls=xo", 1), "tls=custom", "tls=xo", 1)
	}
	return dsn
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: gomysql [probe] DSN < cases")
		os.Exit(2)
	}
	if os.Args[1] == "probe" {
		db, err := sql.Open("mysql", dsnFor(os.Args[2]))
		if err == nil {
			err = db.Ping()
		}
		if err != nil {
			fmt.Println("fail")
			return
		}
		fmt.Println("ok")
		return
	}
	db, err := sql.Open("mysql", dsnFor(os.Args[1]))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		fmt.Println("== " + line)
		stmt, rest, has := strings.Cut(line, " ;; ")
		var xs []any
		if has {
			xs = args(rest)
		}
		if strings.HasPrefix(line, "SELECT") || strings.HasPrefix(line, "SHOW") {
			out, err := selectLines(db, stmt, xs)
			if err != nil {
				fmt.Println(errLine(err))
				continue
			}
			for _, l := range out {
				fmt.Println(l)
			}
			continue
		}
		r, err := db.Exec(stmt, xs...)
		if err != nil {
			fmt.Println(errLine(err))
			continue
		}
		n, _ := r.RowsAffected()
		id, _ := r.LastInsertId()
		fmt.Printf("affected %d id %d\n", n, id)
	}
}

// selectLines is a SELECT's column types and then each row; the whole
// result first, so an error part way prints the error alone.
func selectLines(db *sql.DB, stmt string, xs []any) ([]string, error) {
	rows, err := db.Query(stmt, xs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cts, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	out := []string{"types " + colTypes(cts)}
	var cols []string
	for _, c := range cts {
		cols = append(cols, q(c.Name()))
	}
	for rows.Next() {
		vs := make([]any, len(cts))
		ps := make([]any, len(cts))
		for i := range vs {
			ps[i] = &vs[i]
		}
		if err := rows.Scan(ps...); err != nil {
			return nil, err
		}
		var vals []string
		for i, v := range vs {
			vals = append(vals, val(cts[i], v))
		}
		out = append(out, fmt.Sprintf("Row{cols: [%s], vals: [%s]}", strings.Join(cols, ", "), strings.Join(vals, ", ")))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// the sets after the first (multiStatements): read and dropped, an
	// error among them the query's (decision 0648 item 7; go-sql-driver
	// reports it only to a caller of NextResultSet)
	for rows.NextResultSet() {
		for rows.Next() {
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
