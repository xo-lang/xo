// Reference for the Postgres differential test (scripts/pg-diff.sh): each statement through
// pgx's pgconn (simple protocol), its last result printed with spec/std.md
// 10.9's mapping of text values by type OID, in the Debug form of the
// native std/sql Row, or the error's SQLSTATE and message.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/netip"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// q is Xo's Debug text of a Str or Bytes (std/rt show.go quote): U+FFFD
// for each byte that is not UTF-8.
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

func elemOf(oid uint32) int64 {
	m := map[uint32]int64{1000: 16, 1001: 17, 1003: 19, 1005: 21, 1007: 23, 1016: 20, 1009: 25, 1014: 1042, 1015: 1043, 1021: 700, 1022: 701, 1028: 26, 199: 114, 3807: 3802, 1115: 1114, 1185: 1184}
	if e, ok := m[oid]; ok {
		return e
	}
	return -1
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

func iso(s string, utc bool) (time.Time, bool) {
	t := strings.Replace(s, " ", "T", 1)
	if utc {
		t += "Z"
	} else if i := strings.LastIndexAny(t, "+-"); i > 10 {
		if len(t)-i == 3 {
			t += ":00"
		}
	}
	x, err := time.Parse(time.RFC3339Nano, t)
	return x, err == nil
}

func val(oid uint32, s string) string {
	switch oid {
	case 16:
		return fmt.Sprintf("Flag(b: %v)", s == "t")
	case 20, 21, 23, 26, 28, 29, 5069:
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return fmt.Sprintf("Integer(n: %d)", n)
		}
	case 700, 701:
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return "Real(f: " + xoFloat(f) + ")"
		}
	case 17:
		if strings.HasPrefix(s, `\x`) {
			if b, err := hex.DecodeString(s[2:]); err == nil {
				return "Blob(b: b" + q(string(b))[0:] + ")"
			}
		}
	case 114, 3802:
		return "JsonText(s: " + q(s) + ")"
	case 1184, 1114:
		if t, ok := iso(s, oid == 1114); ok {
			return "At(t: " + t.UTC().Format(time.RFC3339Nano) + ")"
		}
	}
	if e := elemOf(oid); e >= 0 {
		if items, ok := parseArray(s, uint32(e)); ok {
			return "Array(items: [" + strings.Join(items, ", ") + "])"
		}
	}
	return "Text(s: " + q(s) + ")"
}

func parseArray(s string, elem uint32) ([]string, bool) {
	if len(s) < 2 || s[0] != '{' || s[len(s)-1] != '}' {
		return nil, false
	}
	out := []string{}
	if len(s) == 2 {
		return out, true
	}
	i := 1
	for i < len(s)-1 {
		if s[i] == '{' {
			return nil, false
		}
		var cur strings.Builder
		quoted := false
		if s[i] == '"' {
			quoted = true
			i++
			for i < len(s)-1 && s[i] != '"' {
				if s[i] == '\\' && i+1 < len(s) {
					i++
				}
				cur.WriteByte(s[i])
				i++
			}
			i++
		} else {
			for i < len(s)-1 && s[i] != ',' {
				cur.WriteByte(s[i])
				i++
			}
		}
		t := cur.String()
		if !quoted && t == "NULL" {
			out = append(out, "Null")
		} else {
			out = append(out, val(elem, t))
		}
		if i < len(s)-1 && s[i] == ',' {
			i++
		}
	}
	return out, true
}

// params is the text form of each argument of a case line (after " ;; ",
// separated by " | "): T:text, I:int, F:float, B:t or f, N (NULL),
// X:hex (bytea), J:json, D:RFC 3339 time, A:ints and S:strs (comma
// separated, N for NULL), as std/sql's driver sends them.
func params(s string) [][]byte {
	var out [][]byte
	for _, a := range strings.Split(s, " | ") {
		kind, v, _ := strings.Cut(a, ":")
		switch kind {
		case "N":
			out = append(out, nil)
		case "X":
			out = append(out, []byte(`\x`+v))
		case "A", "S":
			var xs []string
			for _, x := range strings.Split(v, ",") {
				switch {
				case x == "N":
					xs = append(xs, "NULL")
				case kind == "S":
					xs = append(xs, `"`+strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(x)+`"`)
				default:
					xs = append(xs, x)
				}
			}
			out = append(out, []byte("{"+strings.Join(xs, ",")+"}"))
		default:
			out = append(out, []byte(v))
		}
	}
	return out
}

