// Reference for the Postgres differential test (scripts/pg-diff.sh): each statement through
// pgx's pgconn (simple protocol), its last result printed with spec/std.md
// 10.9's mapping of text values by type OID, in the Debug form of the
// native std/sql Row, or the error's SQLSTATE and message.
package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
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
	case 20, 21, 23, 26:
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
		stmt, args, hasArgs := strings.Cut(line, " ;; ")
		var res []*pgconn.Result
		if hasArgs {
			r := c.ExecParams(ctx, stmt, params(args), nil, nil, nil).Read()
			res, err = []*pgconn.Result{r}, r.Err
		} else {
			res, err = c.Exec(ctx, line).ReadAll()
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
		var cols []string
		for _, f := range r.FieldDescriptions {
			cols = append(cols, q(f.Name))
		}
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
}