func main() {
	ctx := context.Background()
	if os.Args[1] == "probe" {
		// whether a connection opens and answers a ping: "ok" or "fail"
		c, err := pgconn.Connect(ctx, os.Args[2])
		if err == nil {
			err = c.Ping(ctx)
		}
		if err != nil {
			fmt.Println("fail")
		} else {
			fmt.Println("ok")
		}
		return
	}
	c, err := pgconn.Connect(ctx, os.Args[1])
	if err != nil {
		fmt.Println("connect:", err)
		os.Exit(1)
	}
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "TYPED "); ok {
			fmt.Println("== " + line)
			typed(ctx, c, rest)
			continue
		}
		if rest, ok := strings.CutPrefix(line, "COPYTO "); ok {
			// COPY ... TO STDOUT (decision 0679): pgconn's CopyTo, the
			// data it wrote (before an error too) and its result
			fmt.Println("== " + line)
			var buf bytes.Buffer
			tag, err := c.CopyTo(ctx, &buf, rest)
			fmt.Printf("data %q\n", buf.String())
			var pe *pgconn.PgError
			if errors.As(err, &pe) {
				fmt.Printf("error %s %s\n", pe.Code, pe.Message)
			} else if err != nil {
				fmt.Println("error", err)
			} else {
				fmt.Printf("copied %d\n", tag.RowsAffected())
			}
			continue
		}
		if rest, ok := strings.CutPrefix(line, "LO "); ok {
			fmt.Println("== " + line)
			largeObject(ctx, os.Args[1], rest)
			continue
		}
		if rest, ok := strings.CutPrefix(line, "BATCH "); ok {
			fmt.Println("== " + line)
			batch(ctx, c, rest)
			continue
		}
		if kind, rest, ok := strings.Cut(line, " "); ok && (kind == "STRUCTARGS" || kind == "STRICTSTRUCTARGS") {
			fmt.Println("== " + line)
			structArgs(ctx, os.Args[1], kind == "STRICTSTRUCTARGS", rest)
			continue
		}
		if os.Getenv("PG_FRESH") != "" {
			// a connection of its own for each statement (internal/stdtest
			// TestPgFuzz: a hostile server closes connections at will)
			c.Close(ctx)
			if c, err = pgconn.Connect(ctx, os.Args[1]); err != nil {
				fmt.Println("== " + line)
				fmt.Println("error connect")
				continue
			}
		}
		stmt, args, hasArgs := strings.Cut(line, " ;; ")
		var res []*pgconn.Result
		if hasArgs {
			r := c.ExecParams(ctx, stmt, params(args), nil, nil, nil).Read()
			res, err = []*pgconn.Result{r}, r.Err
		} else {
			res, err = readAll(c.Exec(ctx, line))
		}
		fmt.Println("== " + line)
		var pe *pgconn.PgError
		if errors.As(err, &pe) {
			fmt.Printf("error %s %s\n", pe.Code, pe.Message)
			continue
		} else if err != nil {
			fmt.Println("error", err)
			continue
		}
		r := res[len(res)-1]
		if len(r.FieldDescriptions) == 0 {
			fmt.Printf("affected %d\n", r.CommandTag.RowsAffected())
			continue
		}
		printRows(r)
	}
}

// saRow is a STRUCTARGS line's struct (decision 0682, task #124): the
// Xo side's SaRow with derive SqlRow, field for field.
type saRow struct {
	Name string    `db:"name"`
	N    *int32    `db:"n"`
	F    float64   `db:"f"`
	B    bool      `db:"b"`
	At   time.Time `db:"at"`
	D    []byte    `db:"d"`
	D2   []byte    `db:"d2"`
	Tags []string  `db:"tags"`
	Ns   []int64   `db:"ns"`
	Skip string    `db:"-"`
}

// saRows are the values a STRUCTARGS line names by its number.
func saRows() []saRow {
	seven := int32(7)
	return []saRow{
		{Name: "sa1", N: &seven, F: 1.5, B: true, At: time.Unix(1792458183, 0).UTC(), D: []byte{0, 0xff}, D2: []byte("x"), Tags: []string{"x", "y z"}, Ns: []int64{1, 3}, Skip: "s"},
		{Name: "sa2", N: nil, F: -0.25, B: false, At: time.Unix(0, 0).UTC(), D: []byte{}, D2: nil, Tags: []string{}, Ns: []int64{}},
	}
}

// structArgs runs `N QUERY` of a STRUCTARGS line through pgx's StructArgs
// (StrictStructArgs): the rows affected, the server's error, or "error
// rewrite" for a name the rewrite refuses.
func structArgs(ctx context.Context, dsn string, strict bool, rest string) {
	ns, q, _ := strings.Cut(rest, " ")
	k, _ := strconv.Atoi(ns)
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		fmt.Println("error connect")
		return
	}
	defer conn.Close(ctx)
	var rw pgx.QueryRewriter = pgx.StructArgs(saRows()[k-1])
	if strict {
		rw = pgx.StrictStructArgs(saRows()[k-1])
	}
	tag, err := conn.Exec(ctx, q, rw)
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		fmt.Printf("error %s %s\n", pe.Code, pe.Message)
	} else if err != nil {
		fmt.Println("error rewrite")
	} else {
		fmt.Printf("affected %d\n", tag.RowsAffected())
	}
}

// readAll is MultiResultReader.ReadAll keeping each result's fields when
// it has no rows: ResultReader.Read sets Result.FieldDescriptions only as
// it reads a row, so a SELECT of no rows read as a statement without
// columns (internal/stdtest TestPgFuzz); std/sql gives its column types.
func readAll(mrr *pgconn.MultiResultReader) ([]*pgconn.Result, error) {
	var res []*pgconn.Result
	for mrr.NextResult() {
		rr := mrr.ResultReader()
		fds := append([]pgconn.FieldDescription(nil), rr.FieldDescriptions()...)
		r := rr.Read()
		if r.FieldDescriptions == nil && fds != nil {
			r.FieldDescriptions = fds
		}
		res = append(res, r)
	}
	err := mrr.Close()
	return res, err
}

// batchItems are a BATCH line's queries (decision 0648 item 15): items
// separated by " || ", each a query with " ;; " arguments, "N*" before
// one repeating it N times.
func batchItems(s string) [][2]string {
	var out [][2]string
	for _, it := range strings.Split(s, " || ") {
		n := 1
		if i := strings.Index(it, "*"); i > 0 {
			if k, err := strconv.Atoi(it[:i]); err == nil {
				n, it = k, it[i+1:]
			}
		}
		stmt, args, _ := strings.Cut(it, " ;; ")
		for range n {
			out = append(out, [2]string{stmt, args})
		}
	}
	return out
}

// batch runs a BATCH line through pgconn's ExecBatch (one Sync, the
// extended protocol, text results: pgx's SendBatch underneath) and prints
// the first error alone, or each result's rows and count.
func batch(ctx context.Context, c *pgconn.PgConn, s string) {
	b := &pgconn.Batch{}
	for _, it := range batchItems(s) {
		var ps [][]byte
		if it[1] != "" {
			ps = params(it[1])
		}
		b.ExecParams(it[0], ps, nil, nil, nil)
	}
	res, err := c.ExecBatch(ctx, b).ReadAll()
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		fmt.Printf("error %s %s\n", pe.Code, pe.Message)
		return
	} else if err != nil {
		fmt.Println("error", err)
		return
	}
	for _, r := range res {
		if len(r.FieldDescriptions) > 0 {
			printRows(r)
		}
		fmt.Printf("affected %d\n", r.CommandTag.RowsAffected())
	}
}

func printRows(r *pgconn.Result) {
	var cols []string
	for _, f := range r.FieldDescriptions {
		cols = append(cols, q(f.Name))
	}
	for _, row := range r.Rows {
		if len(row) != len(r.FieldDescriptions) {
			// pgconn keeps a DataRow of another width than the
			// RowDescription (a hostile server's, internal/stdtest
			// TestPgFuzz): there is no reference answer, and Xo must refuse
			fmt.Println("width pgconn")
			return
		}
	}
	fmt.Println(colTypes(r.FieldDescriptions))
	for _, row := range r.Rows {
		var vals []string
		for i, v := range row {
			if v == nil {
				vals = append(vals, "Null")
			} else {
				vals = append(vals, val(r.FieldDescriptions[i].DataTypeOID, string(v)))
			}
		}
		fmt.Printf("Row{cols: [%s], vals: [%s]}\n", strings.Join(cols, ", "), strings.Join(vals, ", "))
	}
}

var typeMap = pgtype.NewMap()

// colTypes is pgx stdlib's Rows.ColumnType* for each field (decision
// 0609): ColumnTypeDatabaseTypeName, ColumnTypeLength and
// ColumnTypePrecisionScale as stdlib/sql.go computes them from the
// field's OID and modifier. "-" where stdlib says ok = false.
func colTypes(fs []pgconn.FieldDescription) string {
	var out []string
	for _, f := range fs {
		name := strconv.FormatInt(int64(f.DataTypeOID), 10)
		if dt, ok := typeMap.TypeForOID(f.DataTypeOID); ok {
			name = strings.ToUpper(dt.Name)
		}
		length := "-"
		switch f.DataTypeOID {
		case pgtype.TextOID, pgtype.ByteaOID:
			length = strconv.FormatInt(math.MaxInt64, 10)
		case pgtype.VarcharOID, pgtype.BPCharOID:
			length = strconv.FormatInt(int64(f.TypeModifier-4), 10)
		case pgtype.VarbitOID:
			length = strconv.FormatInt(int64(f.TypeModifier), 10)
		}
		ps := "-"
		if f.DataTypeOID == pgtype.NumericOID {
			mod := f.TypeModifier - 4
			ps = fmt.Sprintf("%d,%d", (mod>>16)&0xffff, mod&0xffff)
		}
		// Xo's Str holds UTF-8: a name that is not is U+FFFD there, as in the Row line
		out = append(out, fmt.Sprintf("%s:%s:%s:%s", strings.ToValidUTF8(f.Name, "\uFFFD"), name, length, ps))
	}
	return "types " + strings.Join(out, " ")
}

// typed is a `TYPED KIND SELECT ...` line (decision 0679): each cell of
// the result scanned by pgx's pgtype from its text, as KIND (uuid, date,
// time, interval, inet), printed as std/sql's typed reader's value's
// Debug text, NULL, or "error" when pgtype refuses it.
func typed(ctx context.Context, c *pgconn.PgConn, rest string) {
	kind, query, _ := strings.Cut(rest, " ")
	res, err := readAll(c.Exec(ctx, query))
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		fmt.Printf("error %s %s\n", pe.Code, pe.Message)
		return
	} else if err != nil {
		fmt.Println("error", err)
		return
	}
	m := pgtype.NewMap()
	r := res[len(res)-1]
	for _, row := range r.Rows {
		var out []string
		for i, src := range row {
			if src == nil {
				out = append(out, "NULL")
				continue
			}
			oid := r.FieldDescriptions[i].DataTypeOID
			out = append(out, typedCell(m, kind, oid, src))
		}
		fmt.Println("typed " + strings.Join(out, " | "))
	}
}

func typedCell(m *pgtype.Map, kind string, oid uint32, src []byte) string {
	if ek, ok := strings.CutPrefix(kind, "arr:"); ok {
		// an array: pgx's array codec splits it, each element is
		// scanned with the element type (decision 0679)
		var elemOID uint32
		if t, ok := m.TypeForOID(oid); ok {
			ac, ok := t.Codec.(*pgtype.ArrayCodec)
			if !ok {
				return "error"
			}
			elemOID = ac.ElementType.OID
		} else {
			// a type pgx has not loaded (a composite's array, before
			// LoadType): its text through text[]'s parser, which does
			// not depend on the element type
			oid = pgtype.TextArrayOID
		}
		var elems []*string
		if m.Scan(oid, pgtype.TextFormatCode, src, &elems) != nil {
			return "error"
		}
		out := make([]string, len(elems))
		for i, e := range elems {
			if e == nil {
				out[i] = "NULL"
			} else {
				out[i] = typedCell(m, ek, elemOID, []byte(*e))
			}
		}
		return "[" + strings.Join(out, ", ") + "]"
	}
	switch kind {
	case "uuid":
		var u pgtype.UUID
		if m.Scan(oid, pgtype.TextFormatCode, src, &u) != nil {
			return "error"
		}
		return q(u.String())
	case "date":
		var d pgtype.Date
		if m.Scan(oid, pgtype.TextFormatCode, src, &d) != nil || d.InfinityModifier != pgtype.Finite {
			return "error"
		}
		return fmt.Sprintf("Date{year: %d, month: %d, day: %d}", d.Time.Year(), int(d.Time.Month()), d.Time.Day())
	case "time":
		var t pgtype.Time
		if m.Scan(oid, pgtype.TextFormatCode, src, &t) != nil {
			return "error"
		}
		us := t.Microseconds
		return fmt.Sprintf("ClockTime{hour: %d, minute: %d, second: %d, nanos: %d}", us/3600000000, us/60000000%60, us/1000000%60, us%1000000*1000)
	case "interval":
		var iv pgtype.Interval
		if m.Scan(oid, pgtype.TextFormatCode, src, &iv) != nil {
			return "error"
		}
		return fmt.Sprintf("Interval{months: %d, days: %d, micros: %d}", iv.Months, iv.Days, iv.Microseconds)
	case "inet":
		var p netip.Prefix
		if m.Scan(oid, pgtype.TextFormatCode, src, &p) != nil {
			return "error"
		}
		return q(p.String())
	case "fields":
		sc := pgtype.NewCompositeTextScanner(m, src)
		var parts []string
		for sc.Next() {
			if b := sc.Bytes(); b == nil {
				parts = append(parts, "None")
			} else {
				parts = append(parts, "Some("+q(string(b))+")")
			}
		}
		if sc.Err() != nil {
			return "error"
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case "numeric":
		var n pgtype.Numeric
		if m.Scan(oid, pgtype.TextFormatCode, src, &n) != nil {
			return "error"
		}
		s, _ := pgNumParts(n)
		return s
	case "numrange":
		var r pgtype.Range[pgtype.Numeric]
		if m.Scan(oid, pgtype.TextFormatCode, src, &r) != nil {
			return "error"
		}
		return rangeText(r, pgNumParts)
	case "nummulti":
		var mr pgtype.Multirange[pgtype.Range[pgtype.Numeric]]
		if m.Scan(oid, pgtype.TextFormatCode, src, &mr) != nil {
			return "error"
		}
		return multiText(mr, pgNumParts)
	case "intrange":
		var r pgtype.Range[pgtype.Int8]
		if m.Scan(oid, pgtype.TextFormatCode, src, &r) != nil {
			return "error"
		}
		return rangeText(r, func(v pgtype.Int8) (string, bool) { return strconv.FormatInt(v.Int64, 10), true })
	case "daterange":
		var r pgtype.Range[pgtype.Date]
		if m.Scan(oid, pgtype.TextFormatCode, src, &r) != nil {
			return "error"
		}
		return rangeText(r, pgDateText)
	case "timerange":
		if oid == pgtype.TsrangeOID {
			var r pgtype.Range[pgtype.Timestamp]
			if m.Scan(oid, pgtype.TextFormatCode, src, &r) != nil {
				return "error"
			}
			return rangeText(r, pgTsText)
		}
		var r pgtype.Range[pgtype.Timestamptz]
		if m.Scan(oid, pgtype.TextFormatCode, src, &r) != nil {
			return "error"
		}
		return rangeText(r, pgTstzText)
	case "intmulti":
		var mr pgtype.Multirange[pgtype.Range[pgtype.Int8]]
		if m.Scan(oid, pgtype.TextFormatCode, src, &mr) != nil {
			return "error"
		}
		return multiText(mr, func(v pgtype.Int8) (string, bool) { return strconv.FormatInt(v.Int64, 10), true })
	case "datemulti":
		var mr pgtype.Multirange[pgtype.Range[pgtype.Date]]
		if m.Scan(oid, pgtype.TextFormatCode, src, &mr) != nil {
			return "error"
		}
		return multiText(mr, pgDateText)
	case "timemulti":
		var mr pgtype.Multirange[pgtype.Range[pgtype.Timestamptz]]
		if m.Scan(oid, pgtype.TextFormatCode, src, &mr) != nil {
			return "error"
		}
		return multiText(mr, pgTstzText)
	case "datebound":
		var d pgtype.Date
		if m.Scan(oid, pgtype.TextFormatCode, src, &d) != nil {
			return "error"
		}
		switch d.InfinityModifier {
		case pgtype.Infinity:
			return "Infinity"
		case pgtype.NegativeInfinity:
			return "NegInfinity"
		}
		return fmt.Sprintf("Finite(v: Date{year: %d, month: %d, day: %d})", d.Time.Year(), int(d.Time.Month()), d.Time.Day())
	case "timebound":
		var t pgtype.Timestamptz
		if m.Scan(oid, pgtype.TextFormatCode, src, &t) != nil {
			return "error"
		}
		switch t.InfinityModifier {
		case pgtype.Infinity:
			return "Infinity"
		case pgtype.NegativeInfinity:
			return "NegInfinity"
		}
		return "Finite(" + val(oid, string(src)) + ")"
	case "mac":
		var h net.HardwareAddr
		if m.Scan(oid, pgtype.TextFormatCode, src, &h) != nil {
			return "error"
		}
		return q(hex.EncodeToString(h))
	case "hstore":
		var h pgtype.Hstore
		if h.Scan(string(src)) != nil {
			return "error"
		}
		keys := make([]string, 0, len(h))
		for k := range h {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var parts []string
		for _, k := range keys {
			v := "NULL"
			if h[k] != nil {
				v = q(*h[k])
			}
			parts = append(parts, q(k)+": "+v)
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}
	return "error unknown kind " + kind
}

// rangeText is a range in the pgdiff form both sides print: `[a,b)`,
// `(,b]`, `empty`; "error" when a bound has no Xo value (an infinite
// date or timestamp).
func rangeText[T any](r pgtype.Range[T], f func(T) (string, bool)) string {
	if r.LowerType == pgtype.Empty {
		return "empty"
	}
	var b strings.Builder
	switch r.LowerType {
	case pgtype.Inclusive, pgtype.Exclusive:
		v, ok := f(r.Lower)
		if !ok {
			return "error"
		}
		if r.LowerType == pgtype.Inclusive {
			b.WriteString("[" + v)
		} else {
			b.WriteString("(" + v)
		}
	default:
		b.WriteString("(")
	}
	b.WriteString(",")
	switch r.UpperType {
	case pgtype.Inclusive, pgtype.Exclusive:
		v, ok := f(r.Upper)
		if !ok {
			return "error"
		}
		if r.UpperType == pgtype.Inclusive {
			b.WriteString(v + "]")
		} else {
			b.WriteString(v + ")")
		}
	default:
		b.WriteString(")")
	}
	return b.String()
}

func multiText[T any](mr pgtype.Multirange[pgtype.Range[T]], f func(T) (string, bool)) string {
	parts := make([]string, 0, len(mr))
	for _, r := range mr {
		t := rangeText(r, f)
		if t == "error" {
			return t
		}
		parts = append(parts, t)
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func pgDateText(d pgtype.Date) (string, bool) {
	if d.InfinityModifier != pgtype.Finite {
		return "", false
	}
	return fmt.Sprintf("%d-%02d-%02d", d.Time.Year(), int(d.Time.Month()), d.Time.Day()), true
}

func pgTsText(t pgtype.Timestamp) (string, bool) {
	if t.InfinityModifier != pgtype.Finite {
		return "", false
	}
	return t.Time.UTC().Format(time.RFC3339Nano), true
}

func pgTstzText(t pgtype.Timestamptz) (string, bool) {
	if t.InfinityModifier != pgtype.Finite {
		return "", false
	}
	return t.Time.UTC().Format(time.RFC3339Nano), true
}

// pgNumParts is a numeric as both sides print it: COEFeEXP, NaN,
// Infinity, -Infinity.
func pgNumParts(n pgtype.Numeric) (string, bool) {
	switch {
	case n.NaN:
		return "NaN", true
	case n.InfinityModifier == pgtype.Infinity:
		return "Infinity", true
	case n.InfinityModifier == pgtype.NegativeInfinity:
		return "-Infinity", true
	}
	return n.Int.String() + "e" + strconv.Itoa(int(n.Exp)), true
}

// largeObject is an `LO DATA` line (decision 0679): pgx's LargeObjects
// through create, write, seek, read, tell, truncate, close and unlink in
// one transaction, then an unlink of the removed object in another.
func largeObject(ctx context.Context, dsn, data string) {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		fmt.Println("error connect")
		return
	}
	defer conn.Close(ctx)
	say := func(what string, v any, err error) bool {
		var pe *pgconn.PgError
		if errors.As(err, &pe) {
			fmt.Printf("%s: error %s\n", what, pe.Code)
			return false
		}
		if err != nil {
			fmt.Printf("%s: error %v\n", what, err)
			return false
		}
		fmt.Printf("%s: %v\n", what, v)
		return true
	}
	tx, err := conn.Begin(ctx)
	if !say("begin", "ok", err) {
		return
	}
	los := tx.LargeObjects()
	oid, err := los.Create(ctx, 0)
	if !say("create", oid > 0, err) {
		return
	}
	lo, err := los.Open(ctx, oid, pgx.LargeObjectModeRead|pgx.LargeObjectModeWrite)
	if !say("open", "ok", err) {
		return
	}
	n, err := lo.Write([]byte(data))
	say("write", n, err)
	pos, err := lo.Seek(0, 0)
	say("seek", pos, err)
	buf := make([]byte, 100)
	n, err = lo.Read(buf)
	if err == io.EOF {
		err = nil
	}
	say("read", q(string(buf[:n])), err)
	pos, err = lo.Tell()
	say("tell", pos, err)
	say("truncate", "ok", lo.Truncate(3))
	pos, err = lo.Seek(-1, 2)
	say("seek end", pos, err)
	n, err = lo.Read(buf)
	if err == io.EOF {
		err = nil
	}
	say("read end", q(string(buf[:n])), err)
	say("close", "ok", lo.Close())
	say("unlink", "ok", los.Unlink(ctx, oid))
	say("commit", "ok", tx.Commit(ctx))
	tx, err = conn.Begin(ctx)
	if !say("begin", "ok", err) {
		return
	}
	los2 := tx.LargeObjects()
	say("unlink again", "ok", los2.Unlink(ctx, oid))
	say("rollback", "ok", tx.Rollback(ctx))
}
